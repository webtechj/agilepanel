package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Manage the AgilePanel Web GUI Dashboard",
}

var guiDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable the AgilePanel Web GUI Dashboard and block port access",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Banner("Disable AgilePanel Web GUI")
		ui.PrintInfo("Preparing to disable the Web GUI companion and restrict access...")

		if err := server.DisableGui(); err != nil {
			return err
		}

		ui.PrintSuccess("AgilePanel Web GUI Dashboard Disabled Successfully")
		ui.PrintInfo("The GUI service has been stopped and disabled. Port 8889 has been restricted.")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

var guiEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable the AgilePanel Web GUI Dashboard and open port access",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Banner("Enable AgilePanel Web GUI")
		ui.PrintInfo("Preparing to enable the Web GUI companion and allow access...")

		if err := server.EnableGui(); err != nil {
			return err
		}

		ui.PrintSuccess("AgilePanel Web GUI Dashboard Enabled Successfully")
		ui.PrintInfo("The GUI service has been enabled and started. Port 8889 has been opened.")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

var guiUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the AgilePanel Web GUI Dashboard to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Banner("Update AgilePanel Web GUI")
		ui.PrintInfo("Preparing to update the Web GUI companion to the latest version...")

		if err := server.UpdateGui(); err != nil {
			return err
		}

		ui.PrintSuccess("AgilePanel Web GUI Dashboard Updated Successfully")
		ui.PrintInfo("The GUI binary has been updated and the daemon service restarted.")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

func init() {
	guiCmd.AddCommand(guiDisableCmd)
	guiCmd.AddCommand(guiEnableCmd)
	guiCmd.AddCommand(guiUpdateCmd)
	rootCmd.AddCommand(guiCmd)
}
