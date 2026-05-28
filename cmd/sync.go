package cmd

import (
	"github.com/spf13/cobra"

	"agilepanel/internal/site"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize all configurations and update AgilePanel binary",
	RunE: func(cmd *cobra.Command, args []string) error {
		return site.Sync()
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
