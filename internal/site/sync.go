package site

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

// SelfUpdate downloads the latest ap binary from GitHub to replace the running executable.
// By default it fetches from the latest stable GitHub Release.
// Set AGILEPANEL_UPDATE_BRANCH (e.g. "development") to override and pull from a raw branch instead.
func SelfUpdate() error {
	if runtime.GOOS != "linux" {
		fmt.Println("Self-Update (Mock): Downloading latest ap-linux-amd64 to /usr/local/bin/ap")
		return nil
	}

	destPath := "/usr/local/bin/ap"
	branch := os.Getenv("AGILEPANEL_UPDATE_BRANCH")

	var url string
	if branch != "" {
		url = fmt.Sprintf("https://raw.githubusercontent.com/webtechj/agilepanel/%s/ap-linux-amd64", branch)
		fmt.Printf("Self-Update: Using branch override '%s'...\n", branch)
	} else {
		url = "https://github.com/webtechj/agilepanel/releases/latest/download/ap-linux-amd64"
	}

	fmt.Printf("Self-Update: Fetching latest binary from %s...\n", url)
	tmpFile := destPath + ".tmp"
	cmd := exec.Command("curl", "-fsSL", "--proto", "=https", "--tlsv1.2", "-o", tmpFile, url)
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to download new binary: %w", err)
	}

	info, err := os.Stat(tmpFile)
	if err != nil || info.Size() < 1024*1024 { // AgilePanel binary is several MBs; sanity check against truncated HTML error pages
		_ = os.Remove(tmpFile)
		return fmt.Errorf("downloaded binary is truncated or invalid (file size: %d bytes)", info.Size())
	}

	if err := os.Chmod(tmpFile, 0755); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	if err := os.Rename(tmpFile, destPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to replace running binary: %w", err)
	}

	fmt.Println("Self-Update: Binary updated successfully.")
	return nil
}

// Sync regenerates all system configuration files for existing sites to match the panel state database.
func Sync() error {
	// 1. Perform self-update
	if err := SelfUpdate(); err != nil {
		ui.PrintWarning(fmt.Sprintf("Self-update failed: %v", err))
	}

	// 2. Load and lock state
	statePath := config.GetStatePath()
	err := config.WithLockedState(statePath, func(s *config.State) error {
		// Scan for pre-existing site installations
		ui.PrintStep(1, "Scanning /var/www for pre-existing site directories...")
		var webRoot = "/var/www"
		if runtime.GOOS != "linux" {
			if val := os.Getenv("AGILEPANEL_WEBROOT"); val != "" {
				webRoot = val
			} else {
				webRoot = filepath.Join("var", "www")
			}
		}

		if _, err := os.Stat(webRoot); err == nil {
			entries, err := os.ReadDir(webRoot)
			if err == nil {
				for _, entry := range entries {
					if !entry.IsDir() {
						continue
					}
					name := entry.Name()
					if err := ValidateDomain(name); err != nil {
						continue
					}
					domain := strings.ToLower(name)

					// Check if already registered
					alreadyTracked := false
					for i, site := range s.Sites {
						if strings.EqualFold(site.Domain, domain) {
							alreadyTracked = true
							// Reconcile lock status if out of sync with filesystem
							domainPath := filepath.Join(webRoot, name)
							fsLocked := server.IsDirectoryLocked(domainPath)
							if site.IsLocked != fsLocked {
								s.Sites[i].IsLocked = fsLocked
								ui.PrintInfo(fmt.Sprintf("Reconciled lock state for %s: is_locked=%v (synchronized with filesystem)", domain, fsLocked))
							}
							break
						}
					}
					if alreadyTracked {
						continue
					}

					domainPath := filepath.Join(webRoot, name)
					// Verify it has an htdocs directory
					htdocsPath := filepath.Join(domainPath, "htdocs")
					if _, err := os.Stat(htdocsPath); os.IsNotExist(err) {
						continue
					}

					// Auto-detect system user and default configs
					systemUser := SanitizeUser(domain)

					// Detect PHP version
					phpVersion := s.Global.DefaultPHPVersion
					for _, v := range s.Global.SupportedPHPVersions {
						var poolPath string
						if runtime.GOOS == "linux" {
							poolPath = fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", v, domain)
						} else {
							poolPath = filepath.Join("etc", "php", v, "fpm", "pool.d", domain+".conf")
						}
						if _, err := os.Stat(poolPath); err == nil {
							phpVersion = v
							break
						}
					}

					// Detect site type and set PublicDir
					publicDir := htdocsPath
					siteType := "php"

					laravelPublicPath := filepath.Join(htdocsPath, "public")
					if _, err := os.Stat(laravelPublicPath); err == nil {
						publicDir = laravelPublicPath
						siteType = "laravel"
					} else if _, err := os.Stat(filepath.Join(htdocsPath, "wp-config.php")); err == nil {
						siteType = "wp"
					} else if _, err := os.Stat(filepath.Join(domainPath, "wp-config.php")); err == nil {
						siteType = "wp"
					} else if _, err := os.Stat(filepath.Join(htdocsPath, "index.html")); err == nil {
						siteType = "html"
					}

					// Parse database settings if present
					dbName := ""
					dbUser := ""
					dbPass := ""

					// Search for wp-config.php (could be in htdocs or parent domain folder)
					wpConfigSearchPaths := []string{
						filepath.Join(htdocsPath, "wp-config.php"),
						filepath.Join(domainPath, "wp-config.php"),
					}
					for _, path := range wpConfigSearchPaths {
						if _, err := os.Stat(path); err == nil {
							contentBytes, err := os.ReadFile(path)
							if err == nil {
								content := string(contentBytes)
								dbNameRegex := regexp.MustCompile(`define\s*\(\s*['"]DB_NAME['"]\s*,\s*['"]([^'"]+)['"]\s*\)`)
								dbUserRegex := regexp.MustCompile(`define\s*\(\s*['"]DB_USER['"]\s*,\s*['"]([^'"]+)['"]\s*\)`)
								dbPassRegex := regexp.MustCompile(`define\s*\(\s*['"]DB_PASSWORD['"]\s*,\s*['"]([^'"]+)['"]\s*\)`)

								if match := dbNameRegex.FindStringSubmatch(content); len(match) > 1 {
									dbName = match[1]
								}
								if match := dbUserRegex.FindStringSubmatch(content); len(match) > 1 {
									dbUser = match[1]
								}
								if match := dbPassRegex.FindStringSubmatch(content); len(match) > 1 {
									dbPass = match[1]
								}
								break
							}
						}
					}

					// Detect memory limit from wp-config.php if applicable
					memLimit := ""
					if wpMem, _, err := server.ReadWordPressMemoryLimit(publicDir); err == nil && wpMem != "" {
						memLimit = wpMem
					} else if siteType == "woocommerce" {
						memLimit = "256M"
					} else {
						memLimit = "128M"
					}

					fsLocked := server.IsDirectoryLocked(domainPath)
					importedSite := config.SiteConfig{
						Domain:       domain,
						PHPVersion:   phpVersion,
						PublicDir:    publicDir,
						DatabaseName: dbName,
						DatabaseUser: dbUser,
						DatabasePass: dbPass,
						SystemUser:   systemUser,
						IsLocked:     fsLocked,
						Type:         siteType,
						MemoryLimit:  memLimit,
					}

					s.Sites = append(s.Sites, importedSite)
					ui.PrintInfo(fmt.Sprintf("Imported pre-existing site: %s (Type: %s, PHP %s, Memory %s, DB %s, Locked: %v)", domain, siteType, phpVersion, memLimit, dbName, fsLocked))
				}
			}
		}

		ui.PrintStep(2, "Regenerating PHP FPM configurations...")
		for _, site := range s.Sites {
			if site.Type == "html" {
				continue
			}
			ui.PrintInfo(fmt.Sprintf("Syncing PHP pool for site: %s", site.Domain))
			// Clean up any orphaned pool files for this domain in other PHP versions
			_, _ = server.DeleteOrphanedPHPPools(site.Domain, site.PHPVersion, s.Global.SupportedPHPVersions)

			siteCopy := site
			if err := server.WritePHPPool(&siteCopy); err != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to write PHP pool config for %s: %v", site.Domain, err))
			}
			if err := server.ReloadPHP(site.PHPVersion); err != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to reload PHP FPM version %s: %v", site.PHPVersion, err))
			}
		}

		ui.PrintStep(3, "Regenerating global Caddyfile...")
		if err := server.WriteCaddyfile(s); err != nil {
			return fmt.Errorf("failed to write Caddyfile: %w", err)
		}

		ui.PrintStep(4, "Reloading Caddy service...")
		if err := server.ReloadCaddy(s); err != nil {
			return fmt.Errorf("failed to reload Caddy: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Trigger telemetry ping
	if s, err := config.ReadState(statePath); err == nil {
		config.PingAsync(s)
	}

	ui.PrintSuccess("Synchronization Completed")
	return nil
}
