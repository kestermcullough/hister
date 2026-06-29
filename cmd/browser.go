package cmd

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/asciimoo/hister/client"
	"github.com/asciimoo/hister/server/crawler"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var browserImportCmd = &cobra.Command{
	Use:   "import-browser [BROWSER_TYPE] [DB_PATH]",
	Short: "Import Chrome, Firefox or auto-detect browsing history",
	Long: `
Import browsing history from a supported browser.

Usage:
  import-browser                        - auto-detect all installed browsers
  import-browser BROWSER_TYPE           - auto-detect database path
  import-browser DB_PATH                - auto-detect browser type
  import-browser BROWSER_TYPE DB_PATH   - import a browser type with a specific database path

Supported for browser types for auto-detecting: firefox, chrome, chromium, brave, edge, vivaldi, opera, zen, waterfox, Ladybird

The Firefox URL database is usually located at ~/.mozilla/firefox/*.default/places.sqlite
The Chrome/Chromium URL database is usually located at ~/.config/chromium/Default/History
`,
	Args: cobra.RangeArgs(0, 2),
	Run:  importHistory,
}

type browserDBCandidates struct {
	name             string
	table_name       string
	paths_candidates []string
}

type browserDB struct {
	name       string
	table_name string
	paths      []string
}

type importHistoryMultipleChoicePrompt struct {
	choice string
	urls   int
	db     *sql.DB
	q      string
	c      *client.Client
}

type DBToImport struct {
	name         string
	table        string
	databaseFile string
	browserType  string
	db           *sql.DB
	q            string
	c            *client.Client
	count        int
}

func importHistory(cmd *cobra.Command, args []string) {
	// TODO: get skip rules from server
	cfg.Crawler.UserAgent = UserAgent
	applyCrawlerBackendFlags(cmd)

	switch len(args) {
	case 0:
		// Auto-detect all installed browsers.
		dbs := getDBPaths()
		if len(dbs) == 0 {
			log.Fatal().Msg("no browser databases found")
		}
		var databases []DBToImport
		for _, db := range dbs {
			for _, path := range db.paths {
				databases = append(databases, DBToImport{
					table:        db.table_name,
					databaseFile: path,
				})
			}
		}
		importDB(databases, cmd)

	case 1, 2:
		if len(args) == 1 {
			// check if args[0] is a file or not and call the correct function
			if _, err := os.Stat(args[0]); os.IsNotExist(err) {
				importBrowser(strings.ToLower(args[0]), cmd)
			} else {
				importHistoryFile(args[0], cmd)
			}
		} else {
			browser := args[0]
			table_name := browserTableName(browser)
			if table_name == "" {
				log.Warn().Msg(fmt.Sprintf("Unknown browser, couldn't auto detect table name using %s as table name", browser))
				table_name = browser
			}
			importDB([]DBToImport{
				{
					table:        table_name,
					databaseFile: args[1],
				},
			},
				cmd)
		}

	default:
		log.Fatal().Msg(cmd.Long)
	}

	// TODO optional date filter
	//vf := "last_visit_time"
	//if browser == "firefox" {
	//	vf = "last_visit_date"
	//}
	//q += fmt.Sprintf(" AND %s >= datetime('now', 'localtime', '-1 month')", vf)
}

func importBrowser(browser string, cmd *cobra.Command) {
	var found bool

	for _, db := range getDBPaths() {
		if strings.HasPrefix(strings.ToLower(db.name), browser) {
			found = true
			for _, path := range db.paths {
				importDB([]DBToImport{
					{
						table:        db.table_name,
						databaseFile: path,
					},
				},
					cmd)
			}
		}
	}
	if !found {
		log.Fatal().Str("browser", browser).Msg("no database found for browser")
	}
}

func importHistoryFile(file_path string, cmd *cobra.Command) {
	var table string

	if strings.HasSuffix(file_path, "places.sqlite") {
		table = "moz_places"
	} else if strings.HasSuffix(file_path, "History") {
		table = "urls"
	} else if strings.HasSuffix(file_path, "History.db") {
		table = "History"
	} else {
		log.Fatal().Str("file", file_path).Msg("Couldn't auto detect table")
	}

	importDB([]DBToImport{
		{
			table:        table,
			databaseFile: file_path,
		},
	},
		cmd)
}

func importDB(databases []DBToImport, cmd *cobra.Command) {
	var dbsToImport []importHistoryMultipleChoicePrompt
	for _, database := range databases {
		dbFile := database.databaseFile
		table := database.table

		db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?immutable=1&mode=ro", dbFile))
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to open database")
		}
		defer func() {
			if err := db.Close(); err != nil {
				log.Warn().Err(err).Msg("failed to close database")
			}
		}()

		// Fetch skip rules from the server.
		c := newClient()
		resp, err := c.FetchRules()
		if err != nil {
			log.Error().Err(err).Msg("Unable to obtain skip rules from server; using local ones instead")
		} else {
			// TODO: let the user know that their local rules are being overwritten?
			cfg.Rules.Skip.ReStrs = resp.Skip
			if err := cfg.Rules.Skip.Compile(); err != nil {
				log.Error().Err(err).Msg("Unable to compile skip rules from server")
				return
			}
		}

		q := fmt.Sprintf("SELECT DISTINCT count(url) FROM %s WHERE url LIKE 'http://%%' OR url LIKE 'https://%%'", table)
		if i, err := cmd.Flags().GetInt("min-visit"); err == nil && i > 1 {
			q += fmt.Sprintf(" AND visit_count >= %d", i)
		}
		// TODO: apply skip rules to get a more precise count?
		row := db.QueryRow(q)
		var count int
		if err := row.Scan(&count); err != nil {
			log.Debug().Str("query", q).Msg("count query")
			log.Error().Err(err).Msg("Failed to execute counting query")
			return
		}

		if count < 1 {
			exit(1, "No URLs found to import")
		}
		dbsToImport = append(dbsToImport, importHistoryMultipleChoicePrompt{dbFile, count, db, q, c})
		// if !yesNoPrompt(fmt.Sprintf("%d URLs found. Start import form "+dbFile, count), true) {
		// 	return
		// }
	}

	chosen := multipleChoiceImport(dbsToImport)

	for _, database := range chosen {
		q := database.q
		c := database.c
		count := database.count
		db := database.db

		q = strings.Replace(q, "count(url)", "url", 1)
		q += " ORDER BY visit_count DESC"

		fmt.Println(cliBoldStyle.Render("IMPORTING"))

		// Create the crawler once so it is reused across all URLs.
		cfg.Crawler.UserAgent = UserAgent
		cr, crErr := crawler.New(&cfg.Crawler, nil)
		if crErr != nil {
			log.Fatal().Err(crErr).Msg("Failed to create crawler")
		}
		defer func() {
			if err := cr.Close(); err != nil {
				log.Warn().Err(err).Msg("crawler close error")
			}
		}()

		rows, err := db.Query(q, "url")
		if err != nil {
			log.Error().Err(err).Msg("Failed to execute database query")
			return
		}
		defer func() {
			if err := rows.Close(); err != nil {
				log.Warn().Err(err).Msg("failed to close database rows")
			}
		}()
		i := 0
		skipped := 0
		for rows.Next() {
			i += 1
			var u string
			err = rows.Scan(&u)
			if err != nil {
				log.Error().Err(err).Msg("Failed to scan database row")
				return
			}
			// skip URLs only in single user environments
			if !cfg.App.UserHandling && cfg.Rules.IsSkip(u) {
				log.Debug().Str("URL", u).Msg("skip importing URL by rule")
				continue
			}
			exists, err := c.DocumentExists(u)
			if err != nil {
				log.Warn().Err(err).Str("URL", u).Msg("Failed to get info about URL, skipping")
				skipped += 1
				continue
			}
			if exists {
				// skip already added URLs
				continue
			}
			fmt.Printf("[%d/%d] %s\n", i, count, u)
			if err := indexURL(cr, u, ""); err != nil {
				log.Warn().Err(err).Str("url", u).Msg("Failed to index URL")
			}
		}

		if skipped != 0 {
			log.Info().Msgf("Skipped %d URLs", skipped)
		}
	}
}

func getDBPaths() []browserDB {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var candidates []browserDBCandidates

	chromium_table := "urls"
	firefox_table := "moz_places"
	ladybird_table := "History"

	switch runtime.GOOS {
	default:
		log.Fatal().Msgf("Failed to detect os")
	case "darwin":
		candidates = []browserDBCandidates{
			// firefox
			{
				"Firefox",
				firefox_table,
				[]string{
					filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles", "*.default*", "places.sqlite"),
					filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles", "*.default-release*", "places.sqlite"),
				},
			},
			{
				"Firefox Developer Edition",
				firefox_table,
				[]string{
					filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles", "*.dev-edition-default*", "places.sqlite"),
				},
			},
			{
				"Zen",
				firefox_table,
				[]string{
					filepath.Join(home, "Library", "Application Support", "zen", "Profiles", "*Default*", "places.sqlite"),
				},
			},
			{
				"Waterfox",
				firefox_table,
				[]string{
					filepath.Join(home, "Library", "Application Support", "Waterfox", "Profiles", "*.default*", "places.sqlite"),
				},
			},
			{
				"Ladybird",
				ladybird_table,
				[]string{
					filepath.Join(home, "Library", "Application Support", "Ladybird", "History.db"),
				},
			},
			{
				"Chrome",
				chromium_table,
				[]string{
					filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "History"),
					filepath.Join(home, "Library", "Application Support", "Google", "Chrome Beta", "Default", "History"),
					filepath.Join(home, "Library", "Application Support", "Google", "Chrome Canary", "Default", "History"),
				},
			},
			{
				"Chromium",
				chromium_table,
				[]string{
					filepath.Join(home, "Library", "Application Support", "Chromium", "Default", "History"),
				},
			},
			{
				"Brave",
				chromium_table,
				[]string{
					filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser", "Default", "History"),
					filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser-Beta", "Default", "History"),
				},
			},
			{
				"Edge",
				chromium_table,
				[]string{
					filepath.Join(home, "Library", "Application Support", "Microsoft Edge", "Default", "History"),
					filepath.Join(home, "Library", "Application Support", "Microsoft Edge Beta", "Default", "History"),
				},
			},
			{
				"Vivaldi",
				chromium_table,
				[]string{
					filepath.Join(home, "Library", "Application Support", "Vivaldi", "Default", "History"),
				},
			},
			{
				"Opera",
				chromium_table,
				[]string{
					filepath.Join(home, "Library", "Application Support", "com.operasoftware.Opera", "Default", "History"),
				},
			},
		}
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		appData := os.Getenv("APPDATA")
		if localAppData != "" {
			candidates = []browserDBCandidates{
				{
					"firefox",
					firefox_table,
					[]string{
						filepath.Join(appData, "Mozilla", "Firefox", "Profiles", "*.default*", "places.sqlite"),
						filepath.Join(appData, "Mozilla", "Firefox", "Profiles", "*.default-release*", "places.sqlite"),
					},
				},
				{
					"Zen",
					firefox_table,
					[]string{
						filepath.Join(appData, "zen", "Profiles", "*.Default*", "places.sqlite"),
					},
				},
				{
					"Waterfox",
					firefox_table,
					[]string{
						filepath.Join(appData, "Waterfox", "Profiles", "*.default*", "places.sqlite"),
					},
				},
				{
					"Chrome",
					chromium_table,
					[]string{
						filepath.Join(localAppData, "Google", "Chrome", "User Data", "Default", "History"),
						filepath.Join(localAppData, "Google", "Chrome Beta", "User Data", "Default", "History"),
					},
				},
				{
					"Chromium",
					chromium_table,
					[]string{
						filepath.Join(localAppData, "Chromium", "User Data", "Default", "History"),
					},
				},
				{
					"Brave",
					chromium_table,
					[]string{
						filepath.Join(localAppData, "BraveSoftware", "Brave-Browser", "User Data", "Default", "History"),
					},
				},
				{
					"Edge",
					chromium_table,
					[]string{
						filepath.Join(localAppData, "Microsoft", "Edge", "User Data", "Default", "History"),
					},
				},
				{
					"Vivaldi",
					chromium_table,
					[]string{
						filepath.Join(localAppData, "Vivaldi", "User Data", "Default", "History"),
					},
				},
				{
					"Opera",
					chromium_table,
					[]string{
						filepath.Join(appData, "Opera Software", "Opera Stable", "History"),
					},
				},
			}
		}
	case "linux":
		candidates = []browserDBCandidates{
			{
				"firefox",
				firefox_table,
				[]string{
					filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox", "*.default*", "places.sqlite"),
					filepath.Join(home, ".mozilla", "firefox", "*.default*", "places.sqlite"),
				},
			},
			{
				"Firefox Developer Edition",
				firefox_table,
				[]string{
					filepath.Join(home, ".mozilla", "firefox", "*.dev-edition-default*", "places.sqlite"),
				},
			},
			{
				"Zen",
				firefox_table,
				[]string{
					filepath.Join(home, ".zen", "*.Default*", "places.sqlite"),
					filepath.Join(home, ".config", "zen", "*.Default*", "places.sqlite"),
				},
			},
			{
				"Waterfox",
				firefox_table,
				[]string{
					filepath.Join(home, ".waterfox", "Profiles", "*.default*", "places.sqlite"),
				},
			},
			{
				"Ladybird",
				ladybird_table,
				[]string{
					filepath.Join(home, ".local", "share", "Ladybird", "History.db"),
				},
			},
			{
				"Chrome",
				chromium_table,
				[]string{
					filepath.Join(home, ".config", "google-chrome", "Default", "History"),
					filepath.Join(home, ".config", "google-chrome-beta", "Default", "History"),
				},
			},
			{
				"Chromium",
				chromium_table,
				[]string{
					filepath.Join(home, ".config", "chromium", "Default", "History"),
					filepath.Join(home, "snap", "chromium", "common", "chromium", "Default", "History"),
				},
			},
			{
				"Brave",
				chromium_table,
				[]string{
					filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser", "Default", "History"),
				},
			},
			{
				"Edge",
				chromium_table,
				[]string{
					filepath.Join(home, ".config", "microsoft-edge", "Default", "History"),
					filepath.Join(home, ".config", "microsoft-edge-beta", "Default", "History"),
				},
			},
			{
				"Vivaldi",
				chromium_table,
				[]string{
					filepath.Join(home, ".config", "vivaldi", "Default", "History"),
				},
			},
			{
				"Opera",
				chromium_table,
				[]string{
					filepath.Join(home, ".config", "opera", "Default", "History"),
				},
			},
		}
	}

	var dbFiles []browserDB
	var paths []string

	for _, candidate := range candidates {
		for _, globs := range candidate.paths_candidates {
			matches, _ := filepath.Glob(globs)
			for _, p := range matches {
				if _, err := os.Stat(p); err == nil {
					paths = append(paths, p)
				}
			}
		}

		if len(paths) != 0 {
			dbFiles = append(dbFiles, browserDB{candidate.name, candidate.table_name, paths})
		}
		paths = []string{}
	}
	return dbFiles
}

func browserTableName(browser string) string {
	switch strings.ToLower(browser) {
	case "firefox", "zen", "waterfox":
		return "moz_places"
	case "chrome", "chromium", "brave", "edge", "vivaldi", "opera":
		return "urls"
	case "ladybird":
		return "History"
	}
	return ""
}

func multipleChoiceImport(choices []importHistoryMultipleChoicePrompt) []DBToImport {
	r := bufio.NewReader(os.Stdin)
	var s string
	var returnDBs []DBToImport
	println("----Available Histories----")
	for i, choiceData := range choices {
		prefix := getBrowserType(choiceData.choice)
		choice := fmt.Sprint(strconv.Itoa(i), "  |  ", prefix, "  ", choiceData.choice, "  urls: ", choiceData.urls)
		println(choice)
		returnDBs = append(returnDBs, DBToImport{
			name:        prefix,
			browserType: prefix,
			count:       choiceData.urls,
			db:          choiceData.db,
			q:           choiceData.q,
			c:           choiceData.c,
		})
	}
	println("==> Histories to exclude: (eg: \"1 2 3\", browser name or leave empty to to import all)")
	print("==> ")

	s, _ = r.ReadString('\n')

	blacklists := strings.Split(strings.Trim(s, "\n"), " ")

	// Handle blacklisted imports
	var selected []DBToImport
	var unselected bool
	for i, data := range returnDBs {
		for _, blacklist := range blacklists {
			if strconv.Itoa(i) == blacklist || data.name == blacklist {
				unselected = true
				break
			}
		}
		if !unselected {
			selected = append(selected, data)
		}
		unselected = false
	}
	return selected
}

func getBrowserType(path string) string {
	path = strings.ToLower(path)
	if strings.Contains(path, "firefox") {
		return "firefox"
	} else if strings.Contains(path, "zen") {
		return "zen"
	} else if strings.Contains(path, "waterfox") {
		return "waterfox"
	} else if strings.Contains(path, "chrome") {
		return "chrome"
	} else if strings.Contains(path, "chromium") {
		return "chromium"
	} else if strings.Contains(path, "brave") {
		return "brave"
	} else if strings.Contains(path, "edge") {
		return "edge"
	} else if strings.Contains(path, "vivaldi") {
		return "vivaldi"
	} else if strings.Contains(path, "opera") {
		return "opera"
	} else if strings.Contains(path, "ladybird") {
		return "ladybird"
	} else {
		return "unknown"
	}
}
