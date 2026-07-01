package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/asciimoo/hister/client"
	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/files"
	"github.com/asciimoo/hister/server"
	"github.com/asciimoo/hister/server/extractor"
	"github.com/asciimoo/hister/server/indexer"
	"github.com/asciimoo/hister/server/model"

	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
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

func targetUserIDClientOptions(cmd *cobra.Command, global bool) []client.Option {
	targetUserID, _ := cmd.Flags().GetUint("user-id")
	userIDChanged := cmd.Flags().Changed("user-id")
	if global && userIDChanged {
		exit(1, "--global and --user-id are mutually exclusive")
	}
	if global || (!userIDChanged && cfg.App.Public) {
		return []client.Option{client.WithTargetUserID(0)}
	}
	if userIDChanged {
		return []client.Option{client.WithTargetUserID(targetUserID)}
	}
	return nil
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
	PreRun: func(cmd *cobra.Command, _ []string) {
		if public, _ := cmd.Flags().GetBool("public"); public {
			cfg.App.Public = true
		}
		if err := cfg.ValidatePublicMode(); err != nil {
			exit(1, "Failed to initialize config: "+err.Error())
		}
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
			fileQueue := indexer.NewFileIndexQueue()
			go func() {
				if err := fileQueue.Run(context.Background()); err != nil {
					log.Error().Err(err).Msg("File index queue failed")
				}
			}()
			go fileQueue.EnqueueAll(cfg.Indexer.Directories)
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
					fileQueue.EnqueueIndex(path, userID)
				}, func(path string) {
					fileQueue.EnqueueDelete(path)
				}); err != nil {
					log.Error().Err(err).Msg("File watcher failed")
				}
			}()
		}
		server.Version = Version
		server.Listen(cfg)
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
			return r, fmt.Errorf("invalid --start-date: %w", err)
		}
		r.From = t.Unix()
	}
	if v, _ := cmd.Flags().GetString("end-date"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return r, fmt.Errorf("invalid --end-date: %w", err)
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
	listenCmd.Flags().Bool("public", false, "allow unauthenticated access to public search interfaces")

	listURLsCmd.Flags().Bool("offline", false, "connect to the indexer directly without using the HTTP API (server should be stopped)")

	browserImportCmd.Flags().String("backend", "", "Crawler backend to use (\"http\", \"chromedp\", or \"bidi\")")
	// [fork] Register --min-visit on import-browser too. Upstream only registered it on
	// the JSON `import` command, so browser.go's cmd.Flags().GetInt("min-visit") always
	// errored and the visit-count filter silently no-op'd.
	browserImportCmd.Flags().IntP("min-visit", "m", 1, "only import URLs that were opened at least 'min-visit' times")

	importCmd.Flags().IntP("min-visit", "m", 1, "only import URLs that were opened at least 'min-visit' times")
	importCmd.Flags().String("backend", "", "Crawler backend to use (\"http\", \"chromedp\", or \"bidi\")")
	importCmd.Flags().StringToString("backend-option", nil, "Crawler backend option as key=value (repeatable, e.g. --backend-option exec_path=/usr/bin/chromium)")
	importCmd.Flags().StringToString("header", nil, "Extra HTTP header as KEY=VALUE (repeatable, e.g. --header Accept-Language=en)")
	importCmd.Flags().StringArray("cookie", nil, "HTTP cookie as Set-Cookie value (repeatable, e.g. --cookie \"session=abc; Domain=example.com\")")
	importCmd.Flags().String("start-date", "", "only import documents added on or after this date (YYYY-MM-DD)")
	importCmd.Flags().String("end-date", "", "only import documents added on or before this date (YYYY-MM-DD)")
	importCmd.Flags().Bool("global", false, "Make imported documents available for all users (only for admins in multiuser mode)")
	importCmd.Flags().Uint("user-id", 0, "Import documents under the given user ID (only for admins in multiuser mode)")

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
	if err := cfg.ValidatePublicMode(); err != nil {
		exit(1, "Failed to initialize config: "+err.Error())
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

func Execute() error {
	return rootCmd.Execute()
}
