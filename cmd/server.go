package cmd

import (
	"fmt"
	"sort"
	"strings"

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

func renderProgressBar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}
	filled := int((pct / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled

	var color string
	if pct < 60 {
		color = ui.BrightGreen
	} else if pct < 85 {
		color = ui.BrightYellow
	} else {
		color = ui.BrightRed
	}

	bar := color + strings.Repeat("█", filled) + ui.Reset + ui.Muted(strings.Repeat("░", empty))
	return fmt.Sprintf("[%s] %.1f%%", bar, pct)
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

		ui.SectionHeader("REAL-TIME RESOURCES")
		ui.Row("Active Sites", fmt.Sprintf("%d", status.ActiveSites))
		ui.Row("CPU Usage", renderProgressBar(status.RealtimeCPU, 15))
		ui.Row("Load Averages", fmt.Sprintf("%.2f, %.2f, %.2f (1m, 5m, 15m)", status.Load1m, status.Load5m, status.Load15m))
		ui.Row("TCP Connections", fmt.Sprintf("%d active sockets", status.TCPConnections))

		memVal := fmt.Sprintf("%s / %.2f GB used (total %.2f GB)", renderProgressBar(status.MemoryPercentage, 15), status.UsedMemoryGB, status.TotalMemoryGB)
		ui.Row("RAM Memory", memVal)

		swapVal := fmt.Sprintf("%s / %.2f GB used (total %.2f GB)", renderProgressBar(status.SwapPercentage, 15), status.UsedSwapGB, status.TotalSwapGB)
		ui.Row("Swap Memory", swapVal)

		diskVal := fmt.Sprintf("%s / %.2f GB used (total %.2f GB)", renderProgressBar(status.DiskPercentage, 15), status.UsedDiskGB, status.TotalDiskGB)
		ui.Row("Disk Usage (/)", diskVal)

		ui.SectionHeader("TOP 5 REAL-TIME PROCESSES (CPU)")
		if len(status.TopProcesses) == 0 {
			ui.PrintWarning("No active processes detected.")
		} else {
			var cols = []ui.TableColumn{
				{Header: "PID", Width: 8},
				{Header: "CPU %", Width: 8},
				{Header: "MEM %", Width: 8},
				{Header: "Command", Width: 20},
			}
			var rows [][]string
			for _, p := range status.TopProcesses {
				rows = append(rows, []string{
					fmt.Sprintf("%d", p.PID),
					fmt.Sprintf("%.1f%%", p.CPU),
					fmt.Sprintf("%.1f%%", p.Mem),
					p.Comm,
				})
			}
			ui.PrintTable(cols, rows)
		}

		ui.SectionHeader("LAST 24 HOURS METRICS SUMMARY")
		if !status.HasHistorical {
			ui.PrintWarning("Historical metrics log is empty or collecting first data point.")
			ui.PrintInfo("Run 'ap server log-metrics' or wait for the cron job to collect historical usage snapshots.")
		} else {
			ui.Row("Peak CPU Usage", fmt.Sprintf("%.1f%%", status.PeakCPU24h))
			ui.Row("Peak RAM Usage", fmt.Sprintf("%.1f%%", status.PeakMemory24h))
			ui.Row("Peak Swap Usage", fmt.Sprintf("%.1f%%", status.PeakSwap24h))

			fmt.Println()
			ui.PrintInfo("Top 5 Resource Consuming Processes (Peak CPU):")
			var cols = []ui.TableColumn{
				{Header: "Command", Width: 25},
				{Header: "Peak CPU %", Width: 12},
				{Header: "Peak MEM %", Width: 12},
			}
			var rows [][]string
			for _, p := range status.TopProcesses24h {
				rows = append(rows, []string{
					p.Comm,
					fmt.Sprintf("%.1f%%", p.CPU),
					fmt.Sprintf("%.1f%%", p.Mem),
				})
			}
			ui.PrintTable(cols, rows)
		}

		ui.Divider()
		fmt.Println()
		return nil
	},
}

var serverAuthCmd = &cobra.Command{
	Use:   "auth [username] [password]",
	Short: "Configure HTTP Basic Auth credentials for phpMyAdmin and server tools",
	Args:  cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		var username string
		var password string

		if len(args) >= 1 {
			username = args[0]
		} else {
			var err error
			username, err = promptString("Enter username: ")
			if err != nil {
				return err
			}
			if username == "" {
				return fmt.Errorf("username cannot be empty")
			}
		}

		if len(args) >= 2 {
			password = args[1]
		} else {
			var err error
			password, err = promptPassword("Enter password: ")
			if err != nil {
				return err
			}
			if password == "" {
				return fmt.Errorf("password cannot be empty")
			}
		}

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
		ui.PrintInfo("Your phpMyAdmin and administrative tools are now secured behind HTTP Basic Authentication. You must enter these credentials when accessing the phpMyAdmin page at http://[your-ip]:8888.")
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
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serviceName, err := getServiceArg(args)
		if err != nil {
			return err
		}
		err = server.RestartService(serviceName)
		if err != nil {
			return err
		}
		ui.PrintSuccess(fmt.Sprintf("Service %s Restarted", serviceName))
		ui.PrintInfo(fmt.Sprintf("AgilePanel has successfully restarted %s systemctl service to apply configuration or memory updates.", serviceName))
		ui.Divider()
		fmt.Println()
		return nil
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
		err := server.TuneServer()
		if err != nil {
			return err
		}
		ui.PrintSuccess("Server Performance Tuning Completed")
		ui.PrintInfo("AgilePanel has audited your server hardware resources and configured optimized database buffers (30% RAM), Redis UNIX socket connections, persistent swap memories, and strict cache systems.")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

var serverLogMetricsCmd = &cobra.Command{
	Use:    "log-metrics",
	Short:  "Record system resource snapshot metrics (run by cron)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return server.LogMetrics()
	},
}

var serverSecureCmd = &cobra.Command{
	Use:   "secure",
	Short: "Audit and harden server security (SSH policies, UFW firewall, password aging)",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := server.SecureServer()
		if err != nil {
			return err
		}
		ui.PrintSuccess("Server Security Hardening Completed")
		ui.SectionHeader("UBUNTU SERVER SECURITY GUIDANCE")
		ui.PrintInfo("1. Password Aging Enforced: The 'root' user is now subject to a 30-day password rotation policy.")
		ui.PrintInfo("2. SSH Hardening: Root password login is disabled. Please ensure you have added your SSH Public Key to '/root/.ssh/authorized_keys' to access the server as root.")
		ui.PrintInfo("3. Firewall Active: UFW firewall is active and restricting traffic to ports 22 (SSH), 80 (HTTP), 443 (HTTPS), and 8888 (AgilePanel Admin).")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

var serverUnlockGuiCmd = &cobra.Command{
	Use:   "unlock-gui",
	Short: "Disable secondary GUI panel session security locks (in case of lockout)",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := server.UnlockGuiPanel()
		if err != nil {
			return err
		}
		ui.PrintSuccess("GUI Panel Security Lock Disabled")
		ui.PrintInfo("The secondary session lock layer has been disabled. You can now log into your dashboard using Basic Authentication.")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

var serverCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clear log files, old backup files, and unused cache to free up space",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := server.CleanServer()
		if err != nil {
			return err
		}
		ui.PrintSuccess("Server Disk Cleanup Completed")
		ui.PrintInfo("AgilePanel has successfully cleared log files, deleted expired backups, and purged unused caching directories.")
		ui.Divider()
		fmt.Println()
		return nil
	},
}

func init() {
	serverTuneCmd.Flags().StringVar(&adminNameFlag, "admin-name", "", "Admin name for the server")
	serverTuneCmd.Flags().StringVar(&adminEmailFlag, "admin-email", "", "Admin email for SSL installation")

	serverCmd.AddCommand(serverStatusCmd)
	serverCmd.AddCommand(serverAuthCmd)
	serverCmd.AddCommand(serverRestartCmd)
	serverCmd.AddCommand(serverTuneCmd)
	serverCmd.AddCommand(serverSecureCmd)
	serverCmd.AddCommand(serverLogMetricsCmd)
	serverCmd.AddCommand(serverUnlockGuiCmd)
	serverCmd.AddCommand(serverCleanCmd)
	rootCmd.AddCommand(serverCmd)
}
