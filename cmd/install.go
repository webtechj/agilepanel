package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install AgilePanel addons and extensions",
}

var installGuiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Install and configure the AgilePanel Web GUI Dashboard companion",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Banner("Install AgilePanel Web GUI")
		ui.PrintInfo("Preparing to configure the lightweight Web GUI companion...")

		if err := server.InstallGui(); err != nil {
			return err
		}

		ui.PrintSuccess("AgilePanel Web GUI Dashboard Installed Successfully")
		ui.PrintInfo("The GUI runs on dedicated port 8889:")
		ui.PrintInfo("Access URL: http://[your-server-ip]:8889")
		ui.PrintInfo("Log in using your administrative credentials configured via 'ap server auth'.")
		ui.PrintInfo("If credentials are not yet set, use 'admin' / 'admin' and secure it immediately.")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

func init() {
	installCmd.AddCommand(installGuiCmd)
	rootCmd.AddCommand(installCmd)
}
