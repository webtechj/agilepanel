package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CreateSystemUser creates a Linux system user and user group specific to the domain.
func CreateSystemUser(username string) error {
	if runtime.GOOS != "linux" {
		fmt.Printf("OS (Mock): Create system user & group: %s\n", username)
		return nil
	}

	// 1. Create group
	cmdGroup := exec.Command("groupadd", "--system", username)
	var stderr bytes.Buffer
	cmdGroup.Stderr = &stderr
	if err := cmdGroup.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 9 { // 9 means group exists
			return fmt.Errorf("failed to create system group: %w (stderr: %s)", err, stderr.String())
		}
	}

	// 2. Create user (no home directory, disabled login shell)
	cmdUser := exec.Command("useradd", "--system", "--gid", username, "--no-create-home", "--shell", "/usr/sbin/nologin", username)
	stderr.Reset()
	cmdUser.Stderr = &stderr
	if err := cmdUser.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 9 { // 9 means user exists
			return fmt.Errorf("failed to create system user: %w (stderr: %s)", err, stderr.String())
		}
	}

	// 3. Add system user to redis group so PHP-FPM pool can connect to Redis UNIX socket
	_ = exec.Command("usermod", "-a", "-G", "redis", username).Run()

	fmt.Printf("OS: Created system user and group %s successfully.\n", username)
	return nil
}

// DeleteSystemUser deletes the isolated user and group.
func DeleteSystemUser(username string) error {
	if runtime.GOOS != "linux" {
		fmt.Printf("OS (Mock): Delete system user & group: %s\n", username)
		return nil
	}

	// Delete user
	cmdUser := exec.Command("userdel", username)
	var stderr bytes.Buffer
	cmdUser.Stderr = &stderr
	if err := cmdUser.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 6 { // 6 means user not found
			fmt.Printf("OS Warning: failed to delete user %s: %v (stderr: %s)\n", username, err, stderr.String())
		}
	}

	// Delete group
	cmdGroup := exec.Command("groupdel", username)
	stderr.Reset()
	cmdGroup.Stderr = &stderr
	if err := cmdGroup.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 6 { // 6 means group not found
			fmt.Printf("OS Warning: failed to delete group %s: %v (stderr: %s)\n", username, err, stderr.String())
		}
	}

	fmt.Printf("OS: Deleted system user and group %s successfully.\n", username)
	return nil
}
// GetSiteRootDir dynamically locates the root /var/www/[domain] folder from the public path.
func GetSiteRootDir(publicDir string) string {
	parent := filepath.Clean(publicDir)
	for {
		dir := filepath.Dir(parent)
		if filepath.Base(dir) == "www" || dir == parent {
			return parent
		}
		parent = dir
	}
}

// ProvisionSiteDirectory creates and permissions a site's webroot directory.
func ProvisionSiteDirectory(path string, systemUser string) error {
	parentDir := GetSiteRootDir(path) // /var/www/[domain]
	htdocsPath := filepath.Join(parentDir, "htdocs")
	confPath := filepath.Join(parentDir, "conf")
	backupPath := filepath.Join(parentDir, "backup")

	tmpPath := filepath.Join(parentDir, "tmp")

	// Create directories (including the target path itself, which might be a public subfolder)
	for _, dir := range []string{htdocsPath, confPath, backupPath, tmpPath, path} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	if runtime.GOOS != "linux" {
		fmt.Printf("OS (Mock): chown -R %s:caddy %s\n", systemUser, parentDir)
		return nil
	}

	// Assign permissions to site-specific user and caddy/www-data group
	chownCmd := exec.Command("chown", "-R", fmt.Sprintf("%s:caddy", systemUser), parentDir)
	var stderr bytes.Buffer
	chownCmd.Stderr = &stderr
	if err := chownCmd.Run(); err != nil {
		// Fallback to www-data if caddy group doesn't exist
		chownCmdFallback := exec.Command("chown", "-R", fmt.Sprintf("%s:www-data", systemUser), parentDir)
		if errFallback := chownCmdFallback.Run(); errFallback != nil {
			return fmt.Errorf("failed to set directory ownership: %w", errFallback)
		}
	}

	// Restrict parent directory to 0750 (owner rwx, group r-x, others ---)
	// Owner is systemUser, group is caddy. This prevents other tenants (wp_other) from traversing into this site.
	_ = exec.Command("chmod", "0750", parentDir).Run()
	_ = exec.Command("chmod", "0750", htdocsPath).Run()
	_ = exec.Command("chmod", "0750", confPath).Run()

	// Secure backup directory to 0700 so database dumps cannot be accessed by web server or other users
	_ = exec.Command("chmod", "0700", backupPath).Run()

	// Secure tmp directory to 0750 so other site users cannot inspect temporary files
	_ = exec.Command("chmod", "0750", tmpPath).Run()

	fmt.Printf("OS: Provisioned site folders under %s with owner %s.\n", parentDir, systemUser)
	return nil
}

// DeleteSiteDirectory deletes the site directory and all content.
func DeleteSiteDirectory(path string) error {
	parentDir := GetSiteRootDir(path) // /var/www/[domain]
	if _, err := os.Stat(parentDir); err == nil {
		if err := os.RemoveAll(parentDir); err != nil {
			return fmt.Errorf("failed to delete site directory %s: %w", parentDir, err)
		}
		fmt.Printf("OS: Removed site directory %s.\n", parentDir)
	}
	return nil
}

// IsDirectoryLocked checks if the site root directory has the Linux immutable attribute (+i).
func IsDirectoryLocked(path string) bool {
	parentDir := GetSiteRootDir(path)
	if runtime.GOOS != "linux" {
		return false
	}
	cmd := exec.Command("lsattr", "-d", parentDir)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	// lsattr format: "----i---------e------- /var/www/domain"
	fields := strings.Fields(string(out))
	if len(fields) > 0 {
		attrs := fields[0]
		if len(attrs) >= 5 && attrs[4] == 'i' {
			return true
		}
		if strings.Contains(attrs, "i") {
			return true
		}
	}
	return false
}

// unlockDirectoryInternal removes the immutable flag from all files and directories under parentDir without logging.
func unlockDirectoryInternal(parentDir string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	// Remove immutable flag from regular files and directories, safely skipping symlinks.
	cmd := exec.Command("find", parentDir,
		"(", "-type", "f", "-o", "-type", "d", ")",
		"-exec", "chattr", "-f", "-i", "{}", "+",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove immutable flag: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}

// LockDirectory makes all regular files and directories within the site immutable,
// excluding logs, backups, and tmp so logging, backups, and temporary files continue to function,
// and skipping symbolic links which do not support Linux file attributes.
// It is fully idempotent: any existing immutable flags are cleanly cleared first to prevent
// Linux ext4 EPERM errors when setting +i on HTree-indexed directories.
func LockDirectory(path string) error {
	parentDir := GetSiteRootDir(path) // /var/www/[domain]

	if runtime.GOOS != "linux" {
		fmt.Printf("OS (Mock): Locking site directory %s (immutable)\n", parentDir)
		return nil
	}

	// 1. Linux ext4 idempotency guard:
	// HTree-indexed directories (wp-includes, wp-admin, large plugin directories with EXT4_INDEX_FL)
	// cause the ext4 kernel to reject 'chattr +i' with EPERM (exit code 1) if they already have
	// the immutable flag set.
	// We first clear any existing immutable flags across the directory. Clearing immutable
	// attributes ('chattr -i') is idempotent and always succeeds on ext4 regardless of prior state.
	if err := unlockDirectoryInternal(parentDir); err != nil {
		return fmt.Errorf("pre-lock unlock failed: %w", err)
	}

	// 2. Exclude logs, backup, and tmp directories from being locked
	// 3. Select only regular files (-type f) and directories (-type d)
	// 4. Skip symbolic links (-type l) and special files which cause chattr to fail with "Operation not supported"
	cmd := exec.Command("find", parentDir,
		"(", "-name", "logs", "-o", "-name", "log", "-o", "-name", "backup", "-o", "-name", "tmp", ")", "-prune",
		"-o", "(", "-type", "f", "-o", "-type", "d", ")",
		"-exec", "chattr", "-f", "+i", "{}", "+",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to make directory immutable: %w (stderr: %s)", err, stderr.String())
	}

	fmt.Printf("OS: Site directory %s set to immutable (locked).\n", parentDir)
	return nil
}

// UnlockDirectory removes the immutable flag from the site directory and its contents.
func UnlockDirectory(path string) error {
	parentDir := GetSiteRootDir(path) // /var/www/[domain]

	if runtime.GOOS != "linux" {
		fmt.Printf("OS (Mock): Unlocking site directory %s\n", parentDir)
		return nil
	}

	if err := unlockDirectoryInternal(parentDir); err != nil {
		return err
	}

	fmt.Printf("OS: Site directory %s removed from immutable status (unlocked).\n", parentDir)
	return nil
}

// FixPermissions resets files/directories owners and recursively sets directories to 0755, files to 0644, and wp-config.php to 0600.
func FixPermissions(parentDir string, systemUser string) error {
	if runtime.GOOS != "linux" {
		fmt.Printf("OS (Mock): Fix permissions recursively under %s for user %s\n", parentDir, systemUser)
		return nil
	}

	// 1. Reset ownership
	chownCmd := exec.Command("chown", "-R", fmt.Sprintf("%s:caddy", systemUser), parentDir)
	var stderr bytes.Buffer
	chownCmd.Stderr = &stderr
	if err := chownCmd.Run(); err != nil {
		chownCmdFallback := exec.Command("chown", "-R", fmt.Sprintf("%s:www-data", systemUser), parentDir)
		if errFallback := chownCmdFallback.Run(); errFallback != nil {
			return fmt.Errorf("failed to reset ownership: %w", errFallback)
		}
	}

	// 2. Set directory permissions recursively to 0750 (owner rwx, group r-x, others ---)
	findDirsCmd := exec.Command("find", parentDir, "-type", "d", "-exec", "chmod", "0750", "{}", "+")
	_ = findDirsCmd.Run()

	// 3. Set file permissions recursively to 0644
	findFilesCmd := exec.Command("find", parentDir, "-type", "f", "-exec", "chmod", "0644", "{}", "+")
	_ = findFilesCmd.Run()

	// 4. Secure backup and tmp directories specifically
	backupDir := filepath.Join(parentDir, "backup")
	_ = exec.Command("chmod", "0700", backupDir).Run()
	tmpDir := filepath.Join(parentDir, "tmp")
	_ = exec.Command("chmod", "0750", tmpDir).Run()

	// 5. Secure wp-config.php specifically in parentDir and parentDir/htdocs (if exists)
	for _, wpCfg := range []string{filepath.Join(parentDir, "wp-config.php"), filepath.Join(parentDir, "htdocs", "wp-config.php")} {
		if _, err := os.Stat(wpCfg); err == nil {
			chmodConfigCmd := exec.Command("chmod", "0600", wpCfg)
			_ = chmodConfigCmd.Run()
		}
	}

	fmt.Printf("OS: Fixed file/directory permissions recursively under %s.\n", parentDir)
	return nil
}

// ResolvePublicIP queries a public API or local interfaces to determine the server's public IPv4.
func ResolvePublicIP() string {
	client := http.Client{
		Timeout: 2 * time.Second,
	}
	resp, err := client.Get("https://api.ipify.org")
	if err == nil {
		defer resp.Body.Close()
		ipBytes, err := io.ReadAll(resp.Body)
		if err == nil {
			ip := strings.TrimSpace(string(ipBytes))
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	// Fallback: check network interfaces
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String()
				}
			}
		}
	}
	return "127.0.0.1"
}

// SafeExtractZip safely unpacks a ZIP archive into destDir with strict Zip Slip path traversal guards,
// directory containment verification, and symbolic link rejection.
func SafeExtractZip(srcZip string, destDir string) error {
	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return fmt.Errorf("failed to open zip file %s: %w", srcZip, err)
	}
	defer r.Close()

	destDirClean := filepath.Clean(destDir)

	for _, f := range r.File {
		// Clean and sanitize target path
		targetPath := filepath.Join(destDirClean, f.Name)
		targetPathClean := filepath.Clean(targetPath)

		// Strict Zip Slip defense: target must be inside destDirClean
		if !strings.HasPrefix(targetPathClean, destDirClean+string(os.PathSeparator)) && targetPathClean != destDirClean {
			return fmt.Errorf("security violation: illegal path '%s' outside destination directory", f.Name)
		}

		// Reject symbolic links to prevent symlink traversal attacks
		if f.Mode()&os.ModeSymlink != 0 {
			continue // Skip symbolic links for security
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPathClean, 0755); err != nil {
				return fmt.Errorf("failed to create directory in zip: %w", err)
			}
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(targetPathClean), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory for %s: %w", targetPathClean, err)
		}

		// Open source entry in zip
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to read file '%s' from zip: %w", f.Name, err)
		}

		// Strip setuid/setgid bits; enforce regular file mode 0644 (or 0755 if executable)
		mode := os.FileMode(0644)
		if f.Mode()&0111 != 0 {
			mode = 0755
		}

		outFile, err := os.OpenFile(targetPathClean, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create file '%s': %w", targetPathClean, err)
		}

		_, copyErr := io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if copyErr != nil {
			return fmt.Errorf("failed to write content to '%s': %w", targetPathClean, copyErr)
		}
	}

	return nil
}

