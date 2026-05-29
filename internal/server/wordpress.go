package server

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// RunAsUser executes a command on Linux using sudo as the target system user.
func RunAsUser(username string, homeDir string, command string, args ...string) error {
	if runtime.GOOS != "linux" {
		fullArgs := append([]string{command}, args...)
		if homeDir != "" {
			fmt.Printf("WP-CLI (Mock): Run as %s (HOME=%s): %v\n", username, homeDir, fullArgs)
		} else {
			fmt.Printf("WP-CLI (Mock): Run as %s: %v\n", username, fullArgs)
		}
		return nil
	}

	var sudoArgs []string
	if homeDir != "" {
		sudoArgs = append([]string{"-u", username, "env", "HOME=" + homeDir, command}, args...)
	} else {
		sudoArgs = append([]string{"-u", username, command}, args...)
	}
	cmd := exec.Command("sudo", sudoArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("WP-CLI error running %s: %w (stderr: %s)", command, err, stderr.String())
	}
	return nil
}

// InstallWordPress downloads, installs, and configures WordPress and its Redis Object Cache.
// adminUser and adminEmail are provided by the operator and must not be empty.
func InstallWordPress(username string, domain string, publicDir string, dbName string, dbUser string, dbPassword string, redisSocket string, adminUser string, adminEmail string, siteType string) (string, error) {
	adminPassword, err := GenerateSecurePassword()
	if err != nil {
		return "", err
	}
	if len(adminPassword) > 16 {
		adminPassword = adminPassword[:16]
	}
	homeDir := filepath.Dir(publicDir)

	// 1. Download WordPress Core
	fmt.Printf("WP-CLI: Downloading WordPress Core to %s...\n", publicDir)
	if err := RunAsUser(username, homeDir, "wp", "core", "download", "--path="+publicDir); err != nil {
		return "", fmt.Errorf("core download failed: %w", err)
	}

	// 2. Create wp-config.php configuration
	fmt.Println("WP-CLI: Creating wp-config.php database profile...")
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
	fmt.Printf("WP-CLI: Installing WordPress database schemas (admin: %s)...\n", adminUser)
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
	fmt.Println("WP-CLI: Installing and activating redis-cache plugin...")
	if err := RunAsUser(username, homeDir, "wp", "plugin", "install", "redis-cache", "--activate", "--path="+publicDir); err != nil {
		return "", fmt.Errorf("wp plugin install redis-cache failed: %w", err)
	}

	// 5. Configure Redis Cache parameters
	fmt.Println("WP-CLI: Coupling Redis Object Cache parameters to Unix socket...")
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

	// 6. Enable Redis Cache
	fmt.Println("WP-CLI: Enabling Redis Object Cache drop-in...")
	if err := RunAsUser(username, homeDir, "wp", "redis", "enable", "--path="+publicDir); err != nil {
		return "", fmt.Errorf("wp redis enable failed: %w", err)
	}

	// 7. General WordPress performance settings optimizations
	fmt.Println("WP-CLI: Applying performance tuning settings...")
	// Limit post revisions to 3 to keep DB lightweight
	_ = RunAsUser(username, homeDir, "wp", "config", "set", "WP_POST_REVISIONS", "3", "--raw", "--path="+publicDir)
	// Clean up permalinks structure to /%postname%/ (highly recommended for SEO/speed)
	_ = RunAsUser(username, homeDir, "wp", "permalink", "structure", "/%postname%/", "--path="+publicDir)
	// Clean default plugins
	_ = RunAsUser(username, homeDir, "wp", "plugin", "delete", "hello", "--path="+publicDir)

	// 8. WooCommerce Specific Caching Compatibility & Scaling Optimization
	if siteType == "woocommerce" {
		fmt.Println("WP-CLI: Installing and tuning WooCommerce plugin...")
		if err := RunAsUser(username, homeDir, "wp", "plugin", "install", "woocommerce", "--activate", "--path="+publicDir); err != nil {
			fmt.Printf("Warning: WooCommerce plugin installation failed: %v\n", err)
		}

		// Disable background WP-Cron execution to avoid request latency on checkout/payment
		_ = RunAsUser(username, homeDir, "wp", "config", "set", "DISABLE_WP_CRON", "true", "--raw", "--path="+publicDir)

		// Setup high-performance system cron to run WooCommerce background queries securely
		if runtime.GOOS == "linux" {
			cronPath := fmt.Sprintf("/etc/cron.d/agilepanel-cron-%s", strings.ReplaceAll(domain, ".", "-"))
			cronContent := fmt.Sprintf("*/10 * * * * %s cd %s && wp cron event run --due-now >/dev/null 2>&1\n", username, publicDir)
			if err := os.WriteFile(cronPath, []byte(cronContent), 0644); err == nil {
				fmt.Printf("WP-CLI: WooCommerce WP-Cron system task scheduled at %s\n", cronPath)
			}
		}
	}

	// 9. Secure wp-config.php by moving it to the parent directory (outside of public htdocs webroot)
	if runtime.GOOS == "linux" {
		parentDir := filepath.Dir(publicDir)
		configPath := filepath.Join(publicDir, "wp-config.php")
		targetConfigPath := filepath.Join(parentDir, "wp-config.php")

		if _, err := os.Stat(configPath); err == nil {
			if err := os.Rename(configPath, targetConfigPath); err != nil {
				return "", fmt.Errorf("failed to move wp-config.php to parent directory: %w", err)
			}
			_ = os.Chmod(targetConfigPath, 0600)
			fmt.Println("WP-CLI: Secured wp-config.php in parent directory.")
		}
	}

	return adminPassword, nil
}
