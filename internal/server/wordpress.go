package server

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"agilepanel/internal/ui"
)

// RunAsUser executes a command on Linux using sudo as the target system user with intelligent output interception and error diagnostics.
func RunAsUser(username string, homeDir string, command string, args ...string) error {
	opts := CommandOptions{
		RunAsUser:     username,
		HomeDir:       homeDir,
		AllowAutoHeal: true,
		RetryCount:    1,
		Timeout:       120 * time.Second,
	}
	_, err := SmartExecute(command, args, opts)
	return err
}

// RunAsUserWithSite executes a command as system user with site context for automated self-healing (e.g. unlocking immutable locks).
func RunAsUserWithSite(username string, homeDir string, publicDir string, command string, args ...string) error {
	opts := CommandOptions{
		RunAsUser:     username,
		HomeDir:       homeDir,
		SitePublicDir: publicDir,
		AllowAutoHeal: true,
		RetryCount:    1,
		Timeout:       120 * time.Second,
	}
	_, err := SmartExecute(command, args, opts)
	return err
}

// InstallWordPress downloads, installs, and configures WordPress and its Redis Object Cache.
// adminUser and adminEmail are provided by the operator and must not be empty.
func InstallWordPress(username string, domain string, publicDir string, dbName string, dbUser string, dbPassword string, redisSocket string, adminUser string, adminEmail string, adminPassword string, siteType string) (string, error) {
	if adminPassword == "" {
		var err error
		adminPassword, err = GenerateSecurePassword()
		if err != nil {
			return "", err
		}
		if len(adminPassword) > 16 {
			adminPassword = adminPassword[:16]
		}
	}
	homeDir := filepath.Dir(publicDir)

	// 1. Download WordPress Core
	ui.PrintStep(1, fmt.Sprintf("Downloading WordPress Core files to %s...", publicDir))
	if err := RunAsUser(username, homeDir, "wp", "core", "download", "--path="+publicDir); err != nil {
		return "", fmt.Errorf("core download failed: %w", err)
	}

	// 2. Create wp-config.php configuration
	ui.PrintStep(2, "Generating secure database configuration profile (wp-config.php)...")
	tablePrefix, err := GenerateSecurePrefix(6)
	if err != nil {
		return "", fmt.Errorf("failed to generate secure table prefix: %w", err)
	}
	prefixArg := fmt.Sprintf("wp_%s_", tablePrefix)

	if err := RunAsUser(username, homeDir, "wp", "config", "create",
		"--path="+publicDir,
		"--dbname="+dbName,
		"--dbuser="+dbUser,
		"--dbpass="+dbPassword,
		"--dbhost=127.0.0.1",
		"--dbprefix="+prefixArg,
	); err != nil {
		return "", fmt.Errorf("wp config create failed: %w", err)
	}

	// 3. Install WordPress using the operator-supplied admin credentials
	ui.PrintStep(3, fmt.Sprintf("Initializing WordPress database schemas and creating admin user '%s'...", adminUser))
	if err := RunAsUser(username, homeDir, "wp", "core", "install",
		"--path="+publicDir,
		"--url=https://"+domain,
		"--title="+domain,
		"--admin_user="+adminUser,
		"--admin_password="+adminPassword,
		"--admin_email="+adminEmail,
	); err != nil {
		return "", fmt.Errorf("wp core install failed: %w", err)
	}

	// 4. Install Redis Object Cache Plugin
	ui.PrintStep(4, "Installing and activating Redis Object Cache plugin...")
	if err := RunAsUser(username, homeDir, "wp", "plugin", "install", "redis-cache", "--activate", "--path="+publicDir); err != nil {
		return "", fmt.Errorf("wp plugin install redis-cache failed: %w", err)
	}

	// 5. Configure Redis Cache parameters
	ui.PrintStep(5, "Coupling Redis Object Cache prefix and UNIX socket path definitions...")
	if err := RunAsUser(username, homeDir, "wp", "config", "set", "WP_REDIS_SCHEME", "unix", "--path="+publicDir); err != nil {
		return "", fmt.Errorf("wp config set WP_REDIS_SCHEME failed: %w", err)
	}

	if err := RunAsUser(username, homeDir, "wp", "config", "set", "WP_REDIS_PATH", redisSocket, "--path="+publicDir); err != nil {
		return "", fmt.Errorf("wp config set WP_REDIS_PATH failed: %w", err)
	}

	// Set dynamic Cache Prefix to isolate multi-site keys in Redis database
	cachePrefix := strings.ReplaceAll(domain, ".", "_") + "_"
	if err := RunAsUser(username, homeDir, "wp", "config", "set", "WP_CACHE_KEY_PREFIX", cachePrefix, "--path="+publicDir); err != nil {
		return "", fmt.Errorf("wp config set WP_CACHE_KEY_PREFIX failed: %w", err)
	}

	// Micro-timeouts to prevent PHP worker stalls during Redis locking/restarts
	_ = RunAsUser(username, homeDir, "wp", "config", "set", "WP_REDIS_TIMEOUT", "1", "--raw", "--path="+publicDir)
	_ = RunAsUser(username, homeDir, "wp", "config", "set", "WP_REDIS_READ_TIMEOUT", "1", "--raw", "--path="+publicDir)
	// Enable selective flush to isolate cache purges per site prefix
	_ = RunAsUser(username, homeDir, "wp", "config", "set", "WP_REDIS_SELECTIVE_FLUSH", "true", "--raw", "--path="+publicDir)

	// 6. Enable Redis Cache
	ui.PrintStep(6, "Activating Redis Object Cache engine (object-cache.php drop-in)...")
	if err := RunAsUser(username, homeDir, "wp", "redis", "enable", "--path="+publicDir); err != nil {
		return "", fmt.Errorf("wp redis enable failed: %w", err)
	}

	// 7. General WordPress performance settings optimizations
	ui.PrintStep(7, "Applying standard performance optimizations (revisions limits, clean permalinks, memory)...")
	// Limit post revisions to 3 to keep DB lightweight
	_ = RunAsUser(username, homeDir, "wp", "config", "set", "WP_POST_REVISIONS", "3", "--raw", "--path="+publicDir)
	// Clean up permalinks structure to /%postname%/ (highly recommended for SEO/speed)
	_ = RunAsUser(username, homeDir, "wp", "permalink", "structure", "/%postname%/", "--path="+publicDir)
	// Clean default plugins
	_ = RunAsUser(username, homeDir, "wp", "plugin", "delete", "hello", "--path="+publicDir)
	// Set baseline memory limits in wp-config.php
	wpMem := "128M"
	if siteType == "woocommerce" {
		wpMem = "256M"
	}
	_ = RunAsUser(username, homeDir, "wp", "config", "set", "WP_MEMORY_LIMIT", wpMem, "--path="+publicDir)
	_ = RunAsUser(username, homeDir, "wp", "config", "set", "WP_MAX_MEMORY_LIMIT", wpMem, "--path="+publicDir)

	// 8. WooCommerce Specific Caching Compatibility & Scaling Optimization
	if siteType == "woocommerce" {
		ui.PrintStep(8, "Installing WooCommerce core and setting up background cron tasks...")
		if err := RunAsUser(username, homeDir, "wp", "plugin", "install", "woocommerce", "--activate", "--path="+publicDir); err != nil {
			ui.PrintWarning(fmt.Sprintf("WooCommerce plugin installation failed: %v", err))
		}

		// Disable background WP-Cron execution to avoid request latency on checkout/payment
		_ = RunAsUser(username, homeDir, "wp", "config", "set", "DISABLE_WP_CRON", "true", "--raw", "--path="+publicDir)

		// Setup high-performance system cron to run WooCommerce background queries securely
		if runtime.GOOS == "linux" {
			cronPath := fmt.Sprintf("/etc/cron.d/agilepanel-cron-%s", strings.ReplaceAll(domain, ".", "-"))
			cronContent := fmt.Sprintf("PATH=/usr/local/bin:/usr/bin:/bin\n*/10 * * * * %s cd %s && /usr/local/bin/wp cron event run --due-now >/dev/null 2>&1\n", username, publicDir)
			if err := os.WriteFile(cronPath, []byte(cronContent), 0644); err == nil {
				ui.PrintInfo(fmt.Sprintf("WooCommerce WP-Cron system task scheduled at %s", cronPath))
			}
		}
	} else {
		ui.PrintStep(8, "Skipping WooCommerce setup (not requested).")
	}

	// 9. Secure wp-config.php by moving it to the parent directory (outside of public htdocs webroot)
	ui.PrintStep(9, "Hardening installation security (moving config file outside webroot)...")
	if runtime.GOOS == "linux" {
		parentDir := filepath.Dir(publicDir)
		configPath := filepath.Join(publicDir, "wp-config.php")
		targetConfigPath := filepath.Join(parentDir, "wp-config.php")

		if _, err := os.Stat(configPath); err == nil {
			if err := os.Rename(configPath, targetConfigPath); err != nil {
				return "", fmt.Errorf("failed to move wp-config.php to parent directory: %w", err)
			}
			_ = os.Chmod(targetConfigPath, 0600)
			ui.PrintInfo("Secured wp-config.php inside parent directory with owner-only read permissions.")
		}
	} else {
		ui.PrintInfo("Staging: Mock configuration file relocation.")
	}

	return adminPassword, nil
}

// TuneWordPressMemory configures WP_MEMORY_LIMIT and WP_MAX_MEMORY_LIMIT in wp-config.php.
func TuneWordPressMemory(username string, publicDir string, memoryLimit string) error {
	if memoryLimit == "" {
		memoryLimit = "128M"
	}
	homeDir := GetSiteRootDir(publicDir)
	if err := RunAsUser(username, homeDir, "wp", "config", "set", "WP_MEMORY_LIMIT", memoryLimit, "--path="+publicDir); err != nil {
		return err
	}
	return RunAsUser(username, homeDir, "wp", "config", "set", "WP_MAX_MEMORY_LIMIT", memoryLimit, "--path="+publicDir)
}

// FindWPConfigPath searches for wp-config.php in parent directory or public webroot.
func FindWPConfigPath(publicDir string) string {
	if publicDir == "" {
		return ""
	}
	// 1. Direct webroot check
	webrootPath := filepath.Join(publicDir, "wp-config.php")
	if _, err := os.Stat(webrootPath); err == nil {
		return webrootPath
	}
	// 2. Immediate parent directory (e.g. /var/www/domain/ when publicDir is /var/www/domain/htdocs)
	parentDir := filepath.Dir(publicDir)
	parentPath := filepath.Join(parentDir, "wp-config.php")
	if _, err := os.Stat(parentPath); err == nil {
		return parentPath
	}
	// 3. Site root directory
	siteRootDir := GetSiteRootDir(publicDir)
	siteRootPath := filepath.Join(siteRootDir, "wp-config.php")
	if _, err := os.Stat(siteRootPath); err == nil {
		return siteRootPath
	}
	return ""
}

// ParseMemoryMB converts memory limit strings (e.g. "128M", "256M", "1G", "512") into integer megabytes.
func ParseMemoryMB(mem string) int {
	mem = strings.ToUpper(strings.TrimSpace(mem))
	if mem == "" {
		return 0
	}
	multiplier := 1
	if strings.HasSuffix(mem, "G") {
		multiplier = 1024
		mem = strings.TrimSuffix(mem, "G")
	} else if strings.HasSuffix(mem, "M") {
		multiplier = 1
		mem = strings.TrimSuffix(mem, "M")
	} else if strings.HasSuffix(mem, "K") {
		multiplier = 0
		mem = strings.TrimSuffix(mem, "K")
	}
	val, err := strconv.Atoi(mem)
	if err != nil {
		return 0
	}
	return val * multiplier
}

// CompareMemory compares two memory string definitions (e.g. "256M", "128M").
// Returns 1 if memA > memB, -1 if memA < memB, 0 if equal.
func CompareMemory(memA string, memB string) int {
	mbA := ParseMemoryMB(memA)
	mbB := ParseMemoryMB(memB)
	if mbA > mbB {
		return 1
	} else if mbA < mbB {
		return -1
	}
	return 0
}

// ReadWordPressMemoryLimit extracts WP_MEMORY_LIMIT and WP_MAX_MEMORY_LIMIT from wp-config.php.
func ReadWordPressMemoryLimit(publicDir string) (string, string, error) {
	configPath := FindWPConfigPath(publicDir)
	if configPath == "" {
		return "", "", fmt.Errorf("wp-config.php not found")
	}

	contentBytes, err := os.ReadFile(configPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read wp-config.php at %s: %w", configPath, err)
	}
	content := string(contentBytes)

	var wpMem, wpMaxMem string
	reWP := regexp.MustCompile(`(?i)define\s*\(\s*['"]WP_MEMORY_LIMIT['"]\s*,\s*['"]([^'"]+)['"]\s*\)`)
	if match := reWP.FindStringSubmatch(content); len(match) > 1 {
		wpMem = strings.TrimSpace(match[1])
	}

	reWPMax := regexp.MustCompile(`(?i)define\s*\(\s*['"]WP_MAX_MEMORY_LIMIT['"]\s*,\s*['"]([^'"]+)['"]\s*\)`)
	if match := reWPMax.FindStringSubmatch(content); len(match) > 1 {
		wpMaxMem = strings.TrimSpace(match[1])
	}

	return wpMem, wpMaxMem, nil
}

