package server

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	// Create directories (including the target path itself, which might be a public subfolder)
	for _, dir := range []string{htdocsPath, confPath, backupPath, path} {
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

	chmodCmd := exec.Command("chmod", "-R", "0755", parentDir)
	if err := chmodCmd.Run(); err != nil {
		return fmt.Errorf("failed to set directory permissions: %w", err)
	}

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

// LockDirectory makes all files within the site directory immutable.
func LockDirectory(path string) error {
	parentDir := filepath.Dir(path) // /var/www/[domain]

	if runtime.GOOS != "linux" {
		fmt.Printf("OS (Mock): chattr -R +i %s\n", parentDir)
		return nil
	}

	cmd := exec.Command("chattr", "-R", "+i", parentDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to make directory immutable: %w (stderr: %s)", err, stderr.String())
	}

	fmt.Printf("OS: Site directory %s set to immutable (locked).\n", parentDir)
	return nil
}

// UnlockDirectory removes the immutable flag from the site directory.
func UnlockDirectory(path string) error {
	parentDir := filepath.Dir(path) // /var/www/[domain]

	if runtime.GOOS != "linux" {
		fmt.Printf("OS (Mock): chattr -R -i %s\n", parentDir)
		return nil
	}

	cmd := exec.Command("chattr", "-R", "-i", parentDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove immutable flag: %w (stderr: %s)", err, stderr.String())
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

	// 2. Set directory permissions recursively to 0755
	findDirsCmd := exec.Command("find", parentDir, "-type", "d", "-exec", "chmod", "0755", "{}", "+")
	_ = findDirsCmd.Run()

	// 3. Set file permissions recursively to 0644
	findFilesCmd := exec.Command("find", parentDir, "-type", "f", "-exec", "chmod", "0644", "{}", "+")
	_ = findFilesCmd.Run()

	// 4. Secure wp-config.php specifically in parentDir (if exists)
	wpConfigPath := filepath.Join(parentDir, "wp-config.php")
	if _, err := os.Stat(wpConfigPath); err == nil {
		chmodConfigCmd := exec.Command("chmod", "0600", wpConfigPath)
		_ = chmodConfigCmd.Run()
	}

	fmt.Printf("OS: Fixed file/directory permissions recursively under %s.\n", parentDir)
	return nil
}
