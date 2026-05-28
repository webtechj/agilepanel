package site

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
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

// Create provisions a site's infrastructure.
func Create(domain string, phpVersion string, installWP bool) error {
	if err := ValidateDomain(domain); err != nil {
		return err
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

		// 5. Provision and set permissions on web folders
		publicDir := fmt.Sprintf("/var/www/%s/htdocs", domain)
		if err := server.ProvisionSiteDirectory(publicDir, systemUser); err != nil {
			_ = server.DeleteSystemUser(systemUser)
			return fmt.Errorf("failed to provision web folders: %w", err)
		}

		// 6. Create Database
		if err := server.CreateDatabase(dbName, dbUser, dbPassword); err != nil {
			_ = server.DeleteSiteDirectory(publicDir)
			_ = server.DeleteSystemUser(systemUser)
			return fmt.Errorf("database setup failed: %w", err)
		}

		// 7. Write PHP-FPM Pool config
		newSite := config.SiteConfig{
			Domain:       strings.ToLower(domain),
			PHPVersion:   phpVersion,
			PublicDir:    publicDir,
			DatabaseName: dbName,
			DatabaseUser: dbUser,
			SystemUser:   systemUser,
			IsLocked:     false,
		}

		if err := server.WritePHPPool(&newSite); err != nil {
			_ = server.DeleteDatabase(dbName, dbUser)
			_ = server.DeleteSiteDirectory(publicDir)
			_ = server.DeleteSystemUser(systemUser)
			return fmt.Errorf("failed to write PHP-FPM config: %w", err)
		}

		// 8. If WordPress installation is requested, execute WP-CLI routine
		if installWP {
			wpAdminPassword, err = server.InstallWordPress(
				systemUser,
				domain,
				publicDir,
				dbName,
				dbUser,
				dbPassword,
				s.Global.RedisSocketPath,
			)
			if err != nil {
				_ = server.DeletePHPPool(phpVersion, domain)
				_ = server.DeleteDatabase(dbName, dbUser)
				_ = server.DeleteSiteDirectory(publicDir)
				_ = server.DeleteSystemUser(systemUser)
				return fmt.Errorf("WordPress installation failed: %w", err)
			}
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

	// Print credentials for user
	fmt.Println("\n==================================================")
	fmt.Println("             SITE CREATED SUCCESSFULLY            ")
	fmt.Println("==================================================")
	fmt.Printf("Domain:          %s\n", domain)
	fmt.Printf("PHP Version:     %s\n", phpVersion)
	fmt.Printf("System User:     %s\n", SanitizeUser(domain))
	fmt.Printf("Database Name:   %s\n", dbName)
	fmt.Printf("Database User:   %s\n", dbUser)
	fmt.Printf("Database Pass:   %s\n", dbPassword)
	if installWP {
		fmt.Printf("WP Admin User:   admin\n")
		fmt.Printf("WP Admin Pass:   %s\n", wpAdminPassword)
		fmt.Printf("WP Admin Email:  admin@%s\n", domain)
	}
	fmt.Println("==================================================")

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

	return err
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

	return err
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

	return err
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

	homeDir := filepath.Dir(targetSite.PublicDir)

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

	fmt.Printf("Success: Cache cleanup completed for %s.\n", domain)
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

		// 4. Generate new DB password
		dbPassword, err := server.GenerateSecurePassword()
		if err != nil {
			return err
		}

		// 5. Update user and database credentials
		if err := server.CreateDatabase(targetSite.DatabaseName, targetSite.DatabaseUser, dbPassword); err != nil {
			return err
		}

		// 6. Install WordPress again
		wpAdminPassword, err := server.InstallWordPress(
			targetSite.SystemUser,
			targetSite.Domain,
			targetSite.PublicDir,
			targetSite.DatabaseName,
			targetSite.DatabaseUser,
			dbPassword,
			s.Global.RedisSocketPath,
		)
		if err != nil {
			return err
		}

		// 7. Re-lock if it was locked originally
		if targetSite.IsLocked {
			if err := server.LockDirectory(targetSite.PublicDir); err != nil {
				fmt.Printf("Warning: Failed to re-lock site directory: %v\n", err)
			}
		}

		fmt.Printf("Success: Reinstalled WordPress for %s successfully.\n", domain)
		fmt.Println("\n==================================================")
		fmt.Println("             REINSTALL COMPLETE                   ")
		fmt.Println("==================================================")
		fmt.Printf("Domain:          %s\n", domain)
		fmt.Printf("Database Pass:   %s\n", dbPassword)
		fmt.Printf("WP Admin User:   admin\n")
		fmt.Printf("WP Admin Pass:   %s\n", wpAdminPassword)
		fmt.Println("==================================================")

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

	fmt.Printf("Success: SSL renewal sequence triggered for %s.\n", domain)
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

	parentDir := filepath.Dir(targetSite.PublicDir) // /var/www/[domain]
	return server.FixPermissions(parentDir, targetSite.SystemUser)
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
	parentDir := filepath.Dir(targetSite.PublicDir) // /var/www/[domain]
	backupDir := filepath.Join(parentDir, "backup")
	backupFile := fmt.Sprintf("%s-%s.sql", domain, timestamp)
	backupPath := filepath.Join(backupDir, backupFile)

	homeDir := filepath.Dir(targetSite.PublicDir)
	fmt.Printf("WP-CLI: Exporting database dump for %s to %s...\n", domain, backupPath)
	err = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "db", "export", backupPath, "--path="+targetSite.PublicDir)
	if err != nil {
		return fmt.Errorf("failed to export database: %w", err)
	}

	fmt.Printf("Success: Database backed up successfully at %s.\n", backupPath)
	return nil
}
