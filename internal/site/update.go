package site

import (
	"fmt"
	"strings"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

// Update modifies a website's PHP version, memory limits, and performance parameters.
func Update(domain string, targetPHP string, targetMemory string, heavyPlugins bool) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return fmt.Errorf("domain name cannot be empty")
	}

	targetPHP = strings.TrimSpace(targetPHP)
	targetMemory = strings.TrimSpace(targetMemory)

	if heavyPlugins && targetMemory == "" {
		targetMemory = "256M"
	}

	if targetMemory != "" {
		u := strings.ToUpper(targetMemory)
		if !strings.HasSuffix(u, "M") && !strings.HasSuffix(u, "G") {
			u = u + "M"
		}
		targetMemory = u
	}

	if targetPHP == "" && targetMemory == "" {
		return fmt.Errorf("nothing to update for '%s': please specify a PHP version (e.g. --php83, --php82) or memory limit (--memory 256M, --heavy-plugins)", domain)
	}

	statePath := config.GetStatePath()

	var oldPHP string
	var finalPHP string
	var finalMemory string
	var phpChanged bool
	var memoryChanged bool

	err := config.WithLockedState(statePath, func(s *config.State) error {
		foundIdx := -1
		for i, site := range s.Sites {
			if strings.EqualFold(site.Domain, domain) {
				foundIdx = i
				break
			}
		}

		if foundIdx == -1 {
			return fmt.Errorf("site '%s' not found in AgilePanel state", domain)
		}

		site := &s.Sites[foundIdx]
		oldPHP = site.PHPVersion
		finalPHP = oldPHP

		// 1. Evaluate PHP version update
		if targetPHP != "" && targetPHP != oldPHP {
			valid := false
			for _, v := range s.Global.SupportedPHPVersions {
				if v == targetPHP {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("unsupported PHP version '%s' (supported versions: %s)",
					targetPHP, strings.Join(s.Global.SupportedPHPVersions, ", "))
			}
			phpChanged = true
			finalPHP = targetPHP
		}

		// 2. Evaluate Memory / Heavy Plugin update
		currentMem := site.MemoryLimit
		if currentMem == "" {
			if wpMem, _, err := server.ReadWordPressMemoryLimit(site.PublicDir); err == nil && wpMem != "" {
				currentMem = wpMem
			} else if site.Type == "woocommerce" {
				currentMem = "256M"
			} else {
				currentMem = "128M"
			}
		}

		if targetMemory != "" && targetMemory != currentMem {
			memoryChanged = true
			finalMemory = targetMemory
		} else if targetMemory != "" {
			finalMemory = currentMem
		} else {
			finalMemory = currentMem
		}

		if !phpChanged && !memoryChanged {
			ui.PrintInfo(fmt.Sprintf("Site '%s' is already running PHP %s with %s memory limit. No changes required.", domain, oldPHP, finalMemory))
			return nil
		}

		ui.Banner(fmt.Sprintf("Updating Website Settings: %s", domain))

		// Apply changes to site state
		if phpChanged {
			ui.PrintStep(1, fmt.Sprintf("Switching PHP runtime from %s to %s...", oldPHP, finalPHP))
			// Remove old pool config and any orphaned pool configs across other versions
			_, _ = server.DeleteOrphanedPHPPools(domain, finalPHP, s.Global.SupportedPHPVersions)
			_ = server.DeletePHPPool(oldPHP, domain)
			site.PHPVersion = finalPHP
		}

		if memoryChanged {
			ui.PrintStep(2, fmt.Sprintf("Tuning PHP memory limit to %s...", finalMemory))
			site.MemoryLimit = finalMemory
			if finalMemory == "256M" || finalMemory == "512M" {
				site.MaxExecutionTime = 300
				site.MaxInputVars = 5000
			}
		}

		// Write new PHP-FPM pool configuration
		ui.PrintStep(3, fmt.Sprintf("Regenerating PHP-FPM pool configuration for PHP %s...", site.PHPVersion))
		if err := server.WritePHPPool(site); err != nil {
			return fmt.Errorf("failed to write updated PHP pool: %w", err)
		}

		// Update web server routing if PHP version changed
		if phpChanged {
			ui.PrintStep(4, "Updating Caddyfile FastCGI unix socket routes...")
			if err := server.WriteCaddyfile(s); err != nil {
				return fmt.Errorf("failed to update Caddyfile: %w", err)
			}

			ui.PrintStep(5, "Reloading PHP-FPM daemons and Caddy server...")
			_ = server.ReloadPHP(oldPHP)
			_ = server.ReloadPHP(finalPHP)
			if err := server.ReloadCaddy(s); err != nil {
				ui.PrintWarning(fmt.Sprintf("Caddy reload encountered a notice: %v", err))
			}
		} else {
			ui.PrintStep(4, fmt.Sprintf("Reloading PHP %s-FPM to activate new memory limit...", site.PHPVersion))
			if err := server.ReloadPHP(site.PHPVersion); err != nil {
				ui.PrintWarning(fmt.Sprintf("PHP reload encountered a notice: %v", err))
			}
		}

		// Update WordPress wp-config.php if applicable
		isWP := site.Type == "wp" || site.Type == "woocommerce" || site.Type == ""
		if isWP && memoryChanged {
			ui.PrintStep(6, fmt.Sprintf("Syncing WP_MEMORY_LIMIT to %s in WordPress wp-config.php...", finalMemory))
			if err := server.TuneWordPressMemory(site.SystemUser, site.PublicDir, finalMemory); err != nil {
				ui.PrintWarning(fmt.Sprintf("Could not update wp-config.php memory definitions: %v", err))
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	if !phpChanged && !memoryChanged {
		return nil
	}

	ui.PrintSuccess("Site Settings Updated Successfully")
	ui.SectionHeader("UPDATED SITE PROFILE")
	ui.Row("Domain", domain)
	ui.Row("Active PHP Version", finalPHP)
	ui.Row("PHP Memory Limit", finalMemory)
	if finalMemory == "256M" || finalMemory == "512M" {
		ui.RowBadge("Plugin Optimization", "● High-Performance (Elementor / WooCommerce)", ui.BrightGreen)
	} else {
		ui.RowBadge("Plugin Optimization", "○ Standard (128M)", ui.BrightYellow)
	}
	ui.Divider()
	fmt.Println()

	return nil
}
