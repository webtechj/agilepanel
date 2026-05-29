package site

import (
	"bufio"
	"bytes"
	"crypto/x509"
	"encoding/pem"
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
func Create(domain string, phpVersion string, siteType string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	siteType = strings.ToLower(strings.TrimSpace(siteType))
	if siteType == "" {
		siteType = "wp"
	}
	if siteType != "wp" && siteType != "laravel" && siteType != "php" && siteType != "html" {
		return fmt.Errorf("invalid site type '%s' (supported: wp, laravel, php, html)", siteType)
	}

	installWP := siteType == "wp"

	// Prompt for WordPress admin credentials BEFORE the locked state transaction
	var wpAdminUser, wpAdminEmail string
	if installWP {
		if os.Getenv("AGILEPANEL_TEST_MODE") == "true" {
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
	var wpAdminPassword string
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

		// 6. Create Database (skip for HTML)
		createDB := siteType == "wp" || siteType == "laravel" || siteType == "php"
		if createDB {
			if err := server.CreateDatabase(dbName, dbUser, dbPassword); err != nil {
				_ = server.DeleteSiteDirectory(publicDir)
				_ = server.DeleteSystemUser(systemUser)
				return fmt.Errorf("database setup failed: %w", err)
			}
		}

		// 7. Write PHP-FPM Pool config
		newSite := config.SiteConfig{
			Domain:       strings.ToLower(domain),
			PHPVersion:   phpVersion,
			PublicDir:    publicDir,
			DatabaseName: dbName,
			DatabaseUser: dbUser,
			DatabasePass: dbPassword,
			SystemUser:   systemUser,
			IsLocked:     false,
			Type:         siteType,
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

		// 8. Install landing layouts depending on type
		if installWP {
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
			)
			if err != nil {
				_ = server.DeletePHPPool(phpVersion, domain)
				_ = server.DeleteDatabase(dbName, dbUser)
				_ = server.DeleteSiteDirectory(publicDir)
				_ = server.DeleteSystemUser(systemUser)
				return fmt.Errorf("WordPress installation failed: %w", err)
			}
		} else {
			htdocsPath := filepath.Join(server.GetSiteRootDir(publicDir), "htdocs")
			if siteType == "html" {
				indexPath := filepath.Join(htdocsPath, "index.html")
				htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Welcome to %s</title>
    <style>
        body { font-family: 'Outfit', sans-serif; background: #0f172a; color: #f8fafc; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
        .card { background: rgba(30, 41, 59, 0.7); padding: 3rem; border-radius: 12px; border: 1px solid rgba(255, 255, 255, 0.1); text-align: center; max-width: 500px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
        h1 { color: #38bdf8; margin-top: 0; }
        p { color: #94a3b8; font-size: 1.1rem; line-height: 1.6; }
        .badge { background: #0ea5e9; padding: 0.25rem 0.75rem; border-radius: 9999px; font-size: 0.85rem; font-weight: bold; color: white; display: inline-block; }
    </style>
</head>
<body>
    <div class="card">
        <div class="badge">AgilePanel HTML</div>
        <h1>%s</h1>
        <p>Your static HTML website has been successfully provisioned and is online.</p>
        <p style="font-size: 0.9rem; color: #64748b;">Upload your static assets to <code>/var/www/%s/htdocs</code> to replace this page.</p>
    </div>
</body>
</html>`, domain, domain, domain)
				_ = os.WriteFile(indexPath, []byte(htmlContent), 0644)
			} else if siteType == "php" {
				indexPath := filepath.Join(htdocsPath, "index.php")
				phpContent := fmt.Sprintf(`<?php
$domain = $_SERVER['HTTP_HOST'];
?>
<!DOCTYPE html>
<html>
<head>
    <title>Welcome to <?php echo htmlspecialchars($domain); ?></title>
    <style>
        body { font-family: 'Outfit', sans-serif; background: #0f172a; color: #f8fafc; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
        .card { background: rgba(30, 41, 59, 0.7); padding: 3rem; border-radius: 12px; border: 1px solid rgba(255, 255, 255, 0.1); text-align: center; max-width: 500px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
        h1 { color: #10b981; margin-top: 0; }
        p { color: #94a3b8; font-size: 1.1rem; line-height: 1.6; }
        .badge { background: #10b981; padding: 0.25rem 0.75rem; border-radius: 9999px; font-size: 0.85rem; font-weight: bold; color: white; display: inline-block; }
        ul { text-align: left; background: #1e293b; padding: 1.5rem 1.5rem 1.5rem 2.5rem; border-radius: 8px; border: 1px solid rgba(255,255,255,0.05); color: #cbd5e1; font-family: monospace; }
        li { margin-bottom: 0.5rem; }
    </style>
</head>
<body>
    <div class="card">
        <div class="badge">AgilePanel PHP</div>
        <h1><?php echo htmlspecialchars($domain); ?></h1>
        <p>Your custom PHP environment is fully configured and online.</p>
        
        <h3>Database Credentials</h3>
        <ul>
            <li><strong>Host:</strong> localhost (UNIX Socket)</li>
            <li><strong>Database:</strong> %s</li>
            <li><strong>Username:</strong> %s</li>
            <li><strong>Password:</strong> %s</li>
        </ul>

        <div style="margin-top: 2rem; background: #020617; padding: 1rem; border-radius: 6px; font-size: 0.9rem; text-align: left;">
            <strong>Database Connection Test:</strong><br/>
            <?php
            $conn = @new mysqli('localhost', '%s', '%s', '%s');
            if ($conn->connect_error) {
                echo "<span style='color:#f87171;'>❌ Connection Failed: " . htmlspecialchars($conn->connect_error) . "</span>";
            } else {
                echo "<span style='color:#34d399;'>✔ Connection Successful!</span>";
                $conn->close();
            }
            ?>
        </div>
        
        <p style="font-size: 0.9rem; color: #64748b; margin-top: 2rem;">Webroot is at <code>/var/www/<?php echo htmlspecialchars($domain); ?>/htdocs</code></p>
    </div>
</body>
</html>`, dbName, dbUser, dbPassword, dbUser, dbPassword, dbName)
				_ = os.WriteFile(indexPath, []byte(phpContent), 0644)
			} else if siteType == "laravel" {
				publicPath := filepath.Join(htdocsPath, "public")
				_ = os.MkdirAll(publicPath, 0755)
				indexPath := filepath.Join(publicPath, "index.php")
				laravelContent := fmt.Sprintf(`<?php
$domain = $_SERVER['HTTP_HOST'];
?>
<!DOCTYPE html>
<html>
<head>
    <title>Laravel Ready - <?php echo htmlspecialchars($domain); ?></title>
    <style>
        body { font-family: 'Outfit', sans-serif; background: #0f172a; color: #f8fafc; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
        .card { background: rgba(30, 41, 59, 0.7); padding: 3rem; border-radius: 12px; border: 1px solid rgba(255, 255, 255, 0.1); text-align: center; max-width: 600px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
        h1 { color: #f43f5e; margin-top: 0; }
        p { color: #94a3b8; font-size: 1.1rem; line-height: 1.6; }
        .badge { background: #f43f5e; padding: 0.25rem 0.75rem; border-radius: 9999px; font-size: 0.85rem; font-weight: bold; color: white; display: inline-block; }
        pre { text-align: left; background: #020617; padding: 1.5rem; border-radius: 8px; border: 1px solid rgba(255,255,255,0.05); color: #38bdf8; font-size: 0.9rem; overflow-x: auto; }
    </style>
</head>
<body>
    <div class="card">
        <div class="badge">AgilePanel Laravel Ready</div>
        <h1>%s</h1>
        <p>Your server environment is fully configured for Laravel.</p>
        <p>To deploy your Laravel application, configure your <code>.env</code> file in <code>/var/www/%s/htdocs</code> with these settings:</p>
        
        <pre>DB_CONNECTION=mysql
DB_HOST=127.0.0.1
DB_PORT=3306
DB_DATABASE=%s
DB_USERNAME=%s
DB_PASSWORD=%s</pre>

        <p style="font-size: 0.9rem; color: #64748b; margin-top: 2rem;">Caddy is currently serving from: <code>/var/www/%s/htdocs/public</code></p>
    </div>
</body>
</html>`, domain, domain, dbName, dbUser, dbPassword, domain)
				_ = os.WriteFile(indexPath, []byte(laravelContent), 0644)
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

	ui.SectionHeader("SITE")
	ui.Row("Domain", domain)
	ui.Row("PHP Version", phpVersion)
	ui.Row("System User", SanitizeUser(domain))
	ui.Row("Public Directory", fmt.Sprintf("/var/www/%s/htdocs", domain))

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
	}

	ui.Divider()
	ui.PrintInfo("Save these credentials — the password cannot be retrieved later.")
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
		}

		// 6. Install layouts depending on type
		var wpAdminPassword string
		adminUser := targetSite.WPAdminUser
		isWP := targetSite.Type == "wp" || targetSite.Type == ""
		if isWP {
			if adminUser == "" {
				adminUser = "admin"
			}
			adminEmail := targetSite.WPAdminEmail
			if adminEmail == "" {
				adminEmail = "admin@" + targetSite.Domain
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
			)
			if err != nil {
				return err
			}
		} else {
			htdocsPath := filepath.Join(server.GetSiteRootDir(targetSite.PublicDir), "htdocs")
			if targetSite.Type == "html" {
				indexPath := filepath.Join(htdocsPath, "index.html")
				htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Welcome to %s</title>
    <style>
        body { font-family: 'Outfit', sans-serif; background: #0f172a; color: #f8fafc; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
        .card { background: rgba(30, 41, 59, 0.7); padding: 3rem; border-radius: 12px; border: 1px solid rgba(255, 255, 255, 0.1); text-align: center; max-width: 500px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
        h1 { color: #38bdf8; margin-top: 0; }
        p { color: #94a3b8; font-size: 1.1rem; line-height: 1.6; }
        .badge { background: #0ea5e9; padding: 0.25rem 0.75rem; border-radius: 9999px; font-size: 0.85rem; font-weight: bold; color: white; display: inline-block; }
    </style>
</head>
<body>
    <div class="card">
        <div class="badge">AgilePanel HTML</div>
        <h1>%s</h1>
        <p>Your static HTML website has been successfully provisioned and is online.</p>
        <p style="font-size: 0.9rem; color: #64748b;">Upload your static assets to <code>/var/www/%s/htdocs</code> to replace this page.</p>
    </div>
</body>
</html>`, targetSite.Domain, targetSite.Domain, targetSite.Domain)
				_ = os.WriteFile(indexPath, []byte(htmlContent), 0644)
			} else if targetSite.Type == "php" {
				indexPath := filepath.Join(htdocsPath, "index.php")
				phpContent := fmt.Sprintf(`<?php
$domain = $_SERVER['HTTP_HOST'];
?>
<!DOCTYPE html>
<html>
<head>
    <title>Welcome to <?php echo htmlspecialchars($domain); ?></title>
    <style>
        body { font-family: 'Outfit', sans-serif; background: #0f172a; color: #f8fafc; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
        .card { background: rgba(30, 41, 59, 0.7); padding: 3rem; border-radius: 12px; border: 1px solid rgba(255, 255, 255, 0.1); text-align: center; max-width: 500px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
        h1 { color: #10b981; margin-top: 0; }
        p { color: #94a3b8; font-size: 1.1rem; line-height: 1.6; }
        .badge { background: #10b981; padding: 0.25rem 0.75rem; border-radius: 9999px; font-size: 0.85rem; font-weight: bold; color: white; display: inline-block; }
        ul { text-align: left; background: #1e293b; padding: 1.5rem 1.5rem 1.5rem 2.5rem; border-radius: 8px; border: 1px solid rgba(255,255,255,0.05); color: #cbd5e1; font-family: monospace; }
        li { margin-bottom: 0.5rem; }
    </style>
</head>
<body>
    <div class="card">
        <div class="badge">AgilePanel PHP</div>
        <h1><?php echo htmlspecialchars($domain); ?></h1>
        <p>Your custom PHP environment is fully configured and online.</p>
        
        <h3>Database Credentials</h3>
        <ul>
            <li><strong>Host:</strong> localhost (UNIX Socket)</li>
            <li><strong>Database:</strong> %s</li>
            <li><strong>Username:</strong> %s</li>
            <li><strong>Password:</strong> %s</li>
        </ul>

        <div style="margin-top: 2rem; background: #020617; padding: 1rem; border-radius: 6px; font-size: 0.9rem; text-align: left;">
            <strong>Database Connection Test:</strong><br/>
            <?php
            $conn = @new mysqli('localhost', '%s', '%s', '%s');
            if ($conn->connect_error) {
                echo "<span style='color:#f87171;'>❌ Connection Failed: " . htmlspecialchars($conn->connect_error) . "</span>";
            } else {
                echo "<span style='color:#34d399;'>✔ Connection Successful!</span>";
                $conn->close();
            }
            ?>
        </div>
        
        <p style="font-size: 0.9rem; color: #64748b; margin-top: 2rem;">Webroot is at <code>/var/www/<?php echo htmlspecialchars($domain); ?>/htdocs</code></p>
    </div>
</body>
</html>`, targetSite.DatabaseName, targetSite.DatabaseUser, dbPassword, targetSite.DatabaseUser, dbPassword, targetSite.DatabaseName)
				_ = os.WriteFile(indexPath, []byte(phpContent), 0644)
			} else if targetSite.Type == "laravel" {
				publicPath := filepath.Join(htdocsPath, "public")
				_ = os.MkdirAll(publicPath, 0755)
				indexPath := filepath.Join(publicPath, "index.php")
				laravelContent := fmt.Sprintf(`<?php
$domain = $_SERVER['HTTP_HOST'];
?>
<!DOCTYPE html>
<html>
<head>
    <title>Laravel Ready - <?php echo htmlspecialchars($domain); ?></title>
    <style>
        body { font-family: 'Outfit', sans-serif; background: #0f172a; color: #f8fafc; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
        .card { background: rgba(30, 41, 59, 0.7); padding: 3rem; border-radius: 12px; border: 1px solid rgba(255, 255, 255, 0.1); text-align: center; max-width: 600px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
        h1 { color: #f43f5e; margin-top: 0; }
        p { color: #94a3b8; font-size: 1.1rem; line-height: 1.6; }
        .badge { background: #f43f5e; padding: 0.25rem 0.75rem; border-radius: 9999px; font-size: 0.85rem; font-weight: bold; color: white; display: inline-block; }
        pre { text-align: left; background: #020617; padding: 1.5rem; border-radius: 8px; border: 1px solid rgba(255,255,255,0.05); color: #38bdf8; font-size: 0.9rem; overflow-x: auto; }
    </style>
</head>
<body>
    <div class="card">
        <div class="badge">AgilePanel Laravel Ready</div>
        <h1>%s</h1>
        <p>Your server environment is fully configured for Laravel.</p>
        <p>To deploy your Laravel application, configure your <code>.env</code> file in <code>/var/www/%s/htdocs</code> with these settings:</p>
        
        <pre>DB_CONNECTION=mysql
DB_HOST=127.0.0.1
DB_PORT=3306
DB_DATABASE=%s
DB_USERNAME=%s
DB_PASSWORD=%s</pre>

        <p style="font-size: 0.9rem; color: #64748b; margin-top: 2rem;">Caddy is currently serving from: <code>/var/www/%s/htdocs/public</code></p>
    </div>
</body>
</html>`, targetSite.Domain, targetSite.Domain, targetSite.DatabaseName, targetSite.DatabaseUser, dbPassword, targetSite.Domain)
				_ = os.WriteFile(indexPath, []byte(laravelContent), 0644)
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

// SSLRenew deletes cached Caddy certificates for the domain to force immediate renewal on reload.
func SSLRenew(domain string) error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return err
	}

	found := false
	for _, site := range state.Sites {
		if strings.EqualFold(site.Domain, domain) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("site %s not found in state", domain)
	}

	certPaths := []string{
		fmt.Sprintf("/var/lib/caddy/.local/share/caddy/certificates/acme-v02.api.letsencrypt.org-directory/%s", domain),
		fmt.Sprintf("/var/lib/caddy/.local/share/caddy/certificates/acme.zerossl.com-v2-directory/%s", domain),
		fmt.Sprintf("/root/.local/share/caddy/certificates/acme-v02.api.letsencrypt.org-directory/%s", domain),
		fmt.Sprintf("/root/.local/share/caddy/certificates/acme.zerossl.com-v2-directory/%s", domain),
	}

	fmt.Printf("Caddy: Removing cached SSL credentials for %s...\n", domain)
	for _, path := range certPaths {
		if _, err := os.Stat(path); err == nil {
			if err := os.RemoveAll(path); err != nil {
				fmt.Printf("Warning: Failed to delete cert cache folder at %s: %v\n", path, err)
			} else {
				fmt.Printf("SSL: Removed cached certificates at %s\n", path)
			}
		}
	}

	fmt.Println("Caddy: Triggering Caddy configuration reload to force SSL renewal...")
	if err := server.ReloadCaddy(state); err != nil {
		return err
	}

	ui.PrintSuccess("SSL Renewal Triggered")
	ui.PrintInfo("AgilePanel has deleted the local SSL certificate cache for " + domain + " and triggered a Caddy configuration reload. Caddy will dynamically negotiate a new, valid SSL certificate with Let's Encrypt / ZeroSSL on the next connection.")
	ui.Divider()
	fmt.Println()
	return nil
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

// BackupDB exports the MySQL/MariaDB database dump to the secure backup folder.
func BackupDB(domain string) error {
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

	timestamp := time.Now().Format("20060102-150405")
	parentDir := server.GetSiteRootDir(targetSite.PublicDir) // /var/www/[domain]
	backupDir := filepath.Join(parentDir, "backup")
	backupFile := fmt.Sprintf("%s-%s.sql", domain, timestamp)
	backupPath := filepath.Join(backupDir, backupFile)

	if targetSite.Type == "html" || targetSite.DatabaseName == "" {
		return fmt.Errorf("site %s is a static HTML site and does not have a database", domain)
	}

	homeDir := server.GetSiteRootDir(targetSite.PublicDir)
	if targetSite.Type == "wp" || targetSite.Type == "" {
		fmt.Printf("WP-CLI: Exporting database dump for %s to %s...\n", domain, backupPath)
		err = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "db", "export", backupPath, "--path="+targetSite.PublicDir)
	} else {
		fmt.Printf("mysqldump: Exporting database dump for %s to %s...\n", domain, backupPath)
		if runtime.GOOS != "linux" {
			fmt.Printf("DB (Mock): mysqldump -u %s -p%s %s > %s\n", targetSite.DatabaseUser, targetSite.DatabasePass, targetSite.DatabaseName, backupPath)
		} else {
			cmd := exec.Command("mysqldump", "-u"+targetSite.DatabaseUser, "-p"+targetSite.DatabasePass, targetSite.DatabaseName)
			outFile, oErr := os.OpenFile(backupPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if oErr != nil {
				return fmt.Errorf("failed to create backup file: %w", oErr)
			}
			defer outFile.Close()
			cmd.Stdout = outFile

			var stderr bytes.Buffer
			cmd.Stderr = &stderr

			if err = cmd.Run(); err != nil {
				return fmt.Errorf("mysqldump failed: %w (stderr: %s)", err, stderr.String())
			}
			_ = exec.Command("chown", fmt.Sprintf("%s:caddy", targetSite.SystemUser), backupPath).Run()
		}
	}

	if err != nil {
		return fmt.Errorf("failed to export database: %w", err)
	}

	ui.PrintSuccess("Database Backup Completed")
	ui.PrintInfo("AgilePanel has successfully exported a secure MariaDB database SQL dump. The backup file is safely stored at " + backupPath + ".")
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

	for i, site := range state.Sites {
		var statusIcon string
		if site.IsLocked {
			statusIcon = ui.BrightYellow + "⊘" + ui.Reset
		} else {
			statusIcon = ui.BrightGreen + "●" + ui.Reset
		}

		fmt.Printf("  %s  %s\n", statusIcon, ui.Header(site.Domain))
		ui.Row("PHP Version", site.PHPVersion)
		ui.Row("System User", site.SystemUser)
		if site.DatabaseName != "" {
			ui.Row("Database Name", site.DatabaseName)
		} else {
			ui.Row("Database Name", "None (Static/PHP-Only)")
		}

		if i < len(state.Sites)-1 {
			fmt.Println()
		}
	}

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

type SSLInfo struct {
	CertPath   string
	KeyPath    string
	MetaPath   string
	Issuer     string
	Expiration time.Time
	Active     bool
}

// GetSSLInfo searches Caddy storage paths for certificate data.
func GetSSLInfo(domain string) (*SSLInfo, error) {
	if os.Getenv("AGILEPANEL_TEST_MODE") == "true" {
		return &SSLInfo{
			CertPath:   "/var/lib/caddy/.local/share/caddy/certificates/acme-v02.api.letsencrypt.org-directory/" + domain + "/" + domain + ".crt",
			KeyPath:    "/var/lib/caddy/.local/share/caddy/certificates/acme-v02.api.letsencrypt.org-directory/" + domain + "/" + domain + ".key",
			MetaPath:   "/var/lib/caddy/.local/share/caddy/certificates/acme-v02.api.letsencrypt.org-directory/" + domain + "/" + domain + ".json",
			Issuer:     "Let's Encrypt",
			Expiration: time.Now().AddDate(0, 2, 15),
			Active:     true,
		}, nil
	}

	caddyStoragePaths := []string{
		"/var/lib/caddy/.local/share/caddy/certificates",
		"/root/.local/share/caddy/certificates",
	}

	var foundCertPath string
	var foundKeyPath string
	var foundMetaPath string
	var issuer string

	for _, storagePath := range caddyStoragePaths {
		if _, err := os.Stat(storagePath); err != nil {
			continue
		}

		entries, err := os.ReadDir(storagePath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			domainPath := filepath.Join(storagePath, entry.Name(), domain)
			certFile := filepath.Join(domainPath, domain+".crt")
			keyFile := filepath.Join(domainPath, domain+".key")
			metaFile := filepath.Join(domainPath, domain+".json")

			if _, err := os.Stat(certFile); err == nil {
				foundCertPath = certFile
				foundKeyPath = keyFile
				foundMetaPath = metaFile
				issuer = entry.Name()
				if strings.Contains(issuer, "letsencrypt") {
					issuer = "Let's Encrypt"
				} else if strings.Contains(issuer, "zerossl") {
					issuer = "ZeroSSL"
				}
				break
			}
		}

		if foundCertPath != "" {
			break
		}
	}

	if foundCertPath == "" {
		return &SSLInfo{Active: false}, nil
	}

	certBytes, err := os.ReadFile(foundCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate: %w", err)
	}

	block, _ := pem.Decode(certBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse x509 certificate: %w", err)
	}

	return &SSLInfo{
		CertPath:   foundCertPath,
		KeyPath:    foundKeyPath,
		MetaPath:   foundMetaPath,
		Issuer:     issuer,
		Expiration: cert.NotAfter,
		Active:     true,
	}, nil
}

// SelfUpdate downloads the latest ap binary from GitHub to replace the running executable.
func SelfUpdate() error {
	if runtime.GOOS != "linux" {
		fmt.Println("Self-Update (Mock): Downloading latest ap-linux-amd64 to /usr/local/bin/ap")
		return nil
	}

	destPath := "/usr/local/bin/ap"
	url := "https://raw.githubusercontent.com/webtechj/agilepanel/main/ap-linux-amd64"

	fmt.Printf("Self-Update: Fetching latest binary from %s...\n", url)
	tmpFile := destPath + ".tmp"

	cmd := exec.Command("curl", "-L", "-o", tmpFile, url)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to download new binary: %w", err)
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
					for _, site := range s.Sites {
						if strings.EqualFold(site.Domain, domain) {
							alreadyTracked = true
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

					importedSite := config.SiteConfig{
						Domain:       domain,
						PHPVersion:   phpVersion,
						PublicDir:    publicDir,
						DatabaseName: dbName,
						DatabaseUser: dbUser,
						DatabasePass: dbPass,
						SystemUser:   systemUser,
						IsLocked:     false,
						Type:         siteType,
					}

					s.Sites = append(s.Sites, importedSite)
					ui.PrintInfo(fmt.Sprintf("Imported pre-existing site: %s (Type: %s, PHP %s, DB %s)", domain, siteType, phpVersion, dbName))
				}
			}
		}

		ui.PrintStep(2, "Regenerating PHP FPM configurations...")
		for _, site := range s.Sites {
			ui.PrintInfo(fmt.Sprintf("Syncing PHP pool for site: %s", site.Domain))
			if err := server.WritePHPPool(&site); err != nil {
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

// Backup creates a ZIP of the database and a separate ZIP of the site files.
func Backup(domain string) error {
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

	parentDir := server.GetSiteRootDir(targetSite.PublicDir) // /var/www/[domain]
	backupDir := filepath.Join(parentDir, "backup")
	_ = os.MkdirAll(backupDir, 0755)

	hasDB := targetSite.DatabaseName != "" && targetSite.Type != "html"
	dbSqlPath := filepath.Join(backupDir, fmt.Sprintf("%s-db.sql", domain))
	dbZipPath := filepath.Join(backupDir, fmt.Sprintf("%s-db.zip", domain))
	filesZipPath := filepath.Join(backupDir, fmt.Sprintf("%s-files.zip", domain))

	homeDir := server.GetSiteRootDir(targetSite.PublicDir)

	// 1. Export Database
	if hasDB {
		ui.PrintStep(1, "Exporting MariaDB database dump...")
		var err error
		if targetSite.Type == "wp" || targetSite.Type == "" {
			err = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "db", "export", dbSqlPath, "--path="+targetSite.PublicDir)
		} else {
			if runtime.GOOS != "linux" {
				fmt.Printf("DB (Mock): mysqldump -u %s -p%s %s > %s\n", targetSite.DatabaseUser, targetSite.DatabasePass, targetSite.DatabaseName, dbSqlPath)
			} else {
				cmd := exec.Command("mysqldump", "-u"+targetSite.DatabaseUser, "-p"+targetSite.DatabasePass, targetSite.DatabaseName)
				outFile, oErr := os.OpenFile(dbSqlPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
				if oErr != nil {
					return fmt.Errorf("failed to create backup file: %w", oErr)
				}
				defer outFile.Close()
				cmd.Stdout = outFile

				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				err = cmd.Run()
				if err != nil {
					err = fmt.Errorf("mysqldump failed: %w (stderr: %s)", err, stderr.String())
				} else {
					_ = exec.Command("chown", fmt.Sprintf("%s:caddy", targetSite.SystemUser), dbSqlPath).Run()
				}
			}
		}
		if err != nil {
			return fmt.Errorf("failed to export database: %w", err)
		}

		// 2. Compress Database SQL into ZIP
		ui.PrintStep(2, "Compressing database SQL dump into ZIP...")
		if runtime.GOOS == "linux" {
			_ = os.Remove(dbZipPath)
			zipCmd := exec.Command("zip", "-j", dbZipPath, dbSqlPath)
			if err := zipCmd.Run(); err != nil {
				return fmt.Errorf("failed to zip database: %w", err)
			}
		} else {
			// Mock compression on Windows
			_ = os.WriteFile(dbZipPath, []byte("Mock database ZIP content"), 0644)
		}
		_ = os.Remove(dbSqlPath) // Clean up raw SQL file
	} else {
		ui.PrintStep(1, "Skipping database export (static site)...")
		ui.PrintStep(2, "Skipping database ZIP creation (static site)...")
	}

	// 3. Compress Website Files into ZIP
	ui.PrintStep(3, "Compressing website public files into ZIP...")
	if runtime.GOOS == "linux" {
		_ = os.Remove(filesZipPath)
		zipCmd := exec.Command("zip", "-r", "-q", filesZipPath, "htdocs")
		zipCmd.Dir = parentDir // run in /var/www/[domain] to keep paths clean relative to htdocs
		if err := zipCmd.Run(); err != nil {
			return fmt.Errorf("failed to zip files: %w", err)
		}
	} else {
		// Mock compression on Windows
		_ = os.WriteFile(filesZipPath, []byte("Mock files ZIP content"), 0644)
	}

	// 4. Reset correct ownership on backup directory and ZIP files
	if runtime.GOOS == "linux" {
		_ = exec.Command("chown", "-R", fmt.Sprintf("%s:caddy", targetSite.SystemUser), backupDir).Run()
		_ = exec.Command("chmod", "-R", "0755", backupDir).Run()
	}

	ui.PrintSuccess("Manual Backup Completed")
	ui.PrintInfo("AgilePanel has successfully generated separate manual ZIP backups:")
	if hasDB {
		ui.Row("Database ZIP", dbZipPath)
	} else {
		ui.Row("Database ZIP", "(None - Static HTML site)")
	}
	ui.Row("Web Files ZIP", filesZipPath)
	ui.PrintInfo("These archives are owned by the site user and are ready for download via SFTP/FTP using system user credentials.")
	ui.Divider()
	fmt.Println()
	return nil
}

