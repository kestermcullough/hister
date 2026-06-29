package cmd

import (
	"fmt"

	"github.com/asciimoo/hister/client"

	"github.com/spf13/cobra"
)

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
