package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"agilepanel/internal/server"
	"agilepanel/internal/site"
	"agilepanel/internal/ui"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update system package repositories and AgilePanel CLI binary",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Banner("AgilePanel System Update")

		ui.PrintStep(1, "Updating system package repositories...")
		if runtime.GOOS == "linux" {
			aptCmd := exec.Command("apt-get", "update", "-y")
			if err := aptCmd.Run(); err != nil {
				ui.PrintWarning(fmt.Sprintf("System package update failed: %v", err))
			} else {
				ui.PrintInfo("System package index updated successfully.")
			}
		} else {
			fmt.Println("APT (Mock): apt-get update -y")
		}

		ui.PrintStep(2, "Checking and downloading latest AgilePanel CLI binary...")
		if err := site.SelfUpdate(); err != nil {
			return err
		}

		ui.PrintSuccess("AgilePanel update completed successfully.")
		return nil
	},
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade system packages and repair configurations to apply upgrades cleanly",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Banner("AgilePanel System Upgrade")

		ui.PrintStep(1, "Upgrading system packages...")
		if runtime.GOOS == "linux" {
			aptCmd := exec.Command("apt-get", "upgrade", "-y")
			// Inject environment to run non-interactively
			aptCmd.Env = append(aptCmd.Env, "DEBIAN_FRONTEND=noninteractive")
			if err := aptCmd.Run(); err != nil {
				return fmt.Errorf("system package upgrade failed: %w", err)
			}
			ui.PrintInfo("System packages upgraded successfully.")
		} else {
			fmt.Println("APT (Mock): apt-get upgrade -y")
		}

		ui.PrintStep(2, "Running configuration repair and syncing all services...")
		if err := server.RepairInstallation(); err != nil {
			return err
		}

		ui.PrintSuccess("AgilePanel upgrade and configuration repair completed successfully.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(upgradeCmd)
}
