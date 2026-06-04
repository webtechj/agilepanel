package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show a detailed, visually appealing dashboard of AgilePanel installation and stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Resolve Executable Path
		exePath, err := os.Executable()
		if err != nil {
			exePath = "Unknown (ap)"
		} else {
			exePath, _ = filepath.Abs(exePath)
		}

		// 2. Load config state
		statePath := config.GetStatePath()
		state, stateErr := config.ReadState(statePath)
		siteCount := 0
		defaultPHP := "8.3"
		supportedPHPs := []string{"8.1", "8.2", "8.3"}
		if stateErr == nil {
			siteCount = len(state.Sites)
			defaultPHP = state.Global.DefaultPHPVersion
			supportedPHPs = state.Global.SupportedPHPVersions
		}

		// 3. Resolve Public IP
		publicIP := server.ResolvePublicIP()

		// 4. Retrieve stack status
		status, _ := server.GetStatus()

		// ─── STYLISH TERMINAL GRAPHICS ──────────────────────────────────────────

		// Beautiful ASCII Banner
		fmt.Println()
		fmt.Printf("   %s%s█▀▀█ █▀▀█ ░█▀▀█ █▀▀█ █▀▀▄ █▀▀ █    %s\n", ui.BrightCyan, ui.Bold, ui.Reset)
		fmt.Printf("   %s%s█▄▄█ █▄▄█ ░█▄▄█ █▄▄█ █  █ █▀▀ █    %s\n", ui.BrightCyan, ui.Bold, ui.Reset)
		fmt.Printf("   %s%s▀  ▀ █    ░█  ░ ▀  ▀ ▀  ▀ ▀▀▀ ▀▀▀  %s\n", ui.BrightCyan, ui.Bold, ui.Reset)
		fmt.Printf("   %s%s  ⚡ WordPress VPS Management CLI  %s\n", ui.Muted(""), ui.Italic, ui.Reset)
		fmt.Println("  " + ui.BrightBlack + "────────────────────────────────────────────────────────────────────────" + ui.Reset)

		// Panel Info Section
		ui.SectionHeader("AGILEPANEL INSTALLATION DETAILS")
		ui.Row("CLI Version", Version)
		ui.Row("Binary Path", exePath)
		ui.Row("Config State", statePath)
		ui.Row("Host OS/Arch", fmt.Sprintf("%s / %s", runtime.GOOS, runtime.GOARCH))
		ui.Row("Public IP", publicIP)
		ui.Row("Hosted Sites", fmt.Sprintf("%d active website(s)", siteCount))

		guiInstalled, guiVersion := server.GetGuiInfo()
		guiStatusStr := "Not Installed"
		if guiInstalled {
			guiStatusStr = fmt.Sprintf("Installed (v%s)", guiVersion)
		}
		ui.Row("Web GUI Addon", guiStatusStr)

		// Stack & Running Services Section
		ui.SectionHeader("SYSTEM STACK & RUNNING SERVICES")

		// List of services
		type stackService struct {
			Name        string
			VersionInfo string
			Details     string
		}

		services := []stackService{
			{Name: "caddy", VersionInfo: "Web Server (HTTP/3, SSL)", Details: "Config: /etc/caddy/Caddyfile"},
			{Name: "mariadb", VersionInfo: "Database Engine", Details: "Socket: /var/run/mysqld/mysqld.sock"},
			{Name: "redis-server", VersionInfo: "Cache Socket Manager", Details: "Socket: /var/run/redis/redis-server.sock"},
		}

		for _, v := range supportedPHPs {
			isDefault := ""
			if v == defaultPHP {
				isDefault = " (Default)"
			}
			services = append(services, stackService{
				Name:        fmt.Sprintf("php%s-fpm", v),
				VersionInfo: fmt.Sprintf("PHP Process Pool Manager%s", isDefault),
				Details:     fmt.Sprintf("Config: /etc/php/%s/fpm/pool.d/", v),
			})
		}

		// Sort services to group nicely
		sort.Slice(services, func(i, j int) bool {
			return services[i].Name < services[j].Name
		})

		for _, svc := range services {
			active := false
			if status != nil {
				active = status.Services[svc.Name]
			} else {
				// Fallback if systemctl fails or offline
				if runtime.GOOS != "linux" {
					// Dummy default active for demo on non-linux systems
					active = true
				}
			}

			statusText := "○ inactive"
			statusColor := ui.BrightRed
			if active {
				statusText = "● active"
				statusColor = ui.BrightGreen
			}

			// Render service box line
			keyFmt := ui.KeyStr(svc.Name)
			padLength := 15 - len(svc.Name)
			if padLength < 0 {
				padLength = 0
			}
			padding := strings.Repeat(" ", padLength)

			fmt.Printf("  %s%s %s[%s]%s  %s  %s(%s)%s\n",
				keyFmt, padding,
				statusColor, statusText, ui.Reset,
				ui.Value(svc.VersionInfo),
				ui.BrightBlack, svc.Details, ui.Reset,
			)
		}

		// System Resources Section (Real-time meters)
		if status != nil {
			ui.SectionHeader("VPS RESOURCE METRICS")

			// Custom tiny meter helper
			renderMeter := func(pct float64) string {
				width := 10
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
				return color + strings.Repeat("■", filled) + ui.Muted(strings.Repeat("□", empty)) + ui.Reset + fmt.Sprintf(" %5.1f%%", pct)
			}

			ui.Row("CPU Utilization", renderMeter(status.RealtimeCPU))
			ui.Row("RAM Memory Usage", fmt.Sprintf("%s  (%.2f / %.2f GB)", renderMeter(status.MemoryPercentage), status.UsedMemoryGB, status.TotalMemoryGB))
			ui.Row("Swap File Usage", fmt.Sprintf("%s  (%.2f / %.2f GB)", renderMeter(status.SwapPercentage), status.UsedSwapGB, status.TotalSwapGB))
			ui.Row("Root Disk Space", fmt.Sprintf("%s  (%.2f / %.2f GB)", renderMeter(status.DiskPercentage), status.UsedDiskGB, status.TotalDiskGB))
		}

		ui.Divider()
		fmt.Println()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
}
