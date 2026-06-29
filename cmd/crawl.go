package cmd

import (
	"fmt"

	"github.com/asciimoo/hister/server/model"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

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
