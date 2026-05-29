package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

var repairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Repair AgilePanel installation configurations without touching site files or databases",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Banner("Repair AgilePanel Installation")
		ui.PrintInfo("Starting non-destructive configuration repair...")
		
		err := server.RepairInstallation()
		if err != nil {
			return err
		}

		ui.PrintSuccess("AgilePanel Repair Completed")
		ui.PrintInfo("All system configurations, PHP pools, and Caddy settings are synchronized and active.")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(repairCmd)
}
