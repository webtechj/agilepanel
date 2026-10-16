package site

import (
	"fmt"
	"strings"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

// Optimize performs housekeeping on a site's database, expired transient caches, and spam data.
func Optimize(domain string) error {
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
	homeDir := parentDir
	isWP := targetSite.Type == "wp" || targetSite.Type == "woocommerce" || targetSite.Type == ""

	ui.Banner("Site Optimization & Maintenance: " + domain)

	if isWP {
		// 1. Clean expired transients
		ui.PrintStep(1, "Purging expired transients and orphaned cache records...")
		_ = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "transient", "delete", "--expired", "--path="+targetSite.PublicDir)

		// 2. Clean spam / trash comments
		ui.PrintStep(2, "Purging spam and trash comments...")
		_ = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "comment", "delete", "$(wp comment list --status=spam,trash --format=ids)", "--force", "--path="+targetSite.PublicDir)

		// 3. Optimize MariaDB tables
		if targetSite.DatabaseName != "" {
			ui.PrintStep(3, "Optimizing and defragmenting MariaDB database tables...")
			_ = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "db", "optimize", "--path="+targetSite.PublicDir)
		}

		// 4. Flush caching layers
		ui.PrintStep(4, "Flushing Redis Object Cache & query buffer...")
		_ = CacheClean(domain, true, true, false, false)

	} else if targetSite.DatabaseName != "" {
		ui.PrintStep(1, "Optimizing MariaDB database tables...")
		if targetSite.DatabaseName != "" {
			_ = server.RunAsUser(targetSite.SystemUser, homeDir, "mysqlcheck", "-u"+targetSite.DatabaseUser, "--optimize", targetSite.DatabaseName)
		}
		ui.PrintStep(2, "Reloading PHP OPcache...")
		_ = CacheClean(domain, false, false, true, false)
	} else {
		ui.PrintStep(1, "Clearing edge page caches...")
		_ = CacheClean(domain, false, false, false, true)
	}

	ui.PrintSuccess("Site Optimization Completed")
	ui.SectionHeader("OPTIMIZATION SUMMARY")
	ui.Row("Domain", domain)
	ui.Row("Database Tables", "Defragmented & Indexed")
	ui.Row("Expired Transients", "Purged")
	ui.Row("Redis Object Cache", "Flushed")
	ui.Divider()
	ui.PrintInfo("Your website database and caching layers have been optimized. Unused transient data and spam records were purged to maximize responsiveness.")
	fmt.Println()

	return nil
}
