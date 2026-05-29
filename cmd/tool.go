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
		ui.PrintInfo("phpMyAdmin is accessible on port 8888:")
		ui.PrintInfo("http://[your-server-ip]:8888")
		ui.PrintInfo("Secure it with: ap server auth [username] [password]")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

var toolFixPhpMyAdminCmd = &cobra.Command{
	Use:   "fix-phpmyadmin",
	Short: "Regenerate phpMyAdmin config.inc.php to fix configuration errors",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Banner("Fix phpMyAdmin Config")
		ui.PrintInfo("Regenerating a fresh config.inc.php for phpMyAdmin...")

		if err := server.FixPhpMyAdminConfig(); err != nil {
			return err
		}

		ui.PrintSuccess("phpMyAdmin Config Fixed")
		ui.PrintInfo("phpMyAdmin config.inc.php has been regenerated with a fresh blowfish secret.")
		ui.PrintInfo("Access phpMyAdmin at: http://[your-server-ip]:8888")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

func init() {
	toolCmd.AddCommand(toolInstallCmd)
	toolCmd.AddCommand(toolFixPhpMyAdminCmd)
	rootCmd.AddCommand(toolCmd)
}
