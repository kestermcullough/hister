package cmd

import (
	"context"
	"fmt"

	"github.com/asciimoo/hister/client"
	"github.com/asciimoo/hister/server/crawler"
	"github.com/asciimoo/hister/server/extractor"
	"github.com/asciimoo/hister/server/model"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

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
		clientOpts := targetUserIDClientOptions(cmd, global)
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
