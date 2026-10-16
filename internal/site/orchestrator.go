package site

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

var domainRegex = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*\.[a-z]{2,63}$`)

// ValidateDomain checks if a domain name has a valid format.
func ValidateDomain(domain string) error {
	if !domainRegex.MatchString(domain) {
		return fmt.Errorf("invalid domain format: %s", domain)
	}
	return nil
}

// SanitizeUser generates a safe Linux system username from the domain.
func SanitizeUser(domain string) string {
	sanitized := strings.ReplaceAll(domain, ".", "_")
	sanitized = strings.ReplaceAll(sanitized, "-", "_")
	username := "wp_" + sanitized
	if len(username) > 30 {
		username = username[:30]
	}
	return username
}

// promptLine reads a non-empty line from stdin with a prompt label.
func promptLine(label string) string {
	fmt.Printf("  %s%s%s  ", ui.Cyan, label, ui.Reset)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		v := strings.TrimSpace(scanner.Text())
		if v != "" {
			return v
		}
	}
	return ""
}

// Create provisions a site's infrastructure.
func Create(domain string, phpVersion string, siteType string, dbOpt string, importFiles string, importDB string, wpUser string, wpPass string, wpEmail string, wpName string, memoryOpt ...string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	siteType = strings.ToLower(strings.TrimSpace(siteType))
	if siteType == "" {
		siteType = "wp"
	}
	if siteType != "wp" && siteType != "woocommerce" && siteType != "laravel" && siteType != "php" && siteType != "html" {
		return fmt.Errorf("invalid site type '%s' (supported: wp, woocommerce, laravel, php, html)", siteType)
	}

	var memoryLimit string
	if len(memoryOpt) > 0 {
		memoryLimit = strings.TrimSpace(memoryOpt[0])
	}
	if memoryLimit == "" {
		if siteType == "woocommerce" {
			memoryLimit = "256M"
		} else {
			memoryLimit = "128M"
		}
	} else {
		u := strings.ToUpper(memoryLimit)
		if !strings.HasSuffix(u, "M") && !strings.HasSuffix(u, "G") {
			u = u + "M"
		}
		memoryLimit = u
	}

	maxExecutionTime := 60
	maxInputVars := 1000
	if memoryLimit == "256M" || memoryLimit == "512M" || siteType == "woocommerce" {
		maxExecutionTime = 300
		maxInputVars = 5000
	}

	installWP := siteType == "wp" || siteType == "woocommerce"

	// Prompt for WordPress admin credentials BEFORE the locked state transaction
	var wpAdminUser, wpAdminEmail, wpAdminPassword string
	if installWP {
		if wpUser != "" {
			wpAdminUser = wpUser
			wpAdminEmail = wpEmail
			wpAdminPassword = wpPass
		} else if os.Getenv("AGILEPANEL_TEST_MODE") == "true" {
			// In test mode, skip interactive prompts and use safe defaults
			wpAdminUser = "testadmin"
			wpAdminEmail = "testadmin@" + domain
		} else {
			ui.Banner("WordPress Admin Setup")
			ui.PrintInfo("Enter the details for the WordPress administrator account.")
			ui.PrintInfo("These credentials will be used to log in to wp-admin.")
			fmt.Println()
			wpAdminName := promptLine("Full Name         :")
			if wpAdminName == "" {
				return fmt.Errorf("admin full name cannot be empty")
			}
			wpAdminUser = promptLine("Username          :")
			if wpAdminUser == "" {
				return fmt.Errorf("admin username cannot be empty")
			}
			wpAdminEmail = promptLine("Email Address     :")
			if wpAdminEmail == "" {
				return fmt.Errorf("admin email cannot be empty")
			}
			_ = wpAdminName // stored for display only; WP-CLI uses user+email
			fmt.Println()
		}
	}

	statePath := config.GetStatePath()
	var dbPassword string
	var dbName string
	var dbUser string

	err := config.WithLockedState(statePath, func(s *config.State) error {
		// 1. Check for duplicate site
		for _, site := range s.Sites {
			if strings.EqualFold(site.Domain, domain) {
				return fmt.Errorf("site for domain %s already exists", domain)
			}
		}

		// 2. Validate PHP version
		if phpVersion == "" {
			phpVersion = s.Global.DefaultPHPVersion
		}
		phpValid := false
		for _, v := range s.Global.SupportedPHPVersions {
			if v == phpVersion {
				phpValid = true
				break
			}
		}
		if !phpValid {
			return fmt.Errorf("unsupported PHP version: %s (supported: %s)", phpVersion, strings.Join(s.Global.SupportedPHPVersions, ", "))
		}

		// 3. Generate credentials and users
		systemUser := SanitizeUser(domain)
		
		var dbPrefix string
		var err error
		dbPrefix, err = server.GenerateSecurePrefix(6)
		if err != nil {
			return fmt.Errorf("failed to generate database prefix: %w", err)
		}
		
		sanitizedDomain := strings.ReplaceAll(strings.ReplaceAll(domain, ".", "_"), "-", "_")
		dbName = fmt.Sprintf("db_%s_%s", dbPrefix, sanitizedDomain)
		dbUser = fmt.Sprintf("usr_%s_%s", dbPrefix, sanitizedDomain)
		if len(dbUser) > 16 {
			dbUser = dbUser[:16]
		}

		dbPassword, err = server.GenerateSecurePassword()
		if err != nil {
			return fmt.Errorf("failed to generate database password: %w", err)
		}

		// 4. Create system user and user group
		if err := server.CreateSystemUser(systemUser); err != nil {
			return fmt.Errorf("failed to create system user: %w", err)
		}

		// Determine public root folder
		publicDir := fmt.Sprintf("/var/www/%s/htdocs", domain)
		if siteType == "laravel" {
			publicDir = fmt.Sprintf("/var/www/%s/htdocs/public", domain)
		}

		// 5. Provision and set permissions on web folders
		if err := server.ProvisionSiteDirectory(publicDir, systemUser); err != nil {
			_ = server.DeleteSystemUser(systemUser)
			return fmt.Errorf("failed to provision web folders: %w", err)
		}

		// 6. Create Database (depends on dbOpt and siteType)
		var createDB bool
		if dbOpt == "true" {
			createDB = true
		} else if dbOpt == "false" {
			createDB = false
		} else {
			createDB = siteType != "html"
		}
		if createDB {
			if err := server.CreateDatabase(dbName, dbUser, dbPassword); err != nil {
				_ = server.DeleteSiteDirectory(publicDir)
				_ = server.DeleteSystemUser(systemUser)
				return fmt.Errorf("database setup failed: %w", err)
			}
		}

		// 7. Write PHP-FPM Pool config
		newSite := config.SiteConfig{
			Domain:           strings.ToLower(domain),
			PHPVersion:       phpVersion,
			PublicDir:        publicDir,
			DatabaseName:     dbName,
			DatabaseUser:     dbUser,
			DatabasePass:     dbPassword,
			SystemUser:       systemUser,
			IsLocked:         false,
			Type:             siteType,
			MemoryLimit:      memoryLimit,
			MaxExecutionTime: maxExecutionTime,
			MaxInputVars:     maxInputVars,
		}
		if !createDB {
			newSite.DatabaseName = ""
			newSite.DatabaseUser = ""
			newSite.DatabasePass = ""
		}

		if err := server.WritePHPPool(&newSite); err != nil {
			if createDB {
				_ = server.DeleteDatabase(dbName, dbUser)
			}
			_ = server.DeleteSiteDirectory(publicDir)
			_ = server.DeleteSystemUser(systemUser)
			return fmt.Errorf("failed to write PHP-FPM config: %w", err)
		}

		// 8. Install landing layouts depending on type OR import from files/db
		hasImports := importFiles != "" || importDB != ""
		if hasImports {
			htdocsPath := filepath.Join(server.GetSiteRootDir(publicDir), "htdocs")
			if importFiles != "" {
				if _, err := os.Stat(importFiles); err == nil {
					ui.PrintStep(8, "Importing web public files from ZIP...")
					if runtime.GOOS == "linux" {
						_ = os.MkdirAll(htdocsPath, 0755)
						unzipCmd := exec.Command("unzip", "-o", "-q", importFiles, "-d", htdocsPath)
						if err := unzipCmd.Run(); err != nil {
							_ = server.DeletePHPPool(phpVersion, domain)
							if createDB {
								_ = server.DeleteDatabase(dbName, dbUser)
							}
							_ = server.DeleteSiteDirectory(publicDir)
							_ = server.DeleteSystemUser(systemUser)
							return fmt.Errorf("failed to extract import files ZIP: %w", err)
						}
					} else {
						fmt.Printf("Mock: Unzipped %s to %s\n", importFiles, htdocsPath)
					}
				} else {
					_ = server.DeletePHPPool(phpVersion, domain)
					if createDB {
						_ = server.DeleteDatabase(dbName, dbUser)
					}
					_ = server.DeleteSiteDirectory(publicDir)
					_ = server.DeleteSystemUser(systemUser)
					return fmt.Errorf("import files ZIP not found: %s", importFiles)
				}
			}

			if importDB != "" && createDB {
				if _, err := os.Stat(importDB); err == nil {
					ui.PrintStep(9, "Importing database backup...")
					sqlFile := importDB
					var tempDir string

					if strings.HasSuffix(strings.ToLower(importDB), ".zip") {
						tempDir = filepath.Join(os.TempDir(), fmt.Sprintf("import_db_%s_%d", domain, time.Now().Unix()))
						_ = os.MkdirAll(tempDir, 0755)
						defer os.RemoveAll(tempDir)

						if runtime.GOOS == "linux" {
							cmd := exec.Command("unzip", "-o", "-q", importDB, "-d", tempDir)
							if err := cmd.Run(); err == nil {
								entries, err := os.ReadDir(tempDir)
								if err == nil {
									for _, entry := range entries {
										if strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
											sqlFile = filepath.Join(tempDir, entry.Name())
											break
										}
									}
								}
							}
						} else {
							mockSql := filepath.Join(tempDir, "mock.sql")
							_ = os.WriteFile(mockSql, []byte("CREATE TABLE mock (id int);"), 0644)
							sqlFile = mockSql
						}
					}

					if runtime.GOOS == "linux" {
						mysqlCmd := exec.Command("mysql", "-u"+dbUser, dbName)
						mysqlCmd.Env = append(os.Environ(), "MYSQL_PWD="+dbPassword)
						f, err := os.Open(sqlFile)
						if err != nil {
							_ = server.DeletePHPPool(phpVersion, domain)
							_ = server.DeleteDatabase(dbName, dbUser)
							_ = server.DeleteSiteDirectory(publicDir)
							_ = server.DeleteSystemUser(systemUser)
							return fmt.Errorf("failed to open import SQL file: %w", err)
						}
						defer f.Close()
						mysqlCmd.Stdin = f

						var stderr bytes.Buffer
						mysqlCmd.Stderr = &stderr
						if err := mysqlCmd.Run(); err != nil {
							_ = server.DeletePHPPool(phpVersion, domain)
							_ = server.DeleteDatabase(dbName, dbUser)
							_ = server.DeleteSiteDirectory(publicDir)
							_ = server.DeleteSystemUser(systemUser)
							return fmt.Errorf("database import failed: %w (stderr: %s)", err, stderr.String())
						}
					} else {
						fmt.Printf("Mock: Imported database dump %s into db %s\n", sqlFile, dbName)
					}
				} else {
					_ = server.DeletePHPPool(phpVersion, domain)
					_ = server.DeleteDatabase(dbName, dbUser)
					_ = server.DeleteSiteDirectory(publicDir)
					_ = server.DeleteSystemUser(systemUser)
					return fmt.Errorf("import db file not found: %s", importDB)
				}
			}

			if runtime.GOOS == "linux" {
				_ = exec.Command("chown", "-R", systemUser+":www-data", htdocsPath).Run()
				_ = exec.Command("chmod", "-R", "0755", htdocsPath).Run()
			}
		} else if installWP {
			wpAdminPassword, err = server.InstallWordPress(
				systemUser,
				domain,
				publicDir,
				dbName,
				dbUser,
				dbPassword,
				s.Global.RedisSocketPath,
				wpAdminUser,
				wpAdminEmail,
				wpAdminPassword,
				siteType,
			)
			if err != nil {
				_ = server.DeletePHPPool(phpVersion, domain)
				_ = server.DeleteDatabase(dbName, dbUser)
				_ = server.DeleteSiteDirectory(publicDir)
				_ = server.DeleteSystemUser(systemUser)
				return fmt.Errorf("WordPress installation failed: %w", err)
			}
			if memoryLimit != "" {
				_ = server.TuneWordPressMemory(systemUser, publicDir, memoryLimit)
			}
		} else {
			htdocsPath := filepath.Join(server.GetSiteRootDir(publicDir), "htdocs")
			if siteType == "html" {
				indexPath := filepath.Join(htdocsPath, "index.html")
				_ = os.WriteFile(indexPath, []byte(renderHTMLWelcome(domain)), 0644)
			} else if siteType == "php" {
				indexPath := filepath.Join(htdocsPath, "index.php")
				_ = os.WriteFile(indexPath, []byte(renderPHPWelcome(domain, dbName, dbUser)), 0644)
			} else if siteType == "laravel" {
				publicPath := filepath.Join(htdocsPath, "public")
				_ = os.MkdirAll(publicPath, 0755)
				indexPath := filepath.Join(publicPath, "index.php")
				_ = os.WriteFile(indexPath, []byte(renderLaravelWelcome(domain, dbName, dbUser)), 0644)
			}

			// Apply correct ownership and permissions
			_ = server.FixPermissions(server.GetSiteRootDir(publicDir), systemUser)
		}

		// 9a. Persist admin credentials in site config
		if installWP {
			newSite.WPAdminUser = wpAdminUser
			newSite.WPAdminEmail = wpAdminEmail
		}

		// 9. Append configuration to state
		s.Sites = append(s.Sites, newSite)

		// 10. Regenerate Caddyfile
		if err := server.WriteCaddyfile(s); err != nil {
			return fmt.Errorf("failed to write Caddyfile: %w", err)
		}

		// 11. Reload PHP and Caddy service configs
		if err := server.ReloadPHP(phpVersion); err != nil {
			fmt.Printf("Warning: Failed to reload PHP service: %v\n", err)
		}
		if err := server.ReloadCaddy(s); err != nil {
			fmt.Printf("Warning: Failed to reload Caddy service: %v\n", err)
		}

		fmt.Printf("State: Added site configuration for %s successfully.\n", domain)
		return nil
	})

	if err != nil {
		return err
	}

	// Trigger telemetry ping
	if s, err := config.ReadState(statePath); err == nil {
		config.PingAsync(s)
	}

	// ── Pretty summary ────────────────────────────────────────────────────────
	ui.PrintSuccess("Site Created Successfully")

	serverIP := server.ResolvePublicIP()
	stagingURL := fmt.Sprintf("http://%s.%s.sslip.io", domain, serverIP)

	ui.SectionHeader("SITE & STAGING")
	ui.Row("Domain", domain)
	ui.Row("Staging URL", stagingURL)
	ui.Row("PHP Version", phpVersion)
	ui.Row("PHP Memory Limit", memoryLimit)
	ui.Row("System User", SanitizeUser(domain))
	ui.Row("Public Directory", fmt.Sprintf("/var/www/%s/htdocs", domain))

	// Pre-flight DNS check
	resolvedIP, dnsMatches, dnsErr := CheckDomainDNS(domain)
	if dnsErr != nil {
		ui.RowBadge("DNS Status", "▲ Record Missing", ui.BrightYellow)
	} else if !dnsMatches {
		ui.RowBadge("DNS Status", fmt.Sprintf("▲ Resolves to %s", resolvedIP), ui.BrightYellow)
	} else {
		ui.RowBadge("DNS Status", fmt.Sprintf("● Points to this Server (%s)", serverIP), ui.BrightGreen)
	}

	ui.SectionHeader("DATABASE")
	ui.Row("Name", dbName)
	ui.Row("User", dbUser)
	ui.Row("Password", dbPassword)

	if installWP {
		ui.SectionHeader("WORDPRESS ADMIN")
		ui.Row("Username", wpAdminUser)
		ui.Row("Email", wpAdminEmail)
		ui.Row("Password", wpAdminPassword)
		ui.Row("Login URL", "https://"+domain+"/wp-admin")
		ui.Row("Staging Login", stagingURL+"/wp-admin")
	}

	ui.Divider()
	ui.PrintInfo("Save these credentials — the password cannot be retrieved later.")
	if dnsErr != nil {
		ui.PrintWarning(fmt.Sprintf("DNS Notice: Could not resolve %s. Point an A-record to %s for SSL to activate.", domain, serverIP))
	} else if !dnsMatches {
		ui.PrintWarning(fmt.Sprintf("DNS Notice: %s currently resolves to %s (server IP: %s). Point your domain's A-record to this VPS to activate Let's Encrypt SSL.", domain, resolvedIP, serverIP))
	}
	fmt.Println()

	return nil
}

// Delete removes a site's infrastructure.
func Delete(domain string) error {
	statePath := config.GetStatePath()

	err := config.WithLockedState(statePath, func(s *config.State) error {
		foundIdx := -1
		var targetSite config.SiteConfig
		for i, site := range s.Sites {
			if strings.EqualFold(site.Domain, domain) {
				foundIdx = i
				targetSite = site
				break
			}
		}

		if foundIdx == -1 {
			return fmt.Errorf("site %s not found in state", domain)
		}

		// 1. Remove immutable attribute first if locked
		if targetSite.IsLocked {
			if err := server.UnlockDirectory(targetSite.PublicDir); err != nil {
				fmt.Printf("Warning: Failed to unlock site directory: %v\n", err)
			}
		}

		// 2. Drop database and user
		if err := server.DeleteDatabase(targetSite.DatabaseName, targetSite.DatabaseUser); err != nil {
			fmt.Printf("Warning: Database deletion failed: %v\n", err)
		}

		// 3. Delete PHP-FPM pool config file
		if err := server.DeletePHPPool(targetSite.PHPVersion, targetSite.Domain); err != nil {
			fmt.Printf("Warning: Failed to delete PHP pool config: %v\n", err)
		}

		// 4. Delete site directories
		if err := server.DeleteSiteDirectory(targetSite.PublicDir); err != nil {
			fmt.Printf("Warning: Failed to delete site folder structure: %v\n", err)
		}

		// 5. Delete Linux system user and user group
		if err := server.DeleteSystemUser(targetSite.SystemUser); err != nil {
			fmt.Printf("Warning: Failed to delete system user: %v\n", err)
		}

		// 6. Remove site from memory state
		s.Sites = append(s.Sites[:foundIdx], s.Sites[foundIdx+1:]...)

		// 7. Regenerate Caddyfile without this site
		if err := server.WriteCaddyfile(s); err != nil {
			fmt.Printf("Warning: Failed to write updated Caddyfile: %v\n", err)
		}

		// 8. Reload Caddy and PHP
		if err := server.ReloadPHP(targetSite.PHPVersion); err != nil {
			fmt.Printf("Warning: Failed to reload PHP service: %v\n", err)
		}
		if err := server.ReloadCaddy(s); err != nil {
			fmt.Printf("Warning: Failed to reload Caddy service: %v\n", err)
		}

		fmt.Printf("State: Removed site configuration for %s successfully.\n", domain)
		return nil
	})

	if err != nil {
		return err
	}

	// Trigger telemetry ping
	if s, err := config.ReadState(statePath); err == nil {
		config.PingAsync(s)
	}

	ui.PrintSuccess("Site Deleted Successfully")
	ui.PrintInfo("AgilePanel has completely decommissioned " + domain + ". The isolated system user and group were deleted, the MariaDB database was dropped, Caddy virtual host configurations were removed, and all public files were securely erased from disk.")
	ui.Divider()
	fmt.Println()
	return nil
}

// Lock marks the site folder as immutable and updates the state.
func Lock(domain string) error {
	statePath := config.GetStatePath()

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

		// Execute system level chattr lock
		if err := server.LockDirectory(s.Sites[foundIdx].PublicDir); err != nil {
			return fmt.Errorf("failed to lock file directory: %w", err)
		}

		s.Sites[foundIdx].IsLocked = true
		fmt.Printf("State: Site %s marked as locked.\n", domain)
		return nil
	})

	if err != nil {
		return err
	}

	ui.PrintSuccess("Site Directory Locked")
	ui.PrintInfo("AgilePanel has marked the file directory of " + domain + " as read-only/immutable. This stops all write operations in the webroot, protecting your WordPress core files from unauthorized creation, modification, or deletion by external attackers.")
	ui.Divider()
	fmt.Println()
	return nil
}

// Unlock removes the site folder's immutable attributes and updates the state.
func Unlock(domain string) error {
	statePath := config.GetStatePath()

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

		// Execute system level chattr unlock
		if err := server.UnlockDirectory(s.Sites[foundIdx].PublicDir); err != nil {
			return fmt.Errorf("failed to unlock file directory: %w", err)
		}

		s.Sites[foundIdx].IsLocked = false
		fmt.Printf("State: Site %s marked as unlocked.\n", domain)
		return nil
	})

	if err != nil {
		return err
	}

	ui.PrintSuccess("Site Directory Unlocked")
	ui.PrintInfo("AgilePanel has removed the immutable file attribute for " + domain + ". Standard write permissions are restored, allowing you to run WordPress core upgrades, install plugins, update themes, and edit files normally.")
	ui.Divider()
	fmt.Println()
	return nil
}

// CacheClean flushes various types of caches: WordPress, Redis, PHP OPcache, and Caddy edge.
func CacheClean(domain string, cleanWP, cleanRedis, cleanOpcache, cleanCaddy bool) error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return err
	}

	var targetSite config.SiteConfig
	found := false
	for _, site := range state.Sites {
		if strings.EqualFold(site.Domain, domain) {
			targetSite = site
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("site %s not found in state", domain)
	}

	homeDir := server.GetSiteRootDir(targetSite.PublicDir)

	// 1. Flush WordPress internal cache
	if cleanWP {
		fmt.Println("WP-CLI: Flushing WordPress internal cache & transients...")
		err = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "cache", "flush", "--path="+targetSite.PublicDir)
		if err != nil {
			fmt.Printf("Warning: Failed to flush WordPress internal cache: %v\n", err)
		} else {
			fmt.Println("WordPress: Internal cache & transients flushed.")
		}
	}

	// 2. Flush Redis Object Cache
	if cleanRedis {
		fmt.Println("WP-CLI: Flushing Redis Object Cache...")
		err = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "redis", "flush", "--path="+targetSite.PublicDir)
		if err != nil {
			fmt.Printf("Warning: Failed to flush Redis Object Cache: %v\n", err)
		} else {
			fmt.Println("Redis: Object cache flushed.")
		}
	}

	// 3. Reset PHP OPcache (by reloading the PHP version service)
	if cleanOpcache {
		fmt.Println("PHP: Resetting PHP OPcache...")
		if err := server.ReloadPHP(targetSite.PHPVersion); err != nil {
			fmt.Printf("Warning: Failed to reload PHP for OPcache reset: %v\n", err)
		} else {
			fmt.Println("PHP: OPcache reset successfully.")
		}
	}

	// 4. Clear Caddy edge cache (reload config resets in-memory Souin cache)
	if cleanCaddy {
		fmt.Println("Caddy: Clearing Caddy page cache...")
		if err := server.ReloadCaddy(state); err != nil {
			fmt.Printf("Warning: Failed to reload Caddy to clear cache: %v\n", err)
		} else {
			fmt.Println("Caddy: Page cache cleared.")
		}
	}

	ui.PrintSuccess("Cache Cleared Successfully")
	ui.PrintInfo("AgilePanel has flushed the requested caching layers (WordPress transients, Redis database queries, PHP FPM bytecode OPcache, and Caddy reverse proxy page cache). All visitors will now see the latest updates instantly.")
	ui.Divider()
	fmt.Println()
	return nil
}

// Reinstall deletes and re-provisions a site's WordPress files and database credentials.
func Reinstall(domain string) error {
	statePath := config.GetStatePath()
	
	err := config.WithLockedState(statePath, func(s *config.State) error {
		foundIdx := -1
		var targetSite config.SiteConfig
		for i, site := range s.Sites {
			if strings.EqualFold(site.Domain, domain) {
				foundIdx = i
				targetSite = site
				break
			}
		}

		if foundIdx == -1 {
			return fmt.Errorf("site %s not found in state", domain)
		}

		// 1. Unlock if site is locked
		if targetSite.IsLocked {
			if err := server.UnlockDirectory(targetSite.PublicDir); err != nil {
				return fmt.Errorf("failed to unlock site for reinstallation: %w", err)
			}
		}

		// 2. Deletes public folders
		if err := server.DeleteSiteDirectory(targetSite.PublicDir); err != nil {
			return err
		}

		// 3. Recreate public folder and set permissions
		if err := server.ProvisionSiteDirectory(targetSite.PublicDir, targetSite.SystemUser); err != nil {
			return err
		}

		// 4. Generate new DB password (only if database exists)
		dbPassword := ""
		var err error
		createDB := targetSite.DatabaseName != ""
		if createDB {
			dbPassword, err = server.GenerateSecurePassword()
			if err != nil {
				return err
			}
			// 5. Update user and database credentials
			if err := server.CreateDatabase(targetSite.DatabaseName, targetSite.DatabaseUser, dbPassword); err != nil {
				return err
			}
			s.Sites[foundIdx].DatabasePass = dbPassword
		}

		// 6. Install layouts depending on type
		var wpAdminPassword string
		adminUser := targetSite.WPAdminUser
		isWP := targetSite.Type == "wp" || targetSite.Type == "woocommerce" || targetSite.Type == ""
		if isWP {
			if adminUser == "" {
				adminUser = "admin"
			}
			adminEmail := targetSite.WPAdminEmail
			if adminEmail == "" {
				adminEmail = "admin@" + targetSite.Domain
			}
			sType := targetSite.Type
			if sType == "" {
				sType = "wp"
			}
			wpAdminPassword, err = server.InstallWordPress(
				targetSite.SystemUser,
				targetSite.Domain,
				targetSite.PublicDir,
				targetSite.DatabaseName,
				targetSite.DatabaseUser,
				dbPassword,
				s.Global.RedisSocketPath,
				adminUser,
				adminEmail,
				"",
				sType,
			)
			if err != nil {
				return err
			}
		} else {
			htdocsPath := filepath.Join(server.GetSiteRootDir(targetSite.PublicDir), "htdocs")
			if targetSite.Type == "html" {
				indexPath := filepath.Join(htdocsPath, "index.html")
				_ = os.WriteFile(indexPath, []byte(renderHTMLWelcome(targetSite.Domain)), 0644)
			} else if targetSite.Type == "php" {
				indexPath := filepath.Join(htdocsPath, "index.php")
				_ = os.WriteFile(indexPath, []byte(renderPHPWelcome(targetSite.Domain, targetSite.DatabaseName, targetSite.DatabaseUser)), 0644)
			} else if targetSite.Type == "laravel" {
				publicPath := filepath.Join(htdocsPath, "public")
				_ = os.MkdirAll(publicPath, 0755)
				indexPath := filepath.Join(publicPath, "index.php")
				_ = os.WriteFile(indexPath, []byte(renderLaravelWelcome(targetSite.Domain, targetSite.DatabaseName, targetSite.DatabaseUser)), 0644)
			}
			// Apply correct ownership and permissions
			_ = server.FixPermissions(server.GetSiteRootDir(targetSite.PublicDir), targetSite.SystemUser)
		}

		// 7. Re-lock if it was locked originally
		if targetSite.IsLocked {
			if err := server.LockDirectory(targetSite.PublicDir); err != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to re-lock site directory: %v", err))
			}
		}

		ui.PrintSuccess("Reinstall Complete")
		ui.SectionHeader("SITE")
		ui.Row("Domain", domain)
		ui.Row("Type", targetSite.Type)
		if createDB {
			ui.SectionHeader("DATABASE")
			ui.Row("New Password", dbPassword)
		}
		if isWP {
			ui.SectionHeader("WORDPRESS ADMIN")
			ui.Row("Username", adminUser)
			ui.Row("New Password", wpAdminPassword)
			ui.Row("Login URL", "https://"+domain+"/wp-admin")
			ui.Divider()
			ui.PrintInfo("WordPress has been successfully reinstalled! AgilePanel deleted the old public folders, dropped and re-provisioned the database tables, and performed a fresh WordPress Core installation including the Redis Cache plugin.")
		} else {
			ui.Divider()
			ui.PrintInfo("The site has been successfully reinstalled! AgilePanel deleted the old folders, re-provisioned the database credentials (if applicable), and restored the default landing index files.")
		}
		fmt.Println()

		return nil
	})

	return err
}



// FixPermissions restores correct owners and permissions for all files and directories of the site.
func FixPermissions(domain string) error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return err
	}

	var targetSite config.SiteConfig
	found := false
	for _, site := range state.Sites {
		if strings.EqualFold(site.Domain, domain) {
			targetSite = site
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("site %s not found in state", domain)
	}

	parentDir := server.GetSiteRootDir(targetSite.PublicDir)
	err = server.FixPermissions(parentDir, targetSite.SystemUser)
	if err != nil {
		return err
	}

	ui.PrintSuccess("Permissions Restored")
	ui.PrintInfo("AgilePanel has recursively updated file permissions (0644) and directory permissions (0755) under /var/www/" + domain + " to match the isolated user " + targetSite.SystemUser + " and group. This corrects typical '403 Forbidden' errors.")
	ui.Divider()
	fmt.Println()
	return nil
}



// List displays all sites currently registered in the panel state.
func List() error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return err
	}

	ui.Banner("Hosted Websites")

	if len(state.Sites) == 0 {
		ui.PrintInfo("No websites have been created yet on this server.")
		ui.PrintInfo("Run: " + ui.Accent("ap site create domain.com --wp") + " to deploy your first site.")
		fmt.Println()
		return nil
	}

	var cols = []ui.TableColumn{
		{Header: "Domain", Width: 22},
		{Header: "Type", Width: 12},
		{Header: "PHP", Width: 6},
		{Header: "System User", Width: 15},
		{Header: "Database", Width: 20},
		{Header: "Status", Width: 10},
	}

	var rows [][]string
	for _, site := range state.Sites {
		status := ui.BrightGreen + "Active" + ui.Reset
		if site.IsLocked {
			status = ui.BrightYellow + "Locked" + ui.Reset
		}
		sType := site.Type
		if sType == "" {
			sType = "wp"
		}
		db := site.DatabaseName
		if db == "" {
			db = "-"
		}
		rows = append(rows, []string{
			site.Domain,
			strings.ToUpper(sType),
			site.PHPVersion,
			site.SystemUser,
			db,
			status,
		})
	}

	ui.PrintTable(cols, rows)

	ui.Divider()
	fmt.Printf("  %s %d site(s) registered\n", ui.Muted("Total:"), len(state.Sites))
	fmt.Println()
	return nil
}

// Info shows comprehensive configuration details for a specific website, including SSL certificates.
func Info(domain string) error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return err
	}

	var targetSite config.SiteConfig
	found := false
	for _, s := range state.Sites {
		if strings.EqualFold(s.Domain, domain) {
			targetSite = s
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("site %s not found in state", domain)
	}

	parentDir := server.GetSiteRootDir(targetSite.PublicDir)
	configDir := filepath.Join(parentDir, "conf")
	backupDir := filepath.Join(parentDir, "backup")

	dbPassword := targetSite.DatabasePass
	if dbPassword == "" {
		dbPassword = ui.Muted("[not stored in state]")
	}

	sslInfo, err := GetSSLInfo(targetSite.Domain)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to retrieve SSL information: %v", err))
	}

	ui.Banner("Site Configuration: " + strings.ToUpper(targetSite.Domain))

	ui.SectionHeader("GENERAL")
	ui.Row("Domain", targetSite.Domain)
	ui.Row("PHP Version", targetSite.PHPVersion)
	ui.Row("System User", targetSite.SystemUser)
	ui.RowBadge("Status", func() string {
		if targetSite.IsLocked {
			return "Locked (Read-Only)"
		}
		return "Active"
	}(), func() string {
		if targetSite.IsLocked {
			return ui.BrightYellow
		}
		return ui.BrightGreen
	}())

	ui.SectionHeader("SSL / TLS")
	if sslInfo != nil && sslInfo.Active {
		ui.RowBadge("Status", "Active — "+sslInfo.Issuer, ui.BrightGreen)
		daysLeft := int(time.Until(sslInfo.Expiration).Hours() / 24)
		expColor := ui.BrightGreen
		if daysLeft < 14 {
			expColor = ui.BrightRed
		} else if daysLeft < 30 {
			expColor = ui.BrightYellow
		}
		ui.RowBadge("Expiry", fmt.Sprintf("%s  (%d days remaining)",
			sslInfo.Expiration.Format("2006-01-02"), daysLeft), expColor)
		ui.Row("Certificate", sslInfo.CertPath)
		ui.Row("Private Key", sslInfo.KeyPath)
	} else {
		ui.RowBadge("Status", "Inactive / Self-Signed", ui.BrightRed)
	}

	ui.SectionHeader("DIRECTORIES")
	ui.Row("Webroot", targetSite.PublicDir)
	ui.Row("Config", configDir)
	ui.Row("Backups", backupDir)

	ui.SectionHeader("DATABASE")
	ui.Row("Name", targetSite.DatabaseName)
	ui.Row("User", targetSite.DatabaseUser)
	ui.Row("Password", dbPassword)

	if targetSite.WPAdminUser != "" {
		ui.SectionHeader("WORDPRESS ADMIN")
		ui.Row("Username", targetSite.WPAdminUser)
		ui.Row("Email", targetSite.WPAdminEmail)
		ui.Row("Login URL", "https://"+targetSite.Domain+"/wp-admin")
	}

	ui.Divider()
	fmt.Println()
	return nil
}

// Edit opens the PHP-FPM pool configuration in the system's text editor and reloads the service.
func Edit(domain string) error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return err
	}

	var targetSite config.SiteConfig
	found := false
	for _, s := range state.Sites {
		if strings.EqualFold(s.Domain, domain) {
			targetSite = s
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("site %s not found in state", domain)
	}

	poolPath := server.GetPHPPoolPath(targetSite.PHPVersion, targetSite.Domain)

	// Determine editor to use
	editor := os.Getenv("EDITOR")
	if editor != "" {
		if strings.ContainsAny(editor, ";&|<>()$`\r\n") {
			return fmt.Errorf("invalid editor path: command separators or redirection detected")
		}
		baseEditor := filepath.Base(editor)
		allowedEditors := map[string]bool{
			"nano":        true,
			"vim":         true,
			"vi":          true,
			"notepad.exe": true,
			"notepad":     true,
			"emacs":       true,
		}
		if !allowedEditors[strings.ToLower(baseEditor)] {
			return fmt.Errorf("editor %s is not in the whitelist of safe editors", baseEditor)
		}
	}

	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad.exe"
		} else {
			// Try nano first, fallback to vi
			if _, err := exec.LookPath("nano"); err == nil {
				editor = "nano"
			} else {
				editor = "vi"
			}
		}
	}

	fmt.Printf("Opening PHP FPM pool configuration for %s in %s...\n", domain, editor)
	
	if os.Getenv("AGILEPANEL_TEST_MODE") != "true" {
		cmd := exec.Command(editor, poolPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run editor %s: %w", editor, err)
		}
	} else {
		fmt.Printf("Test Mode: Skipping launching editor %s for file %s\n", editor, poolPath)
	}

	fmt.Println("Reloading PHP FPM service to apply changes...")
	if err := server.ReloadPHP(targetSite.PHPVersion); err != nil {
		return fmt.Errorf("failed to reload PHP FPM: %w", err)
	}

	fmt.Println("Success: Web server settings updated successfully.")
	return nil
}
