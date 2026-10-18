package site

import (
	"fmt"
	"strings"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

// SwitchPHP changes the active PHP version for a website and updates web server routing.
func SwitchPHP(domain string, newVersion string) error {
	statePath := config.GetStatePath()

	newVersion = strings.TrimSpace(newVersion)
	if newVersion == "" {
		return fmt.Errorf("PHP version cannot be empty (e.g. 8.1, 8.2, 8.3)")
	}

	var oldVersion string

	err := config.WithLockedState(statePath, func(s *config.State) error {
		foundIdx := -1
		for i, site := range s.Sites {
			if strings.EqualFold(site.Domain, domain) {
				foundIdx = i
				break
			}
		}

		if foundIdx == -1 {
			return fmt.Errorf("site %s not found in state", domain)
		}

		// Validate that the requested PHP version is supported
		valid := false
		for _, v := range s.Global.SupportedPHPVersions {
			if v == newVersion {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("unsupported PHP version '%s' (supported versions: %s)",
				newVersion, strings.Join(s.Global.SupportedPHPVersions, ", "))
		}

		if s.Sites[foundIdx].PHPVersion == newVersion {
			return fmt.Errorf("site %s is already running PHP %s", domain, newVersion)
		}

		oldVersion = s.Sites[foundIdx].PHPVersion

		ui.Banner(fmt.Sprintf("Switching PHP Version: %s (%s ➔ %s)", domain, oldVersion, newVersion))

		// 1. Remove old PHP pool configuration and purge any orphaned pools across other versions
		ui.PrintStep(1, fmt.Sprintf("Removing PHP %s pool and cleaning any orphaned pools...", oldVersion))
		_, _ = server.DeleteOrphanedPHPPools(domain, newVersion, s.Global.SupportedPHPVersions)
		_ = server.DeletePHPPool(oldVersion, domain)

		// 2. Preserve memory limits from state, wp-config.php, or existing pool
		if s.Sites[foundIdx].MemoryLimit == "" {
			if wpMem, _, err := server.ReadWordPressMemoryLimit(s.Sites[foundIdx].PublicDir); err == nil && wpMem != "" {
				s.Sites[foundIdx].MemoryLimit = wpMem
			} else if oldPool, err := server.ReadPHPPoolConfig(oldVersion, domain); err == nil && oldPool.MemoryLimit != "" {
				s.Sites[foundIdx].MemoryLimit = oldPool.MemoryLimit
			}
		}

		// 3. Update site configuration in state
		s.Sites[foundIdx].PHPVersion = newVersion


		// 3. Generate and write new PHP pool configuration
		ui.PrintStep(2, fmt.Sprintf("Generating and provisioning PHP %s pool configuration...", newVersion))
		if err := server.WritePHPPool(&s.Sites[foundIdx]); err != nil {
			return fmt.Errorf("failed to create new PHP pool: %w", err)
		}

		// 4. Update Caddy reverse proxy upstream definitions
		ui.PrintStep(3, "Updating Caddyfile fastcgi unix socket route...")
		if err := server.WriteCaddyfile(s); err != nil {
			return fmt.Errorf("failed to update Caddyfile: %w", err)
		}

		// 5. Reload services
		ui.PrintStep(4, "Reloading PHP-FPM pools and Caddy web server...")
		_ = server.ReloadPHP(oldVersion)
		_ = server.ReloadPHP(newVersion)
		if err := server.ReloadCaddy(s); err != nil {
			return fmt.Errorf("failed to reload Caddy: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// 6. Reset OPcache
	_ = CacheClean(domain, false, false, true, false)

	ui.PrintSuccess("PHP Version Switched Successfully")
	ui.SectionHeader("PHP CONFIGURATION UPDATE")
	ui.Row("Domain", domain)
	ui.Row("Previous PHP Version", oldVersion)
	ui.Row("Active PHP Version", newVersion)
	ui.Row("Status", "● Active & Reloaded")
	ui.Divider()
	ui.PrintInfo("Your website is now processing requests using PHP " + newVersion + ". Fastcgi sockets have been seamlessly swapped without downtime.")
	fmt.Println()

	return nil
}
