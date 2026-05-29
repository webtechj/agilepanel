package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

var toolCmd = &cobra.Command{
	Use:   "tool",
	Short: "Install and manage server tools (phpMyAdmin, etc.)",
}

var toolInstallCmd = &cobra.Command{
	Use:   "install [tool]",
	Short: "Install a server tool (supported: phpmyadmin)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := args[0]
		if toolName != "phpmyadmin" {
			return fmt.Errorf("unsupported tool: %s (supported: phpmyadmin)", toolName)
		}

		ui.Banner("Install phpMyAdmin")
		ui.PrintInfo("Preparing database management tool installation...")

		if err := server.InstallPhpMyAdmin(); err != nil {
			return err
		}

		ui.PrintSuccess("phpMyAdmin Installed Successfully")
		ui.PrintInfo("phpMyAdmin is accessible securely on any of your site domains via:")
		ui.PrintInfo("https://[your-domain.com]/phpmyadmin")
		ui.PrintInfo("Note: Access is protected by the HTTP Basic Auth credentials configured via 'ap server auth'.")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

func init() {
	toolCmd.AddCommand(toolInstallCmd)
	rootCmd.AddCommand(toolCmd)
}
