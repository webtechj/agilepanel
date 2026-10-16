package site

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

// SafeMode enables or disables emergency safe mode for a WordPress site by deactivating plugins.
func SafeMode(domain string, action string) error {
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

	isWP := targetSite.Type == "wp" || targetSite.Type == "woocommerce" || targetSite.Type == ""
	if !isWP {
		return fmt.Errorf("safe mode is only supported for WordPress sites (site type: %s)", targetSite.Type)
	}

	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		action = "on"
	}

	parentDir := server.GetSiteRootDir(targetSite.PublicDir)
	confDir := filepath.Join(parentDir, "conf")
	_ = os.MkdirAll(confDir, 0755)
	markerFile := filepath.Join(confDir, "safemode_plugins.txt")

	homeDir := parentDir

	if action == "on" {
		if _, err := os.Stat(markerFile); err == nil {
			ui.PrintWarning("Safe mode is already enabled for " + domain + ".")
			ui.PrintInfo("Run 'ap site safe-mode " + domain + " off' to restore previous active plugins.")
			return nil
		}

		ui.Banner("Activating WordPress Safe Mode: " + domain)
		ui.PrintStep(1, "Snapshotting currently active plugins list...")

		var activePlugins []string
		if os.Getenv("AGILEPANEL_TEST_MODE") == "true" || runtime.GOOS != "linux" {
			activePlugins = []string{"redis-cache"}
		} else {
			cmd := exec.Command("sudo", "-u", targetSite.SystemUser, "wp", "plugin", "list", "--status=active", "--field=name", "--path="+targetSite.PublicDir)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err == nil {
				for _, line := range strings.Split(stdout.String(), "\n") {
					trimmed := strings.TrimSpace(line)
					if trimmed != "" {
						activePlugins = append(activePlugins, trimmed)
					}
				}
			}
		}

		// Save active plugins to state file
		content := strings.Join(activePlugins, "\n")
		if err := os.WriteFile(markerFile, []byte(content), 0600); err != nil {
			return fmt.Errorf("failed to save safe mode plugin state: %w", err)
		}

		ui.PrintStep(2, "Deactivating all WordPress plugins to clear White Screen of Death (WSOD)...")
		_ = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "plugin", "deactivate", "--all", "--path="+targetSite.PublicDir)

		ui.PrintStep(3, "Flushing Redis and WordPress caches...")
		_ = CacheClean(domain, true, true, true, false)

		ui.PrintSuccess("Safe Mode Activated Successfully")
		ui.SectionHeader("SAFE MODE ACTIVE")
		ui.Row("Domain", domain)
		ui.Row("Deactivated Plugins", fmt.Sprintf("%d plugin(s) safely turned off", len(activePlugins)))
		ui.Row("WordPress Admin", "https://"+domain+"/wp-admin")
		ui.Divider()
		ui.PrintInfo("All plugins have been temporarily deactivated. If a plugin caused a 500 error or crash, your website and wp-admin should now load normally.")
		ui.PrintInfo("When you are ready to re-activate your plugins, run: " + ui.Accent("ap site safe-mode "+domain+" off"))
		fmt.Println()
		return nil

	} else if action == "off" {
		if _, err := os.Stat(markerFile); os.IsNotExist(err) {
			return fmt.Errorf("safe mode is not currently active for %s (no deactivated plugin state found)", domain)
		}

		ui.Banner("Deactivating WordPress Safe Mode: " + domain)
		ui.PrintStep(1, "Reading saved plugin snapshot...")

		data, err := os.ReadFile(markerFile)
		if err != nil {
			return fmt.Errorf("failed to read safe mode plugin state: %w", err)
		}

		lines := strings.Split(string(data), "\n")
		reactivatedCount := 0

		ui.PrintStep(2, "Restoring previous active plugins...")
		for _, line := range lines {
			plugin := strings.TrimSpace(line)
			if plugin != "" {
				_ = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "plugin", "activate", plugin, "--path="+targetSite.PublicDir)
				reactivatedCount++
			}
		}

		// Clean up marker file
		_ = os.Remove(markerFile)

		ui.PrintStep(3, "Flushing WordPress and Redis caches...")
		_ = CacheClean(domain, true, true, true, false)

		ui.PrintSuccess("Safe Mode Deactivated Successfully")
		ui.SectionHeader("SAFE MODE OFF")
		ui.Row("Domain", domain)
		ui.Row("Re-activated Plugins", fmt.Sprintf("%d plugin(s) restored", reactivatedCount))
		ui.Divider()
		ui.PrintInfo("All previous plugins have been re-enabled and caching layers have been cleared.")
		fmt.Println()
		return nil

	} else {
		return fmt.Errorf("invalid action '%s': use 'on' or 'off'", action)
	}
}

// ResetPassword resets the WordPress administrator password via WP-CLI.
func ResetPassword(domain string, username string, newPassword string) error {
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

	isWP := targetSite.Type == "wp" || targetSite.Type == "woocommerce" || targetSite.Type == ""
	if !isWP {
		return fmt.Errorf("password reset is only supported for WordPress sites (site type: %s)", targetSite.Type)
	}

	if username == "" {
		if targetSite.WPAdminUser != "" {
			username = targetSite.WPAdminUser
		} else {
			username = "admin"
		}
	}

	if newPassword == "" {
		var genErr error
		newPassword, genErr = server.GenerateSecurePassword()
		if genErr != nil {
			return genErr
		}
		if len(newPassword) > 16 {
			newPassword = newPassword[:16]
		}
	}

	homeDir := server.GetSiteRootDir(targetSite.PublicDir)

	ui.Banner("Reset WordPress Admin Password")
	ui.PrintStep(1, fmt.Sprintf("Updating password for user '%s' in WordPress database...", username))

	err = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "user", "update", username, "--user_pass="+newPassword, "--path="+targetSite.PublicDir)
	if err != nil {
		return fmt.Errorf("failed to update WordPress user password: %w", err)
	}

	// Update WPAdminUser in state if changed
	_ = config.WithLockedState(statePath, func(s *config.State) error {
		for i := range s.Sites {
			if strings.EqualFold(s.Sites[i].Domain, domain) {
				s.Sites[i].WPAdminUser = username
				break
			}
		}
		return nil
	})

	ui.PrintSuccess("Admin Password Reset Completed")
	ui.SectionHeader("WORDPRESS CREDENTIALS")
	ui.Row("Domain", domain)
	ui.Row("Username", username)
	ui.Row("New Password", newPassword)
	ui.Row("Login URL", "https://"+domain+"/wp-admin")
	ui.Divider()
	ui.PrintInfo("Your WordPress admin credentials are updated. You can now log into your WordPress Dashboard.")
	fmt.Println()

	return nil
}
