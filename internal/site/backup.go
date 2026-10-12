package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

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
			fmt.Printf("DB (Mock): mysqldump -u %s [MYSQL_PWD hidden] %s > %s\n", targetSite.DatabaseUser, targetSite.DatabaseName, backupPath)
		} else {
			cmd := exec.Command("mysqldump", "-u"+targetSite.DatabaseUser, targetSite.DatabaseName)
			cmd.Env = append(os.Environ(), "MYSQL_PWD="+targetSite.DatabasePass)

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

// Backup creates a ZIP of the database and a separate ZIP of the site files.
func Backup(domain string) (retErr error) {
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

	dest := targetSite.BackupDestination
	if dest == "" {
		dest = "local"
	}
	s3Enabled := targetSite.S3Enabled
	isS3 := dest == "s3" && s3Enabled
	timestamp := time.Now().Format("20060102-150405")

	defer func() {
		notifyBackup(domain, timestamp, isS3, state, retErr)
	}()

	parentDir := server.GetSiteRootDir(targetSite.PublicDir) // /var/www/[domain]
	backupDir := filepath.Join(parentDir, "backup")
	_ = os.MkdirAll(backupDir, 0755)

	hasDB := targetSite.DatabaseName != "" && targetSite.Type != "html"
	dbSqlPath := filepath.Join(backupDir, fmt.Sprintf("%s-db.sql", domain))
	dbZipPath := filepath.Join(backupDir, fmt.Sprintf("%s-db-%s.zip", domain, timestamp))
	filesZipPath := filepath.Join(backupDir, fmt.Sprintf("%s-files-%s.zip", domain, timestamp))

	homeDir := server.GetSiteRootDir(targetSite.PublicDir)

	// 1. Export Database
	if hasDB {
		ui.PrintStep(1, "Exporting MariaDB database dump...")
		var err error
		if targetSite.Type == "wp" || targetSite.Type == "" {
			err = server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "db", "export", dbSqlPath, "--path="+targetSite.PublicDir)
		} else {
			if runtime.GOOS != "linux" {
				fmt.Printf("DB (Mock): mysqldump -u %s [MYSQL_PWD hidden] %s > %s\n", targetSite.DatabaseUser, targetSite.DatabaseName, dbSqlPath)
			} else {
				cmd := exec.Command("mysqldump", "-u"+targetSite.DatabaseUser, targetSite.DatabaseName)
				cmd.Env = append(os.Environ(), "MYSQL_PWD="+targetSite.DatabasePass)

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

	// 5. Perform S3 upload if destination is "s3"
	dest = targetSite.BackupDestination
	if dest == "" || dest == "both" {
		dest = "local"
	}

	s3Enabled = targetSite.S3Enabled
	isS3 = dest == "s3" && s3Enabled
	isLocal := !isS3

	if isS3 && state.Global.S3Bucket != "" && state.Global.S3AccessKey != "" && state.Global.S3SecretKey != "" {
		ui.PrintStep(5, "Uploading backups to remote S3 Cloud Storage...")

		filesS3Key := fmt.Sprintf("backups/%s/%s-files-%s.zip", domain, domain, timestamp)
		fmt.Printf("S3: Uploading files ZIP version as: %s\n", filesS3Key)
		err := server.UploadToS3(filesZipPath, filesS3Key, state)
		if err != nil {
			return fmt.Errorf("S3 cloud files backup upload failed: %w", err)
		}

		if hasDB {
			dbS3Key := fmt.Sprintf("backups/%s/%s-db-%s.zip", domain, domain, timestamp)
			fmt.Printf("S3: Uploading database ZIP version as: %s\n", dbS3Key)
			err := server.UploadToS3(dbZipPath, dbS3Key, state)
			if err != nil {
				return fmt.Errorf("S3 cloud database backup upload failed: %w", err)
			}
		}
		ui.PrintSuccess("S3 Cloud Backups Uploaded Successfully")

		// Prune S3 backups according to retention settings
		maxVersions := targetSite.S3BackupVersions
		if maxVersions <= 0 {
			maxVersions = 5
		}
		if maxVersions < 1 {
			maxVersions = 1
		}
		if maxVersions > 7 {
			maxVersions = 7
		}

		s3Keys, err := server.ListS3Backups(domain, state)
		if err == nil {
			tsMap := make(map[string][]string)
			for _, k := range s3Keys {
				baseName := k
				if idx := strings.LastIndex(k, "/"); idx != -1 {
					baseName = k[idx+1:]
				}
				idxFiles := strings.Index(baseName, "-files-")
				idxDB := strings.Index(baseName, "-db-")
				var ts string
				if idxFiles != -1 {
					ts = baseName[idxFiles+len("-files-"):]
				} else if idxDB != -1 {
					ts = baseName[idxDB+len("-db-"):]
				}
				ts = strings.TrimSuffix(ts, ".zip")
				if len(ts) >= 13 && strings.Contains(ts, "-") {
					tsMap[ts] = append(tsMap[ts], k)
				}
			}

			var sortedTs []string
			for ts := range tsMap {
				sortedTs = append(sortedTs, ts)
			}
			sort.Sort(sort.Reverse(sort.StringSlice(sortedTs)))

			if len(sortedTs) > maxVersions {
				ui.PrintInfo(fmt.Sprintf("S3 backups count (%d) exceeds limit (%d). Pruning oldest backups...", len(sortedTs), maxVersions))
				for i := maxVersions; i < len(sortedTs); i++ {
					oldTs := sortedTs[i]
					for _, oldKey := range tsMap[oldTs] {
						fmt.Printf("S3: Deleting old backup file: %s\n", oldKey)
						_ = server.DeleteFromS3(oldKey, state)
					}
				}
			}
		}

		ui.PrintInfo("Backup destination is S3 Cloud Storage only. Removing local backup ZIPs...")
		_ = os.Remove(filesZipPath)
		if hasDB {
			_ = os.Remove(dbZipPath)
		}
	} else if isS3 {
		ui.PrintWarning("Backup destination is configured for S3 Cloud, but global S3 credentials are not set.")
	}

	// 6. Prune local backup versions to max 2
	if isLocal {
		pruneLocalBackups(backupDir, domain, 2)
	}

	ui.PrintSuccess("Backup Operations Completed")
	ui.PrintInfo("AgilePanel has successfully generated separate ZIP backups (and uploaded versions to S3 if configured):")
	if isLocal {
		if hasDB {
			ui.Row("Database ZIP", dbZipPath)
		} else {
			ui.Row("Database ZIP", "(None - Static HTML site)")
		}
		ui.Row("Web Files ZIP", filesZipPath)
		ui.PrintInfo("These archives are owned by the site user and are ready for download via SFTP/FTP using system user credentials.")
	} else {
		ui.PrintInfo("Backups successfully uploaded offsite to S3 Cloud. No local copies were kept as per storage destination settings.")
	}
	ui.Divider()
	fmt.Println()
	return nil
}

// pruneLocalBackups deletes the oldest timestamped local backup ZIPs, keeping maxKeep versions.
func pruneLocalBackups(backupDir, domain string, maxKeep int) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}

	type backupEntry struct {
		name string
		ts   string
	}
	var filesBackups []backupEntry
	var dbBackups []backupEntry

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		filesPrefix := fmt.Sprintf("%s-files-", domain)
		dbPrefix := fmt.Sprintf("%s-db-", domain)
		if strings.HasPrefix(name, filesPrefix) && strings.HasSuffix(name, ".zip") {
			ts := strings.TrimSuffix(strings.TrimPrefix(name, filesPrefix), ".zip")
			filesBackups = append(filesBackups, backupEntry{name: name, ts: ts})
		} else if strings.HasPrefix(name, dbPrefix) && strings.HasSuffix(name, ".zip") {
			ts := strings.TrimSuffix(strings.TrimPrefix(name, dbPrefix), ".zip")
			dbBackups = append(dbBackups, backupEntry{name: name, ts: ts})
		}
	}

	sortBackups := func(list []backupEntry) {
		for i := 0; i < len(list)-1; i++ {
			for j := i + 1; j < len(list); j++ {
				if list[j].ts > list[i].ts {
					list[i], list[j] = list[j], list[i]
				}
			}
		}
	}
	sortBackups(filesBackups)
	sortBackups(dbBackups)

	for i := maxKeep; i < len(filesBackups); i++ {
		_ = os.Remove(filepath.Join(backupDir, filesBackups[i].name))
	}
	for i := maxKeep; i < len(dbBackups); i++ {
		_ = os.Remove(filepath.Join(backupDir, dbBackups[i].name))
	}
}

// ListS3BackupsCLI prints S3 backups for a domain as a JSON list or text output
func ListS3BackupsCLI(domain string, formatJSON bool) error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return err
	}

	keys, err := server.ListS3Backups(domain, state)
	if err != nil {
		return err
	}

	timestampsMap := make(map[string]bool)
	for _, key := range keys {
		baseName := key
		if idx := strings.LastIndex(key, "/"); idx != -1 {
			baseName = key[idx+1:]
		}

		idxFiles := strings.Index(baseName, "-files-")
		idxDB := strings.Index(baseName, "-db-")
		var ts string
		if idxFiles != -1 {
			ts = baseName[idxFiles+len("-files-"):]
		} else if idxDB != -1 {
			ts = baseName[idxDB+len("-db-"):]
		}
		ts = strings.TrimSuffix(ts, ".zip")
		if len(ts) >= 13 && strings.Contains(ts, "-") {
			timestampsMap[ts] = true
		}
	}

	var timestamps []string
	for ts := range timestampsMap {
		timestamps = append(timestamps, ts)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(timestamps)))

	if formatJSON {
		bytes, err := json.Marshal(timestamps)
		if err != nil {
			return err
		}
		fmt.Println(string(bytes))
		return nil
	}

	ui.Banner("S3 Cloud Backups for " + domain)
	if len(timestamps) == 0 {
		ui.PrintWarning("No S3 backup history found.")
		return nil
	}

	ui.PrintInfo("Available S3 backup versions:")
	for _, ts := range timestamps {
		var year, month, day, hour, min, sec string
		if len(ts) == 15 {
			year = ts[0:4]
			month = ts[4:6]
			day = ts[6:8]
			hour = ts[9:11]
			min = ts[11:13]
			sec = ts[13:15]
			fmt.Printf("  • Version: %s  (%s-%s-%s %s:%s:%s)\n", ts, year, month, day, hour, min, sec)
		} else {
			fmt.Printf("  • Version: %s\n", ts)
		}
	}
	ui.Divider()
	fmt.Println()
	return nil
}

// DownloadS3BackupCLI pulls the backup zip files from S3 and saves them locally.
func DownloadS3BackupCLI(domain string, timestamp string) error {
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
	backupDir := filepath.Join(parentDir, "backup")
	_ = os.MkdirAll(backupDir, 0755)

	filesZipPath := filepath.Join(backupDir, fmt.Sprintf("%s-files.zip", domain))
	dbZipPath := filepath.Join(backupDir, fmt.Sprintf("%s-db.zip", domain))

	filesS3Key := fmt.Sprintf("backups/%s/%s-files-%s.zip", domain, domain, timestamp)
	dbS3Key := fmt.Sprintf("backups/%s/%s-db-%s.zip", domain, domain, timestamp)

	ui.PrintStep(1, "Downloading files backup ZIP from S3 Cloud...")
	err = server.DownloadFromS3(filesS3Key, filesZipPath, state)
	if err != nil {
		return fmt.Errorf("failed to download files ZIP from S3: %w", err)
	}

	hasDB := targetSite.DatabaseName != "" && targetSite.Type != "html"
	if hasDB {
		ui.PrintStep(2, "Downloading database backup ZIP from S3 Cloud...")
		err = server.DownloadFromS3(dbS3Key, dbZipPath, state)
		if err != nil {
			fmt.Printf("Warning: Database backup ZIP download failed: %v. Continuing...\n", err)
			_ = os.Remove(dbZipPath)
		}
	}

	if runtime.GOOS == "linux" {
		_ = exec.Command("chown", "-R", fmt.Sprintf("%s:caddy", targetSite.SystemUser), backupDir).Run()
		_ = exec.Command("chmod", "-R", "0755", backupDir).Run()
	}

	ui.PrintSuccess("S3 Backup Download Completed")
	ui.PrintInfo("The requested S3 version backups have been pulled and staged locally. Ready to run restore.")
	ui.Divider()
	fmt.Println()
	return nil
}

// DeleteS3BackupCLI deletes S3 backup files from the S3 cloud storage bucket.
func DeleteS3BackupCLI(domain string, timestamp string) error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return err
	}

	filesS3Key := fmt.Sprintf("backups/%s/%s-files-%s.zip", domain, domain, timestamp)
	dbS3Key := fmt.Sprintf("backups/%s/%s-db-%s.zip", domain, domain, timestamp)

	ui.PrintInfo(fmt.Sprintf("S3: Deleting remote S3 backups for timestamp: %s...", timestamp))

	errFiles := server.DeleteFromS3(filesS3Key, state)
	errDB := server.DeleteFromS3(dbS3Key, state)

	if errFiles != nil {
		ui.PrintWarning(fmt.Sprintf("S3: Failed to delete files backup from S3: %v", errFiles))
	} else {
		ui.PrintSuccess(fmt.Sprintf("S3: Successfully deleted files backup: %s", filesS3Key))
	}

	if errDB != nil {
		var targetSite config.SiteConfig
		for _, s := range state.Sites {
			if strings.EqualFold(s.Domain, domain) {
				targetSite = s
				break
			}
		}
		if targetSite.DatabaseName != "" && targetSite.Type != "html" {
			ui.PrintWarning(fmt.Sprintf("S3: Failed to delete database backup from S3: %v", errDB))
		}
	} else {
		ui.PrintSuccess(fmt.Sprintf("S3: Successfully deleted database backup: %s", dbS3Key))
	}

	ui.Divider()
	return nil
}
