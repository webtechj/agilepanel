package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"agilepanel/internal/site"
)

var (
	phpVersion   string
	installWP    bool
	siteType     string
	createDBFlag string
	yesFlag      bool
	cleanWP      bool
	cleanRedis   bool
	cleanOpcache bool
	cleanCaddy   bool
	importFiles  string
	importDB     string
	wpAdminUser  string
	wpAdminPass  string
	wpAdminEmail string
	wpAdminName  string
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

		dbOpt := createDBFlag
		if actualType == "html" {
			if cmd.Flags().Changed("db") {
				// use explicitly passed value
			} else {
				resp, err := promptString("Do you need a MariaDB database for this HTML site? (y/N): ")
				if err == nil {
					resp = strings.ToLower(strings.TrimSpace(resp))
					if resp == "y" || resp == "yes" {
						dbOpt = "true"
					} else {
						dbOpt = "false"
					}
				} else {
					dbOpt = "false"
				}
			}
		}

		return site.Create(domain, actualPHP, actualType, dbOpt, importFiles, importDB, wpAdminUser, wpAdminPass, wpAdminEmail, wpAdminName)
	},
}

var siteDeleteCmd = &cobra.Command{
	Use:   "delete [domain]",
	Short: "Delete a site and all its assets",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, err := getDomainArg(args)
		if err != nil {
			return err
		}
		if !yesFlag {
			confirmed, err := promptDoubleConfirm(domain, "permanently delete")
			if err != nil {
				return err
			}
			if !confirmed {
				return fmt.Errorf("confirmation failed: domain name did not match or operation cancelled")
			}
		}
		return site.Delete(domain)
	},
}

var siteLockCmd = &cobra.Command{
	Use:   "lock [domain]",
	Short: "Lock a site directory (changes permissions/attributes to immutable)",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, err := getDomainArg(args)
		if err != nil {
			return err
		}
		if !yesFlag {
			confirmed, err := promptDoubleConfirm(domain, "lock/deactivate")
			if err != nil {
				return err
			}
			if !confirmed {
				return fmt.Errorf("confirmation failed: domain name did not match or operation cancelled")
			}
		}
		return site.Lock(domain)
	},
}

var siteUnlockCmd = &cobra.Command{
	Use:   "unlock [domain]",
	Short: "Unlock a site directory (removes immutable attributes)",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, err := getDomainArg(args)
		if err != nil {
			return err
		}
		return site.Unlock(domain)
	},
}

var siteCacheCleanCmd = &cobra.Command{
	Use:   "cache-clean [domain]",
	Short: "Clean various caching layers (WordPress transients, Redis query cache, PHP OPcache, Caddy edge)",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, err := getDomainArg(args)
		if err != nil {
			return err
		}
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
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, err := getDomainArg(args)
		if err != nil {
			return err
		}
		return site.Reinstall(domain)
	},
}

var siteSSLRenewCmd = &cobra.Command{
	Use:   "ssl-renew [domain]",
	Short: "Force Caddy to request a fresh Let's Encrypt / ZeroSSL certificate for a site",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, err := getDomainArg(args)
		if err != nil {
			return err
		}
		return site.SSLRenew(domain)
	},
}

var siteFixPermissionsCmd = &cobra.Command{
	Use:   "fix-permissions [domain]",
	Short: "Restore correct owners and file/directory permissions for a site",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, err := getDomainArg(args)
		if err != nil {
			return err
		}
		return site.FixPermissions(domain)
	},
}

var siteBackupDBCmd = &cobra.Command{
	Use:   "backup-db [domain]",
	Short: "Create a database SQL backup inside the site's secure backup folder",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, err := getDomainArg(args)
		if err != nil {
			return err
		}
		return site.BackupDB(domain)
	},
}

var siteBackupCmd = &cobra.Command{
	Use:   "backup [domain]",
	Short: "Create separate manual ZIP backups of WordPress files and MariaDB database",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, err := getDomainArg(args)
		if err != nil {
			return err
		}
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
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, err := getDomainArg(args)
		if err != nil {
			return err
		}
		return site.Info(domain)
	},
}

var siteEditCmd = &cobra.Command{
	Use:   "edit [domain]",
	Short: "Edit web server configuration for a website",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, err := getDomainArg(args)
		if err != nil {
			return err
		}
		return site.Edit(domain)
	},
}

var s3JsonFlag bool

var siteS3ListCmd = &cobra.Command{
	Use:   "s3-list [domain]",
	Short: "List S3 backup timestamps for a website",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, err := getDomainArg(args)
		if err != nil {
			return err
		}
		return site.ListS3BackupsCLI(domain, s3JsonFlag)
	},
}

var siteS3DownloadCmd = &cobra.Command{
	Use:   "s3-download [domain] [timestamp]",
	Short: "Download S3 backups for a website by timestamp",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		timestamp := args[1]
		return site.DownloadS3BackupCLI(domain, timestamp)
	},
}

var siteS3DeleteCmd = &cobra.Command{
	Use:   "s3-delete [domain] [timestamp]",
	Short: "Delete S3 backup version for a website by timestamp",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		timestamp := args[1]
		return site.DeleteS3BackupCLI(domain, timestamp)
	},
}

func init() {
	siteCreateCmd.Flags().StringVar(&phpVersion, "php", "", "PHP version to use (e.g. 8.3)")
	siteCreateCmd.Flags().BoolVar(&installWP, "wp", false, "Install WordPress automatically (alias for --type=wp)")
	siteCreateCmd.Flags().StringVar(&siteType, "type", "wp", "Type of site to create: wp, woocommerce, laravel, php, html")
	siteCreateCmd.Flags().StringVar(&createDBFlag, "db", "default", "Create MariaDB database for site: true, false, default")
	siteCreateCmd.Flags().StringVar(&importFiles, "import-files", "", "Path to imported website files ZIP")
	siteCreateCmd.Flags().StringVar(&importDB, "import-db", "", "Path to imported database SQL/ZIP file")
	siteCreateCmd.Flags().StringVar(&wpAdminUser, "wp-user", "", "WordPress admin username")
	siteCreateCmd.Flags().StringVar(&wpAdminPass, "wp-pass", "", "WordPress admin password")
	siteCreateCmd.Flags().StringVar(&wpAdminEmail, "wp-email", "", "WordPress admin email")
	siteCreateCmd.Flags().StringVar(&wpAdminName, "wp-name", "", "WordPress admin display name")
 
	siteDeleteCmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Bypass confirmation prompts")
	siteLockCmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Bypass confirmation prompts")
 
	siteCacheCleanCmd.Flags().BoolVar(&cleanWP, "wp", false, "Clean WordPress transients and internal cache")
	siteCacheCleanCmd.Flags().BoolVar(&cleanRedis, "redis", false, "Clean Redis Object Cache")
	siteCacheCleanCmd.Flags().BoolVar(&cleanOpcache, "opcache", false, "Clean PHP OPcache (reloads PHP-FPM)")
	siteCacheCleanCmd.Flags().BoolVar(&cleanCaddy, "caddy", false, "Clean Caddy edge cache")
 
	siteS3ListCmd.Flags().BoolVar(&s3JsonFlag, "json", false, "Output results in raw JSON format")

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
	siteCmd.AddCommand(siteS3ListCmd)
	siteCmd.AddCommand(siteS3DownloadCmd)
	siteCmd.AddCommand(siteS3DeleteCmd)
 
	rootCmd.AddCommand(siteCmd)
}
