package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"agilepanel/internal/site"
)

var (
	phpVersion   string
	installWP    bool
	siteType     string
	cleanWP      bool
	cleanRedis   bool
	cleanOpcache bool
	cleanCaddy   bool
)

var siteCmd = &cobra.Command{
	Use:   "site",
	Short: "Manage sites hosted on the server",
}

var siteCreateCmd = &cobra.Command{
	Use:   "create [domain]",
	Short: "Create a new site (WordPress, Laravel, Custom PHP, Static HTML)",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var domain string
		if len(args) >= 1 {
			domain = args[0]
		} else {
			var err error
			domain, err = promptString("Enter domain name: ")
			if err != nil {
				return err
			}
			if domain == "" {
				return fmt.Errorf("domain name cannot be empty")
			}
		}

		actualType := siteType
		if cmd.Flags().Changed("type") || cmd.Flags().Changed("wp") {
			if installWP {
				actualType = "wp"
			}
		} else {
			pt, err := promptString("Enter site type (wp, laravel, php, html) [wp]: ")
			if err != nil {
				return err
			}
			if pt != "" {
				actualType = pt
			} else {
				actualType = "wp"
			}
		}

		actualPHP := phpVersion
		if !cmd.Flags().Changed("php") {
			pp, err := promptString("Enter PHP version (e.g. 8.2, 8.3) [8.3]: ")
			if err != nil {
				return err
			}
			if pp != "" {
				actualPHP = pp
			} else {
				actualPHP = "8.3"
			}
		}

		return site.Create(domain, actualPHP, actualType)
	},
}

var siteDeleteCmd = &cobra.Command{
	Use:   "delete [domain]",
	Short: "Delete a site and all its assets",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		return site.Delete(domain)
	},
}

var siteLockCmd = &cobra.Command{
	Use:   "lock [domain]",
	Short: "Lock a site directory (changes permissions/attributes to immutable)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		return site.Lock(domain)
	},
}

var siteUnlockCmd = &cobra.Command{
	Use:   "unlock [domain]",
	Short: "Unlock a site directory (removes immutable attributes)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		return site.Unlock(domain)
	},
}

var siteCacheCleanCmd = &cobra.Command{
	Use:   "cache-clean [domain]",
	Short: "Clean various caching layers (WordPress transients, Redis query cache, PHP OPcache, Caddy edge)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		wp := cleanWP
		redis := cleanRedis
		opcache := cleanOpcache
		caddy := cleanCaddy
		if !wp && !redis && !opcache && !caddy {
			wp, redis, opcache, caddy = true, true, true, true
		}
		return site.CacheClean(domain, wp, redis, opcache, caddy)
	},
}

var siteReinstallCmd = &cobra.Command{
	Use:   "reinstall [domain]",
	Short: "Reinstall WordPress core files and database schemas for a site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		return site.Reinstall(domain)
	},
}

var siteSSLRenewCmd = &cobra.Command{
	Use:   "ssl-renew [domain]",
	Short: "Force Caddy to request a fresh Let's Encrypt / ZeroSSL certificate for a site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		return site.SSLRenew(domain)
	},
}

var siteFixPermissionsCmd = &cobra.Command{
	Use:   "fix-permissions [domain]",
	Short: "Restore correct owners and file/directory permissions for a site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		return site.FixPermissions(domain)
	},
}

var siteBackupDBCmd = &cobra.Command{
	Use:   "backup-db [domain]",
	Short: "Create a database SQL backup inside the site's secure backup folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		return site.BackupDB(domain)
	},
}

var siteBackupCmd = &cobra.Command{
	Use:   "backup [domain]",
	Short: "Create separate manual ZIP backups of WordPress files and MariaDB database",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		return site.Backup(domain)
	},
}

var siteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all websites hosted on the server",
	RunE: func(cmd *cobra.Command, args []string) error {
		return site.List()
	},
}

var siteInfoCmd = &cobra.Command{
	Use:   "info [domain]",
	Short: "Show details and credentials of a website",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		return site.Info(domain)
	},
}

var siteEditCmd = &cobra.Command{
	Use:   "edit [domain]",
	Short: "Edit web server configuration for a website",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		return site.Edit(domain)
	},
}

func init() {
	siteCreateCmd.Flags().StringVar(&phpVersion, "php", "", "PHP version to use (e.g. 8.3)")
	siteCreateCmd.Flags().BoolVar(&installWP, "wp", false, "Install WordPress automatically (alias for --type=wp)")
	siteCreateCmd.Flags().StringVar(&siteType, "type", "wp", "Type of site to create: wp, laravel, php, html")

	siteCacheCleanCmd.Flags().BoolVar(&cleanWP, "wp", false, "Clean WordPress transients and internal cache")
	siteCacheCleanCmd.Flags().BoolVar(&cleanRedis, "redis", false, "Clean Redis Object Cache")
	siteCacheCleanCmd.Flags().BoolVar(&cleanOpcache, "opcache", false, "Clean PHP OPcache (reloads PHP-FPM)")
	siteCacheCleanCmd.Flags().BoolVar(&cleanCaddy, "caddy", false, "Clean Caddy edge cache")

	siteCmd.AddCommand(siteCreateCmd)
	siteCmd.AddCommand(siteDeleteCmd)
	siteCmd.AddCommand(siteListCmd)
	siteCmd.AddCommand(siteInfoCmd)
	siteCmd.AddCommand(siteEditCmd)
	siteCmd.AddCommand(siteLockCmd)
	siteCmd.AddCommand(siteUnlockCmd)
	siteCmd.AddCommand(siteCacheCleanCmd)
	siteCmd.AddCommand(siteReinstallCmd)
	siteCmd.AddCommand(siteSSLRenewCmd)
	siteCmd.AddCommand(siteFixPermissionsCmd)
	siteCmd.AddCommand(siteBackupDBCmd)
	siteCmd.AddCommand(siteBackupCmd)

	rootCmd.AddCommand(siteCmd)
}
