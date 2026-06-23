package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/asciimoo/hister/client"
	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/files"
	"github.com/asciimoo/hister/server"
	"github.com/asciimoo/hister/server/crawler"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/extractor"
	"github.com/asciimoo/hister/server/indexer"
	"github.com/asciimoo/hister/server/model"
	"github.com/asciimoo/hister/ui"

	"github.com/bodgit/sevenzip"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const versionBase = "v0.15.0"

var Version = func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return fmt.Sprintf("%s (%s)", versionBase, s.Value[:7])
			}
		}
	}
	return versionBase
}()

var (
	cliErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	cliSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	cliInfoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	cliWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	cliBoldStyle    = lipgloss.NewStyle().Bold(true)
)

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

var (
	cfgFile   string
	cfg       *config.Config
	UserAgent = fmt.Sprintf("Mozilla/5.0 (compatible; Hister/%s; +https://hister.org/)", Version)
)

// stringToAnyMap converts map[string]string to map[string]any, used when
// applying --backend-option flag values to crawler config.
func stringToAnyMap(m map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// parseCookieFlag parses a Set-Cookie header value (e.g. "session=abc; Domain=example.com; Path=/")
// into a CrawlerCookie. Domain is required.
func parseCookieFlag(s string) (config.CrawlerCookie, error) {
	c, err := http.ParseSetCookie(s)
	if err != nil {
		return config.CrawlerCookie{}, fmt.Errorf("cookie %q: %w", s, err)
	}
	if c.Domain == "" {
		return config.CrawlerCookie{}, fmt.Errorf("cookie %q: Domain attribute is required", s)
	}
	path := c.Path
	if path == "" {
		path = "/"
	}
	return config.CrawlerCookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: path}, nil
}

// applyCrawlerBackendFlags reads --backend, --backend-option, --header, and --cookie
// flags from cmd and applies them to cfg.Crawler, overriding any config-file values.
func applyCrawlerBackendFlags(cmd *cobra.Command) {
	if b, _ := cmd.Flags().GetString("backend"); b != "" {
		cfg.Crawler.Backend = b
	}
	if opts, _ := cmd.Flags().GetStringToString("backend-option"); len(opts) > 0 {
		cfg.Crawler.BackendOptions = stringToAnyMap(opts)
	}
	if headers, _ := cmd.Flags().GetStringToString("header"); len(headers) > 0 {
		if cfg.Crawler.Headers == nil {
			cfg.Crawler.Headers = make(map[string]string)
		}
		maps.Copy(cfg.Crawler.Headers, headers)
	}
	if cookies, _ := cmd.Flags().GetStringArray("cookie"); len(cookies) > 0 {
		for _, raw := range cookies {
			ck, err := parseCookieFlag(raw)
			if err != nil {
				exit(1, err.Error())
			}
			cfg.Crawler.Cookies = append(cfg.Crawler.Cookies, ck)
		}
	}
}

var rootCmd = &cobra.Command{
	Use:     "hister",
	Short:   "Your own search engine",
	Long:    "Hister - your own search engine",
	Version: Version,
	//Run: func(_ *cobra.Command, _ []string) {
	//},
}

var listenCmd = &cobra.Command{
	Use:   "listen",
	Short: "Start server",
	Long:  ``,
	PreRun: func(_ *cobra.Command, _ []string) {
		initIndex()
	},
	Run: func(cmd *cobra.Command, _ []string) {
		if a, err := cmd.Flags().GetString("address"); err == nil && cmd.Flags().Changed("address") {
			if err := cfg.UpdateListenAddress(a); err != nil {
				exit(1, `Failed to set server address: `+err.Error())
			}
		}
		if cfg.App.AccessToken != "" && strings.HasPrefix(cfg.BaseURL(""), "http://") {
			log.Warn().Msg("Using authentication token without https. Token is sent plain-text in network requests.")
		}
		if len(cfg.Indexer.Directories) > 0 {
			indexer.IndexAll(cfg.Indexer.Directories)
			go func() {
				if err := files.WatchDirectories(context.Background(), cfg.Indexer.Directories, func(path string) {
					userID, err := files.FindDirUser(cfg.Indexer.Directories, path)
					if err != nil {
						log.Error().Err(err).Str("path", path).Msg("Failed to resolve user for file")
						return
					} else if userID != 0 && !cfg.App.UserHandling {
						log.Error().Str("path", path).Msg("user field set but user_handling is not enabled")
						return
					}
					if err := indexer.IndexFile(path, userID); err != nil {
						log.Debug().Err(err).Str("path", path).Msg("Failed to index file")
					}
				}, func(path string) {
					if err := indexer.DeleteFile(path); err != nil {
						log.Debug().Err(err).Str("path", path).Msg("Failed to delete file from index")
					}
				}); err != nil {
					log.Error().Err(err).Msg("File watcher failed")
				}
			}()
		}
		server.Version = Version
		server.Listen(cfg)
	},
}

var createConfigCmd = &cobra.Command{
	Use:   "create-config [FILENAME]",
	Short: "Create default configuration file",
	Args:  cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		dcfg := config.CreateDefaultConfig()
		cb, err := yaml.Marshal(dcfg)
		if err != nil {
			panic(err)
		}
		if len(args) > 0 {
			fname := args[0]
			if _, err := os.Stat(fname); err == nil {
				exit(1, fmt.Sprintf(`File "%s" already exists`, fname))
			}
			if err := os.WriteFile(fname, cb, 0o600); err != nil {
				exit(1, `Failed to create config file: `+err.Error())
			}
			fmt.Println(cliSuccessStyle.Render("✓") + " Config file created: " + cliInfoStyle.Render(fname))
		} else {
			fmt.Print(string(cb))
		}
	},
}

var listURLsCmd = &cobra.Command{
	Use:   "list-urls",
	Short: "List indexed URLs",
	Long:  `List all indexed URLs by fetching them from the running server`,
	PreRun: func(cmd *cobra.Command, _ []string) {
		offline, _ := cmd.Flags().GetBool("offline")
		if offline {
			initIndex()
		}
	},
	Run: func(cmd *cobra.Command, _ []string) {
		offline, _ := cmd.Flags().GetBool("offline")
		if offline {
			indexer.Iterate(func(doc *document.Document) {
				fmt.Println(doc.URL)
			})
			return
		}
		c := newClient(client.WithTimeout(0))
		pageKey := ""
		for {
			res, err := c.Search(&indexer.Query{Text: "*", PageKey: pageKey, Sort: "domain"})
			if err != nil {
				exit(1, "Failed to fetch URLs: "+err.Error())
			}
			for _, doc := range res.Documents {
				fmt.Println(doc.URL)
			}
			if res.PageKey == "" || len(res.Documents) == 0 {
				break
			}
			pageKey = res.PageKey
		}
	},
}

var listFilesCmd = &cobra.Command{
	Use:   "list-files",
	Short: "List all watched files for indexing",
	Long:  `List all files that match the configured directory watch patterns`,
	Run: func(_ *cobra.Command, _ []string) {
		if len(cfg.Indexer.Directories) == 0 {
			exit(1, "No directories configured for watching")
		}
		for _, dir := range cfg.Indexer.Directories {
			expanded := files.ExpandHome(dir.Path)
			err := filepath.WalkDir(expanded, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					log.Warn().Err(err).Str("path", path).Msg("Error accessing path")
					return nil
				}
				if d.IsDir() {
					if path != expanded && files.ShouldSkipDir(d.Name(), dir.Excludes, dir.IncludeHidden) {
						return filepath.SkipDir
					}
					return nil
				}
				if dir.IsMatching(d.Name()) {
					fmt.Println(path)
				}
				return nil
			})
			if err != nil {
				log.Error().Err(err).Str("directory", expanded).Msg("Failed to walk directory")
			}
		}
	},
}

var browserImportCmd = &cobra.Command{
	Use:   "import-browser [BROWSER_TYPE] [DB_PATH]",
	Short: "Import Chrome, Firefox or auto-detect browsing history",
	Long: `
Import browsing history from a supported browser.

Usage:
  import-browser                        - auto-detect all installed browsers
  import-browser BROWSER_TYPE 			- auto-detect database path
  import-browser DB_PATH				- auto-detect browser type
  import-browser BROWSER_TYPE DB_PATH   - import a browser type with a specific database path

Supported for browser types for auto-detecting: firefox, chrome, chromium, brave, edge, vivaldi, opera, zen, waterfox, Ladybird

The Firefox URL database is usually located at ~/.mozilla/firefox/*.default/places.sqlite
The Chrome/Chromium URL database is usually located at ~/.config/chromium/Default/History
`,
	Args: cobra.RangeArgs(0, 2),
	Run:  importHistory,
}

// searchDocToMap converts a document to a flat map of all available fields.
func searchDocToMap(d *document.Document) map[string]any {
	return map[string]any{
		"id":       d.ID(),
		"url":      d.URL,
		"title":    d.Title,
		"domain":   d.Domain,
		"score":    d.Score,
		"added":    d.Added,
		"language": d.Language,
		"type":     d.Type,
		"text":     d.Text,
		"favicon":  d.Favicon,
		"user_id":  d.UserID,
		"html":     d.HTML,
	}
}

// searchFilterMap returns only the requested keys; returns the full map when fields is empty.
func searchFilterMap(m map[string]any, fields []string) map[string]any {
	if len(fields) == 0 {
		return m
	}
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		out[f] = m[f]
	}
	return out
}

var searchCmd = &cobra.Command{
	Use:   "search [search terms]",
	Short: "Command line search interface",
	Long:  "Command line search interface.\nRun it without arguments to use the TUI interface or pass search terms as arguments to get results on the STDOUT.",
	Args:  cobra.MinimumNArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			if err := ui.SearchTUI(cfg); err != nil {
				exit(1, err.Error())
			}
			return
		}
		qs := strings.Join(args, " ")
		format, _ := cmd.Flags().GetString("format")
		limit, _ := cmd.Flags().GetInt("limit")

		// Parse and validate --fields.
		var fields []string
		includeHTML := false
		if fieldsRaw, _ := cmd.Flags().GetString("fields"); fieldsRaw != "" {
			validFields := map[string]bool{
				"id": true, "url": true, "title": true, "domain": true, "score": true,
				"added": true, "language": true, "type": true, "text": true,
				"favicon": true, "user_id": true, "html": true,
			}
			for f := range strings.SplitSeq(fieldsRaw, ",") {
				f = strings.TrimSpace(f)
				if f == "" {
					continue
				}
				if !validFields[f] {
					exit(1, "Unknown field: "+f+" (valid fields: id, url, title, domain, score, added, language, type, text, favicon, user_id, html)")
				}
				fields = append(fields, f)
				if f == "html" {
					includeHTML = true
				}
			}
		}

		// CSV column order: use --fields if given, else a sensible default.
		csvFields := fields
		if format == "csv" && len(csvFields) == 0 {
			csvFields = []string{"title", "url", "domain", "score", "added", "language", "text"}
		}

		// printDoc emits a single document in the requested format.
		var csvWriter *csv.Writer
		printDoc := func(d *document.Document) {
			m := searchFilterMap(searchDocToMap(d), fields)
			switch format {
			case "json":
				b, err := json.Marshal(m)
				if err != nil {
					exit(1, "Failed to encode JSON: "+err.Error())
				}
				fmt.Printf("%s,\n", b)
			case "csv":
				row := make([]string, 0, len(csvFields))
				for _, f := range csvFields {
					row = append(row, fmt.Sprintf("%v", m[f]))
				}
				if err := csvWriter.Write(row); err != nil {
					exit(1, "Failed to write CSV row: "+err.Error())
				}
			default:
				if len(fields) == 0 {
					fmt.Printf("%s\n%s\n\n", d.Title, d.URL)
				} else {
					parts := make([]string, 0, len(fields))
					for _, f := range fields {
						parts = append(parts, fmt.Sprintf("%v", m[f]))
					}
					fmt.Println(strings.Join(parts, "\n"))
					if len(fields) > 1 {
						fmt.Println()
					}
				}
			}
		}

		// Format-specific initialisation.
		switch format {
		case "json":
			fmt.Println("[")
		case "csv":
			csvWriter = csv.NewWriter(os.Stdout)
			if err := csvWriter.Write(csvFields); err != nil {
				exit(1, "Failed to write CSV header: "+err.Error())
			}
		}

		// Page through all results, streaming output directly.
		c := newClient()
		var (
			pageKey string
			total   int
			done    bool
		)
		for !done {
			res, err := c.Search(&indexer.Query{Text: qs, IncludeHTML: includeHTML, PageKey: pageKey})
			if err != nil {
				exit(1, "Search failed: "+err.Error())
			}
			for _, d := range res.Documents {
				printDoc(d)
				total++
				if limit > 0 && total >= limit {
					done = true
					break
				}
			}
			if res.PageKey == "" || len(res.Documents) == 0 {
				done = true
			}
			pageKey = res.PageKey
		}

		// Format-specific teardown.
		switch format {
		case "json":
			fmt.Println("]")
		case "csv":
			csvWriter.Flush()
			if err := csvWriter.Error(); err != nil {
				exit(1, "Failed to write CSV: "+err.Error())
			}
		}
	},
}

var indexCmd = &cobra.Command{
	Use:   "index URL [URL...]",
	Short: "Index URL [URL...]",
	Long:  "Index one or more URLs",
	Args:  cobra.MinimumNArgs(0),
	PreRun: func(cmd *cobra.Command, args []string) {
		recursive, _ := cmd.Flags().GetBool("recursive")
		jobID, _ := cmd.Flags().GetString("job-id")
		if recursive || jobID != "" {
			initDB()
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		global, _ := cmd.Flags().GetBool("global")
		targetUserID, _ := cmd.Flags().GetUint("user-id")
		userIDChanged := cmd.Flags().Changed("user-id")
		if global && userIDChanged {
			exit(1, "--global and --user-id are mutually exclusive")
		}

		var clientOpts []client.Option
		if global {
			clientOpts = append(clientOpts, client.WithTargetUserID(0))
		} else if userIDChanged {
			clientOpts = append(clientOpts, client.WithTargetUserID(targetUserID))
		}
		if allowSensitive, _ := cmd.Flags().GetBool("allow-sensitive"); allowSensitive {
			clientOpts = append(clientOpts, client.WithAllowSensitive())
		}

		force, _ := cmd.Flags().GetBool("force")
		recursive, _ := cmd.Flags().GetBool("recursive")
		jobID, _ := cmd.Flags().GetString("job-id")
		label, _ := cmd.Flags().GetString("label")
		noRobots, _ := cmd.Flags().GetBool("no-robots")
		cfg.Crawler.UserAgent = UserAgent
		applyCrawlerBackendFlags(cmd)
		if ua, _ := cmd.Flags().GetString("user-agent"); ua != "" {
			UserAgent = ua
			cfg.Crawler.UserAgent = ua
		}
		if cmd.Flags().Changed("delay") {
			d, _ := cmd.Flags().GetInt("delay")
			cfg.Crawler.Delay = d
		}
		if cmd.Flags().Changed("timeout") {
			t, _ := cmd.Flags().GetInt("timeout")
			cfg.Crawler.Timeout = t
		}

		var robotsCache *crawler.RobotsCache
		if !noRobots && !cfg.Crawler.NoRobots {
			robotsCache = crawler.NewRobotsCache(cfg.Crawler.UserAgent)
		}

		if recursive {
			// Persistent crawl mode (always).

			var (
				startURL       string
				validatorRules *crawler.ValidatorRules
			)

			// Generate a random job ID when none was given.
			if jobID == "" {
				var err error
				jobID, err = model.GenerateCrawlJobID()
				if err != nil {
					exit(1, "Failed to generate crawl job ID: "+err.Error())
				}
			}

			existingJob, err := model.GetCrawlJob(jobID)
			if err != nil {
				exit(1, "Failed to load crawl job: "+err.Error())
			}

			if existingJob == nil {
				// New job: require at least one URL.
				if len(args) == 0 {
					exit(1, "at least one URL is required to start a new crawl job")
				}
				startURL = args[0]

				maxDepth, _ := cmd.Flags().GetInt("max-depth")
				maxLinks, _ := cmd.Flags().GetInt("max-links")
				allowedDomains, _ := cmd.Flags().GetStringArray("allowed-domain")
				excludeDomains, _ := cmd.Flags().GetStringArray("exclude-domain")
				allowedPatterns, _ := cmd.Flags().GetStringArray("allowed-pattern")
				excludePatterns, _ := cmd.Flags().GetStringArray("exclude-pattern")

				validatorRules = &crawler.ValidatorRules{
					MaxDepth:        maxDepth,
					MaxLinks:        maxLinks,
					AllowedDomains:  allowedDomains,
					ExcludeDomains:  excludeDomains,
					AllowedPatterns: allowedPatterns,
					ExcludePatterns: excludePatterns,
				}

				rulesJSON, err := crawler.MarshalValidatorRules(validatorRules)
				if err != nil {
					exit(1, "Failed to serialize validator rules: "+err.Error())
				}
				if err := model.CreateCrawlJob(jobID, startURL, rulesJSON, label); err != nil {
					exit(1, "Failed to create crawl job: "+err.Error())
				}
				fmt.Println("Starting crawl job:", jobID)
			} else {
				// Resume existing job.
				startURL = existingJob.StartURL
				validatorRules, err = crawler.UnmarshalValidatorRules(existingJob.ValidatorRules)
				if err != nil {
					exit(1, "Failed to restore validator rules: "+err.Error())
				}
				// Use stored label unless --label was explicitly overridden.
				if !cmd.Flags().Changed("label") {
					label = existingJob.Label
				}
				fmt.Println("Resuming crawl job:", jobID)
			}

			validator, err := crawler.NewValidator(validatorRules)
			if err != nil {
				exit(1, "Invalid crawler rules: "+err.Error())
			}

			// Pre-seed visited counter from already-processed URLs.
			done, err := model.CountCrawlURLsByStatus(jobID, model.CrawlURLDone)
			if err != nil {
				exit(1, "Failed to count done URLs: "+err.Error())
			}
			failed, err := model.CountCrawlURLsByStatus(jobID, model.CrawlURLFailed)
			if err != nil {
				exit(1, "Failed to count failed URLs: "+err.Error())
			}
			validator.SetVisited(int(done + failed))

			cr, err := crawler.NewPersistent(&cfg.Crawler, jobID, robotsCache)
			if err != nil {
				exit(1, "Failed to initialize persistent crawler: "+err.Error())
			}
			defer func() {
				if err := cr.Close(); err != nil {
					log.Warn().Err(err).Msg("crawler close error")
				}
			}()

			if err := crawlAndIndex(startURL, cr, validator, force, label, clientOpts...); err != nil {
				exit(1, "Crawl failed: "+err.Error())
			}
			return
		}

		// Resume an existing job by ID without --recursive.
		if jobID != "" {
			existingJob, err := model.GetCrawlJob(jobID)
			if err != nil {
				exit(1, "Failed to load crawl job: "+err.Error())
			}
			if existingJob == nil {
				exit(1, "Crawl job not found: "+jobID+". Use --recursive to start a new job.")
			}

			validatorRules, err := crawler.UnmarshalValidatorRules(existingJob.ValidatorRules)
			if err != nil {
				exit(1, "Failed to restore validator rules: "+err.Error())
			}
			// Use stored label unless --label was explicitly overridden.
			if !cmd.Flags().Changed("label") {
				label = existingJob.Label
			}
			fmt.Println("Resuming crawl job:", jobID)

			validator, err := crawler.NewValidator(validatorRules)
			if err != nil {
				exit(1, "Invalid crawler rules: "+err.Error())
			}

			done, err := model.CountCrawlURLsByStatus(jobID, model.CrawlURLDone)
			if err != nil {
				exit(1, "Failed to count done URLs: "+err.Error())
			}
			failed, err := model.CountCrawlURLsByStatus(jobID, model.CrawlURLFailed)
			if err != nil {
				exit(1, "Failed to count failed URLs: "+err.Error())
			}
			validator.SetVisited(int(done + failed))

			cr, err := crawler.NewPersistent(&cfg.Crawler, jobID, robotsCache)
			if err != nil {
				exit(1, "Failed to initialize persistent crawler: "+err.Error())
			}
			defer func() {
				if err := cr.Close(); err != nil {
					log.Warn().Err(err).Msg("crawler close error")
				}
			}()

			if err := crawlAndIndex(existingJob.StartURL, cr, validator, force, label, clientOpts...); err != nil {
				exit(1, "Crawl failed: "+err.Error())
			}
			return
		}

		// Plain index mode (no crawling).
		if len(args) == 0 {
			exit(1, "at least one URL is required")
		}

		// Create the crawler once so the bidi backend reuses its
		// WebSocket connection and session across all URLs.
		cr, err := crawler.New(&cfg.Crawler, robotsCache)
		if err != nil {
			exit(1, "Failed to create crawler: "+err.Error())
		}
		defer func() {
			if err := cr.Close(); err != nil {
				log.Warn().Err(err).Msg("crawler close error")
			}
		}()

		c := newClient(clientOpts...)
		for _, u := range args {
			if !force {
				exists, err := c.DocumentExists(u)
				if err != nil {
					log.Warn().Err(err).Str("URL", u).Msg("Failed to check if URL is already indexed")
				} else if exists {
					log.Info().Str("URL", u).Msg("URL already indexed, skipping (use --force to reindex)")
					continue
				}
			}
			if err := indexURL(cr, u, label, clientOpts...); err != nil {
				log.Warn().Err(err).Str("URL", u).Msg("Failed to index URL")
			}
		}
	},
}

func init() {
	indexCmd.Flags().String("label", "", "Label to attach to all indexed documents")
	indexCmd.Flags().Bool("force", false, "Reindex URLs even if they are already in the index. Already indexed URLs are skipped otherwise")
	indexCmd.Flags().BoolP("recursive", "r", false, "Recursively crawl linked pages")
	indexCmd.Flags().Int("max-depth", 0, "Maximum crawl depth (0 = unlimited)")
	indexCmd.Flags().Int("max-links", 0, "Maximum number of pages to visit (0 = unlimited)")
	indexCmd.Flags().StringArray("allowed-domain", nil, "Domain to allow during crawl (repeatable; empty = all)")
	indexCmd.Flags().StringArray("exclude-domain", nil, "Domain to exclude during crawl (repeatable)")
	indexCmd.Flags().StringArray("allowed-pattern", nil, "Regexp pattern URLs must match to be followed (repeatable; empty = all)")
	indexCmd.Flags().StringArray("exclude-pattern", nil, "Regexp pattern; matching URLs are skipped (repeatable)")
	indexCmd.Flags().Bool("global", false, "Make indexed documents available for all users (only for admins in multiuser mode)")
	indexCmd.Flags().Uint("user-id", 0, "Index documents under the given user ID (only for admins in multiuser mode)")
	indexCmd.Flags().String("job-id", "", "Persistent crawl job ID; use with --recursive to start a new job or alone to resume an existing one")
	indexCmd.Flags().String("backend", "", "Crawler backend to use (\"http\", \"chromedp\", or \"bidi\")")
	indexCmd.Flags().StringToString("backend-option", nil, "Crawler backend option as key=value (repeatable, e.g. --backend-option exec_path=/usr/bin/chromium)")
	indexCmd.Flags().StringToString("header", nil, "Extra HTTP header as KEY=VALUE (repeatable, e.g. --header Accept-Language=en)")
	indexCmd.Flags().StringArray("cookie", nil, "HTTP cookie as Set-Cookie value (repeatable, e.g. --cookie \"session=abc; Domain=example.com\")")
	indexCmd.Flags().Bool("no-robots", false, "Disable robots.txt compliance during crawling")
	indexCmd.Flags().Int("delay", 0, "Delay in seconds between requests (0 = no delay; overrides config)")
	indexCmd.Flags().Int("timeout", 0, "Request timeout in seconds (0 = 5s default; overrides config)")
	indexCmd.Flags().String("user-agent", "", "User-agent string for requests (overrides config)")
	indexCmd.Flags().Bool("allow-sensitive", false, "Skip sensitive content checks allowing sensitive content being indexed.")
}

var deleteCmd = &cobra.Command{
	Use:   "delete QUERY",
	Short: "Remove documents from the index",
	Long: `Remove documents from the index using the search query language.

The QUERY syntax is the same as the search queries.

Examples:
  hister delete "url:https://example.com/page"
  hister delete "url:file:///home/user/file.pdf"
  hister delete "domain:example.com"
  hister delete "language:en domain:example.com"

Non-admin users are restricted to their own documents by the server.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		c := newClient()
		dry, _ := cmd.Flags().GetBool("dry")
		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose {
			var (
				pageKey string
				total   uint64
			)
			for {
				res, err := c.Search(&indexer.Query{Text: args[0], PageKey: pageKey, Sort: "domain"})
				if err != nil {
					exit(1, "Failed to search: "+err.Error())
				}
				if total == 0 {
					total = res.Total
				}
				for _, doc := range res.Documents {
					fmt.Println(doc.URL)
				}
				if res.PageKey == "" || len(res.Documents) == 0 {
					break
				}
				pageKey = res.PageKey
			}
			if dry {
				fmt.Printf("%d document(s) would be deleted\n", total)
			} else {
				fmt.Printf("Deleting %d document(s)\n", total)
			}
			return
		}
		if dry {
			res, err := c.Search(&indexer.Query{Text: args[0]})
			if err != nil {
				exit(1, "Failed to search: "+err.Error())
			}
			fmt.Printf("%d document(s) would be deleted\n", res.Total)
			return
		}
		if err := c.DeleteDocuments(args[0]); err != nil {
			exit(1, "Failed to delete: "+err.Error())
		}
	},
}

var createUserCmd = &cobra.Command{
	Use:    "create-user USERNAME",
	Short:  "Create a new user",
	Long:   "Create a new user account (requires user_handling to be enabled)",
	Args:   cobra.ExactArgs(1),
	PreRun: requireUserHandlingAndInitDB,
	Run: func(cmd *cobra.Command, args []string) {
		username := args[0]
		password, err := promptPassword("Password: ")
		if err != nil {
			exit(1, "Failed to read password: "+err.Error())
		}
		if len(password) < 8 {
			exit(1, "password must be at least 8 characters long")
		}
		confirm, err := promptPassword("Confirm password: ")
		if err != nil {
			exit(1, "Failed to read password: "+err.Error())
		}
		if password != confirm {
			exit(1, "passwords do not match")
		}
		isAdmin, _ := cmd.Flags().GetBool("admin")
		if _, err := model.CreateUser(username, password, isAdmin); err != nil {
			exit(1, "Failed to create user: "+err.Error())
		}
		fmt.Println(cliSuccessStyle.Render("✓") + " User created: " + cliInfoStyle.Render(username))
	},
}

var deleteUserCmd = &cobra.Command{
	Use:    "delete-user USERNAME",
	Short:  "Delete a user",
	Long:   "Delete a user account (requires user_handling to be enabled). Use --purge to also remove all indexed documents belonging to the user.",
	Args:   cobra.ExactArgs(1),
	PreRun: requireUserHandlingAndInitDB,
	Run: func(cmd *cobra.Command, args []string) {
		username := args[0]
		u, err := model.GetUser(username)
		if err != nil {
			exit(1, "Failed to get user: "+err.Error())
		}
		c := newClient()
		q := fmt.Sprintf("user_id:%d", u.ID)
		res, err := c.Search(&indexer.Query{Text: q})
		if err != nil {
			exit(1, "Failed to check user documents: "+err.Error())
		}
		if res.Total > 0 {
			purge, _ := cmd.Flags().GetBool("purge")
			if !purge {
				exit(1, fmt.Sprintf("User %q has %d indexed document(s). Use --purge to delete them along with the user.", username, res.Total))
			}
			if err := c.DeleteDocuments(q); err != nil {
				exit(1, "Failed to purge user documents: "+err.Error())
			}
			fmt.Printf("%s Purged %d document(s) for user %s\n", cliSuccessStyle.Render("✓"), res.Total, cliInfoStyle.Render(username))
		}
		if err := model.DeleteUser(username); err != nil {
			exit(1, "Failed to delete user: "+err.Error())
		}
		fmt.Println(cliSuccessStyle.Render("✓") + " User deleted: " + cliInfoStyle.Render(username))
	},
}

var showUserCmd = &cobra.Command{
	Use:    "show-user USERNAME",
	Short:  "Show user information",
	Long:   "Display information about a user account (requires user_handling to be enabled)",
	Args:   cobra.ExactArgs(1),
	PreRun: requireUserHandlingAndInitDB,
	Run: func(cmd *cobra.Command, args []string) {
		u, err := model.GetUser(args[0])
		if err != nil {
			exit(1, "Failed to get user: "+err.Error())
		}
		admin := "no"
		if u.IsAdmin {
			admin = "yes"
		}
		fmt.Println(cliInfoStyle.Render("Username:   ") + u.Username)
		fmt.Println(cliInfoStyle.Render("ID:         ") + fmt.Sprintf("%d", u.ID))
		fmt.Println(cliInfoStyle.Render("Admin:      ") + admin)
		if showToken, _ := cmd.Flags().GetBool("token"); showToken {
			fmt.Println(cliInfoStyle.Render("Token:      ") + u.Token)
		}
		fmt.Println(cliInfoStyle.Render("Created at: ") + u.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Println(cliInfoStyle.Render("Updated at: ") + u.UpdatedAt.Format("2006-01-02 15:04:05"))
	},
}

var updateUserCmd = &cobra.Command{
	Use:    "update-user USERNAME",
	Short:  "Update a user",
	Long:   "Update a user account (requires user_handling to be enabled). Use flags to change username, regenerate token, or toggle admin status.",
	Args:   cobra.ExactArgs(1),
	PreRun: requireUserHandlingAndInitDB,
	Run: func(cmd *cobra.Command, args []string) {
		username := args[0]
		changed := false

		if newUsername, _ := cmd.Flags().GetString("username"); newUsername != "" {
			if err := model.UpdateUsername(username, newUsername); err != nil {
				exit(1, "Failed to update username: "+err.Error())
			}
			fmt.Println(cliSuccessStyle.Render("✓") + " Username changed: " + cliInfoStyle.Render(username) + " → " + cliInfoStyle.Render(newUsername))
			username = newUsername
			changed = true
		}

		if regen, _ := cmd.Flags().GetBool("regen-token"); regen {
			token, err := model.RegenerateTokenByUsername(username)
			if err != nil {
				exit(1, "Failed to regenerate token: "+err.Error())
			}
			fmt.Println(cliSuccessStyle.Render("✓") + " New token for " + cliInfoStyle.Render(username) + ": " + cliInfoStyle.Render(token))
			changed = true
		}

		if toggle, _ := cmd.Flags().GetBool("toggle-admin"); toggle {
			isAdmin, err := model.ToggleAdmin(username)
			if err != nil {
				exit(1, "Failed to toggle admin: "+err.Error())
			}
			status := "disabled"
			if isAdmin {
				status = "enabled"
			}
			fmt.Println(cliSuccessStyle.Render("✓") + " Admin " + status + " for " + cliInfoStyle.Render(username))
			changed = true
		}

		if !changed {
			exit(1, "no changes specified - use --username, --regen-token, or --toggle-admin")
		}
	},
}

var crawlCmd = &cobra.Command{
	Use:   "crawl",
	Short: "Manage persistent crawl jobs",
	Long:  "Manage persistent crawl jobs",
}

var crawlListCmd = &cobra.Command{
	Use:   "list",
	Short: "List persistent crawl jobs",
	Long:  "Display all persistent crawl jobs with their status and URL counts",
	Args:  cobra.NoArgs,
	PreRun: func(_ *cobra.Command, _ []string) {
		initDB()
	},
	Run: func(cmd *cobra.Command, args []string) {
		jobs, err := model.ListCrawlJobs()
		if err != nil {
			exit(1, "Failed to list crawl jobs: "+err.Error())
		}
		if len(jobs) == 0 {
			fmt.Println("No crawl jobs found.")
			return
		}
		for _, j := range jobs {
			stats, err := model.GetCrawlJobStats(j.ID)
			if err != nil {
				log.Warn().Err(err).Str("job_id", j.ID).Msg("failed to get job stats")
			}
			fmt.Printf("%s  %-12s  %s\n",
				cliInfoStyle.Render(j.ID),
				j.Status,
				j.StartURL,
			)
			fmt.Printf("  pending: %d  done: %d  failed: %d  skipped: %d  created: %s\n",
				stats.Pending, stats.Done, stats.Failed, stats.Skipped,
				j.CreatedAt.Format("2006-01-02 15:04:05"),
			)
		}
	},
}

var crawlDeleteCmd = &cobra.Command{
	Use:   "delete JOB_ID",
	Short: "Delete a persistent crawl job",
	Long:  "Delete a crawl job and all its associated URL tracking data",
	Args:  cobra.ExactArgs(1),
	PreRun: func(_ *cobra.Command, _ []string) {
		initDB()
	},
	Run: func(cmd *cobra.Command, args []string) {
		jobID := args[0]
		if err := model.DeleteCrawlJob(jobID); err != nil {
			exit(1, "Failed to delete crawl job: "+err.Error())
		}
		fmt.Println(cliSuccessStyle.Render("✓") + " Crawl job deleted: " + cliInfoStyle.Render(jobID))
	},
}

var exportCmd = &cobra.Command{
	Use:   "export OUTPUT_FILE [QUERY...]",
	Short: "Export indexed documents to a JSON file",
	Long: `Export all indexed documents, or only those matching a search query, to a JSON file.

Each document is written as a single JSON line. Lines not starting with '{' are
structural markers ('[', ']', ',') and can be safely skipped by parsers.

Use --start-date and --end-date (format: YYYY-MM-DD) to only export
documents added within the given date range.

Use '-' as OUTPUT_FILE to write to stdout.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		outputFile := args[0]
		queryStr := strings.Join(args[1:], " ")
		if queryStr == "" {
			queryStr = "*"
		}

		dateRange, err := parseDateRangeFlags(cmd)
		if err != nil {
			exit(1, err.Error())
		}

		var out *os.File
		if outputFile == "-" {
			out = os.Stdout
		} else {
			f, err := os.Create(outputFile)
			if err != nil {
				exit(1, "Failed to create output file: "+err.Error())
			}
			defer func() {
				if err := f.Close(); err != nil {
					log.Error().Err(err).Msg("Failed to close output file")
				}
			}()
			out = f
		}

		bw := bufio.NewWriter(out)
		defer func() {
			if err := bw.Flush(); err != nil {
				log.Error().Err(err).Msg("Failed to flush output")
			}
		}()

		if _, err := fmt.Fprintln(bw, "["); err != nil {
			exit(1, "Write error: "+err.Error())
		}

		c := newClient(client.WithTimeout(0))
		first := true
		count := 0
		pageKey := ""
		for {
			res, err := c.Search(&indexer.Query{
				Text:        queryStr,
				PageKey:     pageKey,
				IncludeHTML: true,
				IncludeText: true,
				DateFrom:    dateRange.From,
				DateTo:      dateRange.To,
			})
			if err != nil {
				exit(1, "Search failed: "+err.Error())
			}
			for _, d := range res.Documents {
				b, merr := json.Marshal(d)
				if merr != nil {
					log.Warn().Err(merr).Str("url", d.URL).Msg("Failed to serialize document, skipping")
					continue
				}
				if !first {
					if _, werr := fmt.Fprintln(bw, ","); werr != nil {
						exit(1, "Write error: "+werr.Error())
					}
				}
				first = false
				if _, werr := bw.Write(b); werr != nil {
					exit(1, "Write error: "+werr.Error())
				}
				if _, werr := fmt.Fprintln(bw); werr != nil {
					exit(1, "Write error: "+werr.Error())
				}
				count++
			}
			if res.PageKey == "" || len(res.Documents) == 0 {
				break
			}
			pageKey = res.PageKey
		}

		if _, err := fmt.Fprintln(bw, "]"); err != nil {
			exit(1, "Write error: "+err.Error())
		}

		if outputFile != "-" {
			fmt.Printf("%s Exported %d document(s) to %s\n",
				cliSuccessStyle.Render("✓"), count, cliInfoStyle.Render(outputFile))
		}
	},
}

var importCmd = &cobra.Command{
	Use:   "import INPUT_FILE [INPUT_FILE...]",
	Short: "Import documents from export JSON or HTML files",
	Long: `Import documents from one or more files previously created by the export
command.

JSON files are read line by line; each line starting with '{' is parsed as a
document and submitted to the running server. Content is re-processed
server-side from the stored HTML.

An input file may be a plain JSON file or a 7z-compressed archive (.7z)
containing a single JSON file.

HTML files (.html or .htm) can also be imported: the URL is extracted from
the HTML (canonical link, OpenGraph/Twitter meta tags, etc.) and the document
is submitted to the running server for processing.

Multiple files may be given; they are imported in order and the result is
reported as a combined total.

Use --start-date and --end-date (format: YYYY-MM-DD) to only import
documents whose "added" timestamp falls within the given date range.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		skip, _ := cmd.Flags().GetBool("skip-existing")

		dateRange, err := parseDateRangeFlags(cmd)
		if err != nil {
			exit(1, err.Error())
		}

		c := newClient(client.WithTimeout(0))
		imported := 0
		skipped := 0
		errCount := 0

		for _, inputFile := range args {
			var i, s, e int
			if ext := strings.ToLower(filepath.Ext(inputFile)); ext == ".html" || ext == ".htm" {
				i, s, e = importHTMLFile(c, inputFile, skip)
			} else {
				i, s, e = importJSONFile(c, inputFile, skip, dateRange.From, dateRange.To)
			}
			imported += i
			skipped += s
			errCount += e
		}

		msg := fmt.Sprintf("%s Imported %d document(s)", cliSuccessStyle.Render("✓"), imported)
		if skipped > 0 {
			msg += fmt.Sprintf(" (%d skipped)", skipped)
		}
		if errCount > 0 {
			msg += fmt.Sprintf(" (%d errors)", errCount)
		}
		fmt.Println(msg)
	},
}

// importJSONFile imports documents from a JSON export file (optionally a
// 7z-compressed archive) and submits them to the running server. It returns
// the number of documents imported, skipped and failed.
func importJSONFile(c *client.Client, inputFile string, skip bool, startDate, endDate int64) (imported, skipped, errCount int) {
	var reader io.Reader

	if strings.HasSuffix(strings.ToLower(inputFile), ".7z") {
		sz, err := sevenzip.OpenReader(inputFile)
		if err != nil {
			log.Warn().Err(err).Str("file", inputFile).Msg("Failed to open 7z archive, skipping")
			return 0, 0, 1
		}
		defer func() {
			if err := sz.Close(); err != nil {
				log.Error().Err(err).Msg("Failed to close 7z archive")
			}
		}()

		var jsonEntry *sevenzip.File
		for _, entry := range sz.File {
			if strings.HasSuffix(strings.ToLower(entry.Name), ".json") {
				jsonEntry = entry
				break
			}
		}
		if jsonEntry == nil {
			log.Warn().Str("file", inputFile).Msg("No JSON file found inside 7z archive, skipping")
			return 0, 0, 1
		}
		rc, err := jsonEntry.Open()
		if err != nil {
			log.Warn().Err(err).Str("file", inputFile).Msg("Failed to open JSON entry in 7z archive, skipping")
			return 0, 0, 1
		}
		defer func() {
			if err := rc.Close(); err != nil {
				log.Error().Err(err).Msg("Failed to close 7z entry reader")
			}
		}()
		reader = rc
	} else {
		f, err := os.Open(inputFile)
		if err != nil {
			log.Warn().Err(err).Str("file", inputFile).Msg("Failed to open input file, skipping")
			return 0, 0, 1
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Error().Err(err).Msg("Failed to close input file")
			}
		}()
		reader = f
	}

	const maxLineSize = 16 * 1024 * 1024 // 16 MB covers large HTML+favicon lines
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var d document.Document
		if err := json.Unmarshal(line, &d); err != nil {
			log.Warn().Err(err).Msg("Failed to parse document line, skipping")
			errCount++
			continue
		}
		if (startDate != 0 && d.Added < startDate) || (endDate != 0 && d.Added > endDate) {
			log.Debug().Str("url", d.URL).Int64("added", d.Added).Msg("Skipping document outside of date range")
			skipped++
			continue
		}
		if skip {
			exists, err := c.DocumentExists(d.URL)
			if err != nil {
				log.Warn().Err(err).Str("url", d.URL).Msg("Failed to check if document exists, skipping")
				errCount++
				continue
			}
			if exists {
				log.Debug().Str("url", d.URL).Msg("Document already exists, skipping")
				skipped++
				continue
			}
		}
		if err := c.AddDocumentJSON(&d); err != nil {
			log.Warn().Err(err).Str("url", d.URL).Msg("Failed to add document")
			errCount++
			continue
		}
		imported++
	}

	if err := scanner.Err(); err != nil {
		log.Warn().Err(err).Str("file", inputFile).Msg("Failed to read input file")
		errCount++
	}

	return imported, skipped, errCount
}

// importHTMLFile reads a single HTML file, builds a document from it by
// extracting the URL from the HTML, and submits it to the running server. It
// returns the number of documents imported, skipped and failed.
func importHTMLFile(c *client.Client, inputFile string, skip bool) (imported, skipped, errCount int) {
	data, err := os.ReadFile(inputFile)
	if err != nil {
		log.Warn().Err(err).Str("file", inputFile).Msg("Failed to read HTML file, skipping")
		return 0, 0, 1
	}

	d, err := document.FromHTML(string(data))
	if err != nil {
		log.Warn().Err(err).Str("file", inputFile).Msg("Failed to import HTML file, skipping")
		return 0, 0, 1
	}

	if skip {
		exists, err := c.DocumentExists(d.URL)
		if err != nil {
			log.Warn().Err(err).Str("url", d.URL).Msg("Failed to check if document exists, skipping")
			return 0, 0, 1
		}
		if exists {
			log.Debug().Str("url", d.URL).Msg("Document already exists, skipping")
			return 0, 1, 0
		}
	}

	if err := c.AddDocumentJSON(d); err != nil {
		log.Warn().Err(err).Str("url", d.URL).Msg("Failed to add document")
		return 0, 0, 1
	}

	return 1, 0, 0
}

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Reindex",
	Long:  `Recreate index`,
	Run: func(cmd *cobra.Command, args []string) {
		skipSensitive := false
		if b, err := cmd.Flags().GetBool("exclude-sensitive"); err == nil {
			skipSensitive = b
		}
		c := newClient(client.WithTimeout(0))
		if err := c.Reindex(skipSensitive, cfg.Indexer.DetectLanguages); err != nil {
			msg := "Reindex error: " + err.Error()
			if isConnectionError(err) {
				msg += "\n  Make sure the Hister server is running before executing reindex."
			}
			exit(1, msg)
		}
	},
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove orphaned data files",
	Long:  `Remove HTML and favicon files from the data directories that are no longer referenced by any document in the index`,
	Run: func(_ *cobra.Command, _ []string) {
		c := newClient(client.WithTimeout(0))
		result, err := c.Cleanup()
		if err != nil {
			msg := "Cleanup error: " + err.Error()
			if isConnectionError(err) {
				msg += "\n  Make sure the Hister server is running before executing cleanup."
			}
			exit(1, msg)
		}
		fmt.Printf("Removed %d orphaned HTML file(s)\n", result.HTMLRemoved)
		fmt.Printf("Removed %d orphaned favicon file(s)\n", result.FaviconRemoved)
	},
}

func exit(errno int, msg string) {
	if errno != 0 {
		fmt.Println(cliErrorStyle.Render("Error!") + " " + msg)
	} else {
		fmt.Println(msg)
	}
	os.Exit(errno)
}

func isConnectionError(err error) bool {
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

type dateRangeFlags struct {
	From int64
	To   int64
}

func parseDateRangeFlags(cmd *cobra.Command) (dateRangeFlags, error) {
	var r dateRangeFlags
	if v, _ := cmd.Flags().GetString("start-date"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return r, fmt.Errorf("Invalid --start-date: %w", err)
		}
		r.From = t.Unix()
	}
	if v, _ := cmd.Flags().GetString("end-date"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return r, fmt.Errorf("Invalid --end-date: %w", err)
		}
		r.To = t.AddDate(0, 0, 1).Unix() - 1
	}
	return r, nil
}

func requireUserHandlingAndInitDB(_ *cobra.Command, _ []string) {
	if !cfg.App.UserHandling {
		exit(1, "user_handling is not enabled in configuration")
	}
	initDB()
}

func init() {
	dcfg := config.CreateDefaultConfig()
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "config.yml", "config file (default paths: ./config.yml or $HOME/.histerrc or $HOME/.config/hister/config.yml)")
	rootCmd.PersistentFlags().StringP("log-level", "l", "info", "set log level (possible options: error, warning, info, debug, trace)")
	rootCmd.PersistentFlags().StringP("search-url", "s", dcfg.App.SearchURL, "set default search engine url")
	rootCmd.PersistentFlags().StringP("server-url", "u", dcfg.Server.BaseURL, "hister server URL")
	rootCmd.PersistentFlags().StringP("token", "t", "", "access token (overrides config access_token)")
	rootCmd.PersistentFlags().Int("client-timeout", 0, "HTTP client timeout in seconds for server communication (0 = no timeout; default if unset: 10s)")

	rootCmd.AddCommand(listenCmd)
	rootCmd.AddCommand(createConfigCmd)
	rootCmd.AddCommand(listURLsCmd)
	rootCmd.AddCommand(listFilesCmd)
	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(browserImportCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(reindexCmd)
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(createUserCmd)
	rootCmd.AddCommand(deleteUserCmd)
	rootCmd.AddCommand(showUserCmd)
	rootCmd.AddCommand(updateUserCmd)
	rootCmd.AddCommand(crawlCmd)
	crawlCmd.AddCommand(crawlListCmd)
	crawlCmd.AddCommand(crawlDeleteCmd)

	listenCmd.Flags().StringP("address", "a", dcfg.Server.Address, "Listen address")

	listURLsCmd.Flags().Bool("offline", false, "connect to the indexer directly without using the HTTP API (server should be stopped)")

	browserImportCmd.Flags().String("backend", "", "Crawler backend to use (\"http\", \"chromedp\", or \"bidi\")")

	importCmd.Flags().IntP("min-visit", "m", 1, "only import URLs that were opened at least 'min-visit' times")
	importCmd.Flags().String("backend", "", "Crawler backend to use (\"http\", \"chromedp\", or \"bidi\")")
	importCmd.Flags().StringToString("backend-option", nil, "Crawler backend option as key=value (repeatable, e.g. --backend-option exec_path=/usr/bin/chromium)")
	importCmd.Flags().StringToString("header", nil, "Extra HTTP header as KEY=VALUE (repeatable, e.g. --header Accept-Language=en)")
	importCmd.Flags().StringArray("cookie", nil, "HTTP cookie as Set-Cookie value (repeatable, e.g. --cookie \"session=abc; Domain=example.com\")")
	importCmd.Flags().String("start-date", "", "only import documents added on or after this date (YYYY-MM-DD)")
	importCmd.Flags().String("end-date", "", "only import documents added on or before this date (YYYY-MM-DD)")

	exportCmd.Flags().String("start-date", "", "only export documents added on or after this date (YYYY-MM-DD)")
	exportCmd.Flags().String("end-date", "", "only export documents added on or before this date (YYYY-MM-DD)")

	createUserCmd.Flags().Bool("admin", false, "create user with admin privileges")

	updateUserCmd.Flags().String("username", "", "new username")
	updateUserCmd.Flags().Bool("regen-token", false, "regenerate access token")
	updateUserCmd.Flags().Bool("toggle-admin", false, "toggle admin status")

	deleteCmd.Flags().Bool("dry", false, "display the number of documents that would be deleted without actually deleting them")
	deleteCmd.Flags().BoolP("verbose", "v", false, "list all URLs that would be deleted before performing the deletion. Can be used with --dry")

	deleteUserCmd.Flags().Bool("purge", false, "also delete all indexed documents belonging to the user")

	showUserCmd.Flags().Bool("token", false, "display the user's access token")

	importCmd.Flags().Bool("skip-existing", false, "Do not overwrite documents that are already in the index")

	reindexCmd.Flags().BoolP("exclude-sensitive", "x", false, "don't add documents that contain sensitive content matched by config.SensitiveContentPatterns")

	searchCmd.Flags().StringP("format", "f", "text", "output format: text, json, csv")
	searchCmd.Flags().StringP("fields", "F", "", "comma-separated list of document fields to display (id, url, title, domain, score, added, language, type, text, favicon, user_id, html)")
	searchCmd.Flags().IntP("limit", "L", 0, "maximum number of results to display (0 means no limit)")

	cobra.OnInitialize(initialize)

	zerolog.CallerMarshalFunc = func(_ uintptr, file string, line int) string {
		dir, fn := filepath.Split(file)
		if dir == "" {
			return fn + ":" + strconv.Itoa(line)
		}
		_, subdir := filepath.Split(strings.TrimSuffix(dir, "/"))
		return subdir + "/" + fn + ":" + strconv.Itoa(line)
	}
	log.Logger = log.With().Caller().Logger()
	log.Logger = log.Output(newConsoleWriter(os.Stderr, false))
}

func newConsoleWriter(out io.Writer, noColor bool) zerolog.ConsoleWriter {
	return zerolog.ConsoleWriter{
		Out:     out,
		NoColor: noColor,
		FormatTimestamp: func(i any) string {
			return i.(string)
		},
		FormatLevel: func(i any) string {
			level := strings.ToUpper(fmt.Sprintf("%-6s", i))
			if noColor {
				return fmt.Sprintf("| %s |", level)
			}
			var color lipgloss.Color
			switch i {
			case "trace":
				color = lipgloss.Color("240") // dark gray
			case "debug":
				color = lipgloss.Color("12") // bright blue
			case "info":
				color = lipgloss.Color("10") // bright green
			case "warn", "warning":
				color = lipgloss.Color("11") // bright yellow
			case "error":
				color = lipgloss.Color("9") // bright red
			case "fatal", "panic":
				color = lipgloss.Color("196") // bold red
			default:
				color = lipgloss.Color("15") // white
			}
			return fmt.Sprintf("| %s |", lipgloss.NewStyle().Foreground(color).Bold(true).Render(level))
		},
	}
}

func initialize() {
	if ll := os.Getenv("HISTER__APP__LOG_LEVEL"); ll != "debug" {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	initConfig()
	if cfg.Crawler.UserAgent != "" {
		UserAgent = cfg.Crawler.UserAgent
	}
	initLog()
	log.Debug().Str("filename", cfg.Filename()).Msg("Config initialization complete")
	log.Debug().Msg("Logging initialization complete")
}

func initConfig() {
	var err error

	if !rootCmd.PersistentFlags().Changed("config") {
		if envConfig := os.Getenv("HISTER_CONFIG"); envConfig != "" {
			cfgFile = envConfig
		}
	}

	cfg, err = config.Load(cfgFile)
	if err != nil {
		exit(1, "Failed to initialize config: "+err.Error())
	}

	if v, _ := rootCmd.PersistentFlags().GetString("log-level"); v != "" && (rootCmd.Flags().Changed("log-level") || cfg.App.LogLevel == "") {
		cfg.App.LogLevel = v
	}
	if v, _ := rootCmd.PersistentFlags().GetString("search-url"); v != "" && (rootCmd.Flags().Changed("search-url") || cfg.App.SearchURL == "") {
		cfg.App.SearchURL = v
	}
	if v, _ := rootCmd.PersistentFlags().GetString("server-url"); v != "" && rootCmd.Flags().Changed("server-url") {
		if err := cfg.UpdateBaseURL(v); err != nil {
			exit(1, "Failed to initialize config: "+err.Error())
		}
	}
	if v, _ := rootCmd.PersistentFlags().GetString("token"); rootCmd.PersistentFlags().Changed("token") {
		cfg.App.AccessToken = v
	}
}

func initLog() {
	switch cfg.App.LogLevel {
	case "error", "err":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "warning", "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		log.Warn().Str("Invalid config log level", cfg.App.LogLevel)
	}

	var out io.Writer = os.Stderr
	noColor := false
	if cfg.App.LogFile != "" {
		f, err := os.OpenFile(cfg.App.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
		if err != nil {
			log.Error().Err(err).Str("log_file", cfg.App.LogFile).Msg("Failed to open log file, falling back to stderr")
		} else {
			out = f
			noColor = true
		}
	}

	switch cfg.App.LogFormat {
	case "json":
		log.Logger = log.Logger.Output(out)
	case "text", "":
		log.Logger = log.Logger.Output(newConsoleWriter(out, noColor))
	default:
		log.Warn().Str("log_format", cfg.App.LogFormat).Msg("Invalid log format, using text")
		log.Logger = log.Logger.Output(newConsoleWriter(out, noColor))
	}
}

func initDB() {
	err := model.Init(cfg)
	if err != nil {
		exit(1, err.Error())
	}
	log.Debug().Msg("Database initialization complete")
}

func initExtractor() {
	if err := extractor.Init(cfg.Extractors); err != nil {
		exit(1, "Extractor initialization error: "+err.Error())
	}
}

func initIndex() {
	initDB()
	initExtractor()
	if err := indexer.Init(cfg); err != nil {
		exit(1, "Indexer initialization error: "+err.Error())
	}
	v, err := model.GetIndexerVersion()
	if err != nil {
		exit(1, "Failed to retrieve indexer version: "+err.Error())
	}
	if v == -1 {
		// Fresh installation — record current version, no reindex needed.
		if err := model.SetIndexerVersion(indexer.Version); err != nil {
			exit(1, "Failed to set indexer version: "+err.Error())
		}
	} else if indexer.Version > v {
		log.Warn().Msg(cliWarningStyle.Render("There is a new indexer version. Run `hister reindex` to update your index."))
	}
	log.Debug().Msg("Indexer initialization complete")
}

type passwordModel struct {
	input textinput.Model
	done  bool
	err   error
}

func (m passwordModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m passwordModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEsc:
			m.err = errors.New("cancelled")
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m passwordModel) View() string {
	if m.done || m.err != nil {
		return ""
	}
	return m.input.View() + "\n"
}

func promptPassword(prompt string) (string, error) {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '*'
	ti.Prompt = prompt
	ti.Focus()

	m := passwordModel{input: ti}
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return "", err
	}
	final := result.(passwordModel)
	if final.err != nil {
		return "", final.err
	}
	return final.input.Value(), nil
}

//func yesNoPrompt(label string, def bool) bool {
//	choices := "Y/n"
//	if !def {
//		choices = "y/N"
//	}
//
//	prompt := fmt.Appendf(nil, "%s [%s] ", label, choices)
//	r := bufio.NewReader(os.Stdin)
//	var s string
//
//	for {
//		if _, err := os.Stderr.Write(prompt); err != nil {
//			return def
//		}
//		s, _ = r.ReadString('\n')
//		s = strings.TrimSpace(s)
//		if s == "" {
//			return def
//		}
//		s = strings.ToLower(s)
//		if s == "y" || s == "yes" {
//			return true
//		}
//		if s == "n" || s == "no" {
//			return false
//		}
//	}
//}

//func stringPrompt(label string) string {
//	var s string
//	r := bufio.NewReader(os.Stdin)
//	for {
//		fmt.Fprint(os.Stderr, label+" ")
//		s, _ = r.ReadString('\n')
//		if s != "" {
//			break
//		}
//	}
//	return strings.TrimSpace(s)
//}
//
//func intPrompt(label string, def int64) int64 {
//	var s string
//	r := bufio.NewReader(os.Stdin)
//	prompt := fmt.Sprintf("%s [%d] ", label, def)
//	for {
//		fmt.Fprint(os.Stderr, prompt)
//		s, _ = r.ReadString('\n')
//		s = strings.TrimSpace(s)
//		if s == "" {
//			return def
//		}
//		i, err := strconv.ParseInt("12345", 10, 64)
//		if err != nil {
//			log.Error().Err(err).Msg("Invalid integer")
//		} else {
//			return i
//		}
//	}
//}
//
//func choicePrompt(label string, choices []string) string {
//	prompt := []byte(fmt.Sprintf("%s [%s,%s] ", label, strings.ToUpper(choices[0]), strings.Join(choices[1:], ",")))
//
//	r := bufio.NewReader(os.Stdin)
//	var s string
//
//	for {
//		_, _ = os.Stderr.Write(prompt)
//		s, _ = r.ReadString('\n')
//		s = strings.TrimSpace(s)
//		if s == "" {
//			return choices[0]
//		}
//		s = strings.ToLower(s)
//		if slices.Contains(choices, s) {
//			return s
//		}
//	}
//}

func indexURL(cr crawler.Crawler, u string, label string, clientOpts ...client.Option) error {
	if u == "" {
		log.Warn().Msg("URL must not be empty")
		return nil
	}
	v, err := crawler.NewValidator(&crawler.ValidatorRules{MaxLinks: 1})
	if err != nil {
		return fmt.Errorf("failed to create validator: %w", err)
	}
	ch, err := cr.Crawl(context.Background(), u, v)
	if err != nil {
		return fmt.Errorf("failed to fetch %s: %w", u, err)
	}
	d, ok := <-ch
	if !ok {
		return fmt.Errorf("failed to fetch %s: no response", u)
	}
	if err := d.Process(nil, extractor.Extract); err != nil {
		return fmt.Errorf("failed to process document: %w", err)
	}
	if d.Favicon == "" {
		if err := d.DownloadFavicon(UserAgent); err != nil {
			log.Debug().Err(err).Str("URL", d.URL).Msg("failed to download favicon")
		}
	}
	d.Label = label
	c := newClient(clientOpts...)
	if err := c.AddDocumentJSON(d); err != nil {
		return fmt.Errorf("failed to send page to hister: %w", err)
	}
	return nil
}

func crawlAndIndex(startURL string, cr crawler.Crawler, v *crawler.Validator, force bool, label string, clientOpts ...client.Option) error {
	ch, err := cr.Crawl(context.Background(), startURL, v)
	if err != nil {
		return err
	}
	c := newClient(clientOpts...)
	for doc := range ch {
		if !force {
			exists, err := c.DocumentExists(doc.URL)
			if err != nil {
				log.Warn().Err(err).Str("url", doc.URL).Msg("failed to check if URL is already indexed")
			} else if exists {
				log.Info().Str("url", doc.URL).Msg("URL already indexed, skipping (use --force to reindex)")
				continue
			}
		}
		if err := doc.Process(nil, extractor.Extract); err != nil {
			log.Warn().Err(err).Str("url", doc.URL).Msg("failed to process crawled document")
			continue
		}
		if doc.Favicon == "" {
			if err := doc.DownloadFavicon(UserAgent); err != nil {
				log.Debug().Err(err).Str("url", doc.URL).Msg("failed to download favicon")
			}
		}
		doc.Label = label
		if err := c.AddDocumentJSON(doc); err != nil {
			log.Warn().Err(err).Str("url", doc.URL).Msg("failed to index crawled document")
		}
	}
	return nil
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

func newClient(extraOpts ...client.Option) *client.Client {
	opts := []client.Option{client.WithUserAgent(UserAgent)}
	if cfg.App.AccessToken != "" {
		opts = append(opts, client.WithAccessToken(cfg.App.AccessToken))
	}
	if rootCmd.PersistentFlags().Changed("client-timeout") {
		t, _ := rootCmd.PersistentFlags().GetInt("client-timeout")
		opts = append(opts, client.WithTimeout(time.Duration(t)*time.Second))
	}
	opts = append(opts, extraOpts...)
	return client.New(cfg.BaseURL(""), opts...)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// browserTableName returns the SQL table name for the given browser, or an
// empty string for unrecognized browsers.
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
