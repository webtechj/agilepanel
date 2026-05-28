package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
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

		fmt.Println("=========================================")
		fmt.Println("             AGILEPANEL STATUS           ")
		fmt.Println("=========================================")
		fmt.Printf("Active Sites:  %d\n", status.ActiveSites)
		fmt.Printf("System Memory: Total: %s, Free/Available: %s\n", status.TotalMemory, status.FreeMemory)
		fmt.Println("-----------------------------------------")
		fmt.Println("Service Statuses:")
		for svc, active := range status.Services {
			statusStr := "inactive 🔴"
			if active {
				statusStr = "active 🟢"
			}
			fmt.Printf("  %-15s: %s\n", svc, statusStr)
		}
		fmt.Println("=========================================")
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

		fmt.Printf("Success: HTTP Basic Authentication configured. Username: %s\n", username)
		return nil
	},
}

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
		return server.TuneServer()
	},
}

func init() {
	serverCmd.AddCommand(serverStatusCmd)
	serverCmd.AddCommand(serverAuthCmd)
	serverCmd.AddCommand(serverRestartCmd)
	serverCmd.AddCommand(serverTuneCmd)
	rootCmd.AddCommand(serverCmd)
}
