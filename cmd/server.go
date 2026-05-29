package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage global server settings and status",
}

var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of global server dependencies and resources",
	RunE: func(cmd *cobra.Command, args []string) error {
		status, err := server.GetStatus()
		if err != nil {
			return err
		}

		ui.Banner("AgilePanel Server Status")

		ui.SectionHeader("RESOURCES")
		ui.Row("Active Sites", fmt.Sprintf("%d", status.ActiveSites))
		ui.Row("Total Memory", status.TotalMemory)
		ui.Row("Available Memory", status.FreeMemory)

		ui.SectionHeader("SERVICES")

		// Sort service names for consistent output
		svcs := make([]string, 0, len(status.Services))
		for svc := range status.Services {
			svcs = append(svcs, svc)
		}
		sort.Strings(svcs)

		for _, svc := range svcs {
			active := status.Services[svc]
			if active {
				ui.RowBadge(svc, "● active", ui.BrightGreen)
			} else {
				ui.RowBadge(svc, "○ inactive", ui.BrightRed)
			}
		}

		ui.Divider()
		fmt.Println()
		return nil
	},
}

var serverAuthCmd = &cobra.Command{
	Use:   "auth [username] [password]",
	Short: "Configure HTTP Basic Auth credentials for phpMyAdmin and server tools",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		username := args[0]
		password := args[1]

		hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}
		hash := string(hashedBytes)

		statePath := config.GetStatePath()
		err = config.WithLockedState(statePath, func(s *config.State) error {
			s.Global.AdminUser = username
			s.Global.AdminPasswordHash = hash

			// Regenerate Caddyfile with basic_auth blocks
			if err := server.WriteCaddyfile(s); err != nil {
				return fmt.Errorf("failed to write Caddyfile: %w", err)
			}

			// Reload Caddy
			if err := server.ReloadCaddy(s); err != nil {
				return fmt.Errorf("failed to reload Caddy: %w", err)
			}

			return nil
		})
		if err != nil {
			return err
		}

		ui.PrintSuccess("HTTP Basic Auth Configured")
		ui.SectionHeader("CREDENTIALS")
		ui.Row("Username", username)
		ui.Row("Scope", "phpMyAdmin + server tools")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

var (
	adminNameFlag  string
	adminEmailFlag string
)

var serverRestartCmd = &cobra.Command{
	Use:   "restart [service|all]",
	Short: "Restart system services (caddy, mariadb, redis, php-fpm, or all)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serviceName := args[0]
		return server.RestartService(serviceName)
	},
}

var serverTuneCmd = &cobra.Command{
	Use:   "tune",
	Short: "Optimize system configurations (Swap memory allocation, MariaDB buffer pool sizes, kernel cache swappiness)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if adminNameFlag != "" || adminEmailFlag != "" {
			statePath := config.GetStatePath()
			err := config.WithLockedState(statePath, func(s *config.State) error {
				if adminNameFlag != "" {
					s.Global.AdminName = adminNameFlag
				}
				if adminEmailFlag != "" {
					s.Global.AdminEmail = adminEmailFlag
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return server.TuneServer()
	},
}

func init() {
	serverTuneCmd.Flags().StringVar(&adminNameFlag, "admin-name", "", "Admin name for the server")
	serverTuneCmd.Flags().StringVar(&adminEmailFlag, "admin-email", "", "Admin email for SSL installation")

	serverCmd.AddCommand(serverStatusCmd)
	serverCmd.AddCommand(serverAuthCmd)
	serverCmd.AddCommand(serverRestartCmd)
	serverCmd.AddCommand(serverTuneCmd)
	rootCmd.AddCommand(serverCmd)
}
