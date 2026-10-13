package site

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

// Restore restores website files and/or MariaDB database from a local backup archive.
func Restore(domain string, backupTarget string) error {
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

	parentDir := server.GetSiteRootDir(targetSite.PublicDir) // /var/www/[domain]
	backupDir := filepath.Join(parentDir, "backup")

	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		return fmt.Errorf("backup directory does not exist: %s", backupDir)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	var filesZips []string
	var dbZips []string
	var dbSqls []string

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		fullPath := filepath.Join(backupDir, name)

		if strings.HasPrefix(name, domain+"-files-") && strings.HasSuffix(name, ".zip") {
			filesZips = append(filesZips, fullPath)
		} else if strings.HasPrefix(name, domain+"-db-") && strings.HasSuffix(name, ".zip") {
			dbZips = append(dbZips, fullPath)
		} else if strings.HasPrefix(name, domain) && strings.HasSuffix(name, ".sql") {
			dbSqls = append(dbSqls, fullPath)
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(filesZips)))
	sort.Sort(sort.Reverse(sort.StringSlice(dbZips)))
	sort.Sort(sort.Reverse(sort.StringSlice(dbSqls)))

	var targetFilesZip string
	var targetDBBackup string

	backupTarget = strings.TrimSpace(backupTarget)

	if backupTarget == "" || strings.EqualFold(backupTarget, "latest") {
		if len(filesZips) > 0 {
			targetFilesZip = filesZips[0]
		}
		if len(dbZips) > 0 {
			targetDBBackup = dbZips[0]
		} else if len(dbSqls) > 0 {
			targetDBBackup = dbSqls[0]
		}
	} else {
		// Specific filename or timestamp provided
		for _, fz := range filesZips {
			if strings.Contains(fz, backupTarget) {
				targetFilesZip = fz
				break
			}
		}
		for _, dz := range dbZips {
			if strings.Contains(dz, backupTarget) {
				targetDBBackup = dz
				break
			}
		}
		if targetDBBackup == "" {
			for _, ds := range dbSqls {
				if strings.Contains(ds, backupTarget) {
					targetDBBackup = ds
					break
				}
			}
		}
	}

	if targetFilesZip == "" && targetDBBackup == "" {
		return fmt.Errorf("no matching backup archives found for '%s' in %s", domain, backupDir)
	}

	ui.Banner("Restoring Site Backup: " + domain)

	// 1. Unlock directory if currently locked
	if targetSite.IsLocked {
		ui.PrintStep(1, "Temporarily unlocking site directory for restore...")
		_ = server.UnlockDirectory(targetSite.PublicDir)
	}

	// 2. Restore Files
	if targetFilesZip != "" {
		ui.PrintStep(2, fmt.Sprintf("Extracting website files from %s...", filepath.Base(targetFilesZip)))
		if runtime.GOOS == "linux" {
			unzipCmd := exec.Command("unzip", "-o", "-q", targetFilesZip, "-d", parentDir)
			if err := unzipCmd.Run(); err != nil {
				return fmt.Errorf("failed to unzip website files: %w", err)
			}
		} else {
			// Mock extraction in non-linux
			fmt.Printf("Mock unzip: %s into %s\n", targetFilesZip, parentDir)
		}

		// Reapply correct ownership and permissions
		_ = server.FixPermissions(parentDir, targetSite.SystemUser)
	} else {
		ui.PrintStep(2, "Skipping files restore (no files archive selected)...")
	}

	// 3. Restore Database
	if targetDBBackup != "" && targetSite.DatabaseName != "" {
		ui.PrintStep(3, fmt.Sprintf("Restoring database from %s...", filepath.Base(targetDBBackup)))
		sqlPathToImport := targetDBBackup
		needCleanup := false

		if strings.HasSuffix(targetDBBackup, ".zip") {
			// Unzip the SQL file temporarily
			if runtime.GOOS == "linux" {
				unzipCmd := exec.Command("unzip", "-o", "-q", targetDBBackup, "-d", backupDir)
				if err := unzipCmd.Run(); err != nil {
					return fmt.Errorf("failed to extract database dump from ZIP: %w", err)
				}
				// Look for extracted SQL file
				candidate := filepath.Join(backupDir, fmt.Sprintf("%s-db.sql", domain))
				if _, err := os.Stat(candidate); err == nil {
					sqlPathToImport = candidate
					needCleanup = true
				}
			} else {
				// Mock extraction
				mockSql := filepath.Join(backupDir, "mock_restore.sql")
				_ = os.WriteFile(mockSql, []byte("-- mock sql"), 0644)
				sqlPathToImport = mockSql
				needCleanup = true
			}
		}

		// Import database
		homeDir := parentDir
		if targetSite.Type == "wp" || targetSite.Type == "" {
			err = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "db", "import", sqlPathToImport, "--path="+targetSite.PublicDir)
			if err != nil {
				if needCleanup {
					_ = os.Remove(sqlPathToImport)
				}
				return fmt.Errorf("WP-CLI database import failed: %w", err)
			}
		} else {
			if runtime.GOOS == "linux" {
				cmd := exec.Command("mysql", "-u"+targetSite.DatabaseUser, targetSite.DatabaseName)
				cmd.Env = append(os.Environ(), "MYSQL_PWD="+targetSite.DatabasePass)
				sqlFile, oErr := os.Open(sqlPathToImport)
				if oErr != nil {
					if needCleanup {
						_ = os.Remove(sqlPathToImport)
					}
					return fmt.Errorf("failed to open SQL file: %w", oErr)
				}
				cmd.Stdin = sqlFile
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				err = cmd.Run()
				sqlFile.Close()
				if err != nil {
					if needCleanup {
						_ = os.Remove(sqlPathToImport)
					}
					return fmt.Errorf("mysql import failed: %w (stderr: %s)", err, stderr.String())
				}
			}
		}

		if needCleanup {
			_ = os.Remove(sqlPathToImport)
		}
	} else {
		ui.PrintStep(3, "Skipping database restore (none selected or static site)...")
	}

	// 4. Re-lock if originally locked
	if targetSite.IsLocked {
		ui.PrintStep(4, "Re-locking site directory to restore immutable protection...")
		_ = server.LockDirectory(targetSite.PublicDir)
	}

	// 5. Clean cache layers
	ui.PrintStep(5, "Purging caching layers (WordPress, Redis, OPcache, Caddy)...")
	_ = CacheClean(domain, true, true, true, true)

	ui.PrintSuccess("Site Restore Completed Successfully")
	ui.SectionHeader("RESTORE SUMMARY")
	ui.Row("Domain", domain)
	if targetFilesZip != "" {
		ui.Row("Files Archive", filepath.Base(targetFilesZip))
	}
	if targetDBBackup != "" {
		ui.Row("Database Archive", filepath.Base(targetDBBackup))
	}
	ui.Divider()
	ui.PrintInfo("Your website files and database schemas have been safely rolled back and all caching layers were cleared.")
	fmt.Println()

	return nil
}
