package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
)

// InstallPhpMyAdmin downloads and configures phpMyAdmin in /usr/share/phpmyadmin.
func InstallPhpMyAdmin() error {
	destDir := "/usr/share/phpmyadmin"

	if runtime.GOOS != "linux" {
		// Mock installation for testing on Windows
		fmt.Printf("Tools (Mock): Installing phpMyAdmin to %s...\n", destDir)
		mockDir := filepath.Join("usr", "share", "phpmyadmin")
		_ = os.MkdirAll(mockDir, 0755)
		_ = os.WriteFile(filepath.Join(mockDir, "index.php"), []byte("<?php echo 'phpMyAdmin Mock'; ?>"), 0644)
		_ = os.WriteFile(filepath.Join(mockDir, "config.sample.inc.php"), []byte("<?php $cfg['blowfish_secret'] = ''; ?>"), 0644)
		fmt.Println("Tools (Mock): Configured config.inc.php with secure blowfish secret.")
		return nil
	}

	// Check if already installed
	if _, err := os.Stat(destDir); err == nil {
		return fmt.Errorf("phpMyAdmin is already installed at %s", destDir)
	}

	fmt.Println("Tools: Downloading phpMyAdmin 5.2.1...")
	tmpZip := "/tmp/phpmyadmin.zip"
	downloadCmd := exec.Command("curl", "-L", "-o", tmpZip, "https://files.phpmyadmin.net/phpMyAdmin/5.2.1/phpMyAdmin-5.2.1-all-languages.zip")
	if err := downloadCmd.Run(); err != nil {
		return fmt.Errorf("failed to download phpMyAdmin zip: %w", err)
	}
	defer os.Remove(tmpZip)

	fmt.Println("Tools: Extracting phpMyAdmin...")
	tmpExtractDir := "/tmp/phpmyadmin_extracted"
	_ = os.RemoveAll(tmpExtractDir)
	unzipCmd := exec.Command("unzip", "-q", tmpZip, "-d", tmpExtractDir)
	if err := unzipCmd.Run(); err != nil {
		return fmt.Errorf("failed to unzip phpMyAdmin: %w", err)
	}
	defer os.RemoveAll(tmpExtractDir)

	// The extracted folder is usually phpMyAdmin-5.2.1-all-languages
	extractedFolders, err := os.ReadDir(tmpExtractDir)
	if err != nil || len(extractedFolders) == 0 {
		return fmt.Errorf("failed to locate extracted folder: %v", err)
	}
	srcDir := filepath.Join(tmpExtractDir, extractedFolders[0].Name())

	// Ensure parent destination directory exists
	if err := os.MkdirAll(filepath.Dir(destDir), 0755); err != nil {
		return fmt.Errorf("failed to create destination parent folder: %w", err)
	}

	// Move to /usr/share/phpmyadmin
	moveCmd := exec.Command("mv", srcDir, destDir)
	if err := moveCmd.Run(); err != nil {
		return fmt.Errorf("failed to move phpMyAdmin to destination: %w", err)
	}

	// Configure blowfish_secret
	fmt.Println("Tools: Generating phpMyAdmin secure config...")
	secretBytes := make([]byte, 16)
	_, _ = rand.Read(secretBytes)
	blowfishSecret := hex.EncodeToString(secretBytes)

	configSamplePath := filepath.Join(destDir, "config.sample.inc.php")
	configPath := filepath.Join(destDir, "config.inc.php")

	configBytes, err := os.ReadFile(configSamplePath)
	if err != nil {
		return fmt.Errorf("failed to read config sample: %w", err)
	}

	configStr := string(configBytes)
	// Replace $cfg['blowfish_secret'] = ''; with $cfg['blowfish_secret'] = 'secret';
	importRegex := regexp.MustCompile(`\$cfg\['blowfish_secret'\]\s*=\s*['"]['"];`)
	configStr = importRegex.ReplaceAllString(configStr, fmt.Sprintf("$cfg['blowfish_secret'] = '%s';", blowfishSecret))

	if err := os.WriteFile(configPath, []byte(configStr), 0644); err != nil {
		return fmt.Errorf("failed to write phpMyAdmin config: %w", err)
	}

	// Set correct permissions
	_ = exec.Command("chown", "-R", "caddy:caddy", destDir).Run()
	_ = exec.Command("chmod", "-R", "0755", destDir).Run()

	fmt.Println("Tools: phpMyAdmin successfully installed and secured.")
	return nil
}
