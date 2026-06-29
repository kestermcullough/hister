package cmd

import (
	"fmt"
	"os"

	"github.com/asciimoo/hister/config"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

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
