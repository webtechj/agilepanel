package server

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"runtime"
	"strings"

	"agilepanel/internal/config"
	"agilepanel/internal/ui"
)

// RepairInstallation restores all configuration files and applies optimisations.
func RepairInstallation() error {
	// 1. Load panel state database
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return fmt.Errorf("failed to read AgilePanel state database: %w", err)
	}

	// 2. Re-run system performance tuning and default webserver configuration
	ui.PrintStep(1, "Applying database, swap, and Redis performance optimizations...")
	if err := TuneServer(); err != nil {
		ui.PrintWarning(fmt.Sprintf("Tuning encountered errors: %v", err))
	}

	// 3. Recreate PHP pools for all sites
	ui.PrintStep(2, fmt.Sprintf("Recreating PHP pools for %d sites...", len(state.Sites)))
	for _, site := range state.Sites {
		ui.PrintInfo(fmt.Sprintf("Writing PHP pool for %s (PHP %s)...", site.Domain, site.PHPVersion))
		if err := WritePHPPool(&site); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to write PHP pool config for %s: %v", site.Domain, err))
		}
	}

	// 4. Recreate Caddyfile configuration
	ui.PrintStep(3, "Regenerating global Caddyfile configuration...")
	if err := WriteCaddyfile(state); err != nil {
		return fmt.Errorf("failed to regenerate Caddyfile configuration: %w", err)
	}

	// 5. Restore SSH Password Login for recovery
	if runtime.GOOS == "linux" {
		ui.PrintStep(4, "Restoring SSH configurations to allow password login (recovery mode)...")
		sshdPath := "/etc/ssh/sshd_config"
		if data, err := ioutil.ReadFile(sshdPath); err == nil {
			content := string(data)
			modified := false
			if strings.Contains(content, "PermitRootLogin prohibit-password") {
				content = strings.ReplaceAll(content, "PermitRootLogin prohibit-password", "PermitRootLogin yes")
				modified = true
			}
			if modified {
				if err := ioutil.WriteFile(sshdPath, []byte(content), 0644); err == nil {
					_ = exec.Command("systemctl", "reload", "ssh").Run()
					_ = exec.Command("systemctl", "reload", "sshd").Run()
					ui.PrintInfo("SSH Configuration updated to allow password login. Reloaded SSH daemon.")
				}
			}
		}
	}

	// 6. Reload services to apply repaired configs
	ui.PrintStep(5, "Reloading Caddy and PHP-FPM services...")
	
	// Reload PHP for unique versions used in sites
	reloadedVersions := make(map[string]bool)
	for _, site := range state.Sites {
		if !reloadedVersions[site.PHPVersion] {
			ui.PrintInfo(fmt.Sprintf("Reloading PHP-FPM %s service...", site.PHPVersion))
			if err := ReloadPHP(site.PHPVersion); err != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to reload PHP %s: %v", site.PHPVersion, err))
			}
			reloadedVersions[site.PHPVersion] = true
		}
	}

	// Reload Caddy
	ui.PrintInfo("Reloading Caddy service...")
	if err := ReloadCaddy(state); err != nil {
		return fmt.Errorf("failed to reload Caddy: %w", err)
	}

	return nil
}
