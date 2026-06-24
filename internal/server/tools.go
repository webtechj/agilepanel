package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
		_ = os.WriteFile(filepath.Join(mockDir, "config.inc.php"), []byte("<?php $cfg['blowfish_secret'] = 'mock'; ?>"), 0644)
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

	// Generate a cryptographically secure 32-byte blowfish secret
	fmt.Println("Tools: Generating phpMyAdmin secure config...")
	secretBytes := make([]byte, 32)
	_, _ = rand.Read(secretBytes)
	blowfishSecret := hex.EncodeToString(secretBytes)

	// Write a complete config.inc.php from scratch — avoids brittle regex parsing
	// of config.sample.inc.php which differs across phpMyAdmin versions.
	configPath := filepath.Join(destDir, "config.inc.php")
	configContent := "<?php\n" +
		"/**\n" +
		" * AgilePanel - phpMyAdmin Configuration\n" +
		" * Generated automatically. Do not edit manually.\n" +
		" */\n" +
		"declare(strict_types=1);\n\n" +
		"$cfg['blowfish_secret'] = '" + blowfishSecret + "';\n\n" +
		"$i = 0;\n" +
		"$i++;\n\n" +
		"/* Server configuration */\n" +
		"$cfg['Servers'][$i]['auth_type']      = 'cookie';\n" +
		"$cfg['Servers'][$i]['host']           = '127.0.0.1';\n" +
		"$cfg['Servers'][$i]['connect_type']   = 'tcp';\n" +
		"$cfg['Servers'][$i]['compress']       = false;\n" +
		"$cfg['Servers'][$i]['AllowNoPassword'] = false;\n\n" +
		"/* Upload/Save directories */\n" +
		"$cfg['UploadDir'] = '';\n" +
		"$cfg['SaveDir']   = '';\n\n" +
		"/* Temp directory for phpMyAdmin */\n" +
		"$cfg['TempDir'] = '/tmp/phpmyadmin_tmp/';\n"

	if err := os.WriteFile(configPath, []byte(configContent), 0640); err != nil {
		return fmt.Errorf("failed to write phpMyAdmin config: %w", err)
	}

	// Create temp dir for phpMyAdmin sessions
	_ = os.MkdirAll("/tmp/phpmyadmin_tmp", 0777)

	// Set correct permissions
	_ = exec.Command("chown", "-R", "www-data:www-data", destDir).Run()
	_ = exec.Command("chmod", "-R", "0755", destDir).Run()
	_ = exec.Command("chmod", "0640", configPath).Run()

	// Configure firewall rules for port 8888
	if _, err := exec.LookPath("ufw"); err == nil {
		fmt.Println("Tools: Opening port 8888 in UFW...")
		_ = exec.Command("ufw", "allow", "8888/tcp").Run()
	} else if _, err := exec.LookPath("firewall-cmd"); err == nil {
		fmt.Println("Tools: Opening port 8888 in firewalld...")
		_ = exec.Command("firewall-cmd", "--permanent", "--add-port=8888/tcp").Run()
		_ = exec.Command("firewall-cmd", "--reload").Run()
	}

	fmt.Println("Tools: phpMyAdmin successfully installed and secured.")
	return nil
}

// FixPhpMyAdminConfig regenerates config.inc.php for an existing phpMyAdmin installation.
// Use this to recover from config errors without reinstalling.
func FixPhpMyAdminConfig() error {
	destDir := "/usr/share/phpmyadmin"

	if runtime.GOOS != "linux" {
		fmt.Println("Tools (Mock): Regenerating phpMyAdmin config.inc.php")
		return nil
	}

	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		return fmt.Errorf("phpMyAdmin is not installed at %s. Run 'ap tool install phpmyadmin' first", destDir)
	}

	// Generate a fresh blowfish secret
	secretBytes := make([]byte, 32)
	_, _ = rand.Read(secretBytes)
	blowfishSecret := hex.EncodeToString(secretBytes)

	configPath := filepath.Join(destDir, "config.inc.php")
	configContent := "<?php\n" +
		"/**\n" +
		" * AgilePanel - phpMyAdmin Configuration\n" +
		" * Regenerated automatically by 'ap tool fix-phpmyadmin'.\n" +
		" */\n" +
		"declare(strict_types=1);\n\n" +
		"$cfg['blowfish_secret'] = '" + blowfishSecret + "';\n\n" +
		"$i = 0;\n" +
		"$i++;\n\n" +
		"/* Server configuration */\n" +
		"$cfg['Servers'][$i]['auth_type']       = 'cookie';\n" +
		"$cfg['Servers'][$i]['host']            = '127.0.0.1';\n" +
		"$cfg['Servers'][$i]['connect_type']    = 'tcp';\n" +
		"$cfg['Servers'][$i]['compress']        = false;\n" +
		"$cfg['Servers'][$i]['AllowNoPassword'] = false;\n\n" +
		"/* Upload/Save directories */\n" +
		"$cfg['UploadDir'] = '';\n" +
		"$cfg['SaveDir']   = '';\n\n" +
		"/* Temp directory for phpMyAdmin */\n" +
		"$cfg['TempDir'] = '/tmp/phpmyadmin_tmp/';\n"

	if err := os.WriteFile(configPath, []byte(configContent), 0640); err != nil {
		return fmt.Errorf("failed to write phpMyAdmin config: %w", err)
	}

	// Ensure temp directory exists
	_ = os.MkdirAll("/tmp/phpmyadmin_tmp", 0777)

	// Restore ownership and permissions
	_ = exec.Command("chown", "-R", "www-data:www-data", destDir).Run()
	_ = exec.Command("chmod", "-R", "0755", destDir).Run()
	_ = exec.Command("chmod", "0640", configPath).Run()

	fmt.Println("Tools: phpMyAdmin config.inc.php regenerated successfully.")
	return nil
}

// InstallGui downloads the pre-compiled AgilePanel Web GUI, configures systemd, starts the service, and configures firewall rules.
func InstallGui() error {
	destPath := "/usr/local/bin/agilepanel-gui"

	if runtime.GOOS != "linux" {
		fmt.Printf("Tools (Mock): Installing AgilePanel Web GUI addon to %s...\n", destPath)
		return nil
	}

	// Ensure system dependencies for backup/restore (zip/unzip) are present
	fmt.Println("Tools: Ensuring system zip and unzip utility packages are installed...")
	_ = exec.Command("apt-get", "install", "-y", "zip", "unzip").Run()

	// Stop service if running and remove old binary to prevent "Text file busy" (exit status 23)
	fmt.Println("Tools: Stopping existing agilepanel-gui daemon...")
	_ = exec.Command("systemctl", "stop", "agilepanel-gui").Run()
	_ = os.Remove(destPath)

	fmt.Println("Tools: Downloading AgilePanel GUI companion binary...")
	branch := os.Getenv("AGILEPANEL_UPDATE_BRANCH")
	var guiURL string
	if branch != "" {
		guiURL = fmt.Sprintf("https://raw.githubusercontent.com/webtechj/agilepanel-gui/%s/agilepanel-gui-linux-amd64", branch)
		fmt.Printf("Tools: Using branch override '%s'...\n", branch)
	} else {
		guiURL = "https://github.com/webtechj/agilepanel-gui/releases/latest/download/agilepanel-gui-linux-amd64"
	}
	downloadCmd := exec.Command("curl", "-L", "-o", destPath, guiURL)
	if err := downloadCmd.Run(); err != nil {
		return fmt.Errorf("failed to download agilepanel-gui binary: %w", err)
	}

	fmt.Println("Tools: Setting executable permissions on GUI binary...")
	if err := exec.Command("chmod", "+x", destPath).Run(); err != nil {
		return fmt.Errorf("failed to set execution permission on GUI binary: %w", err)
	}

	fmt.Println("Tools: Configuring systemd service 'agilepanel-gui.service'...")
	serviceContent := `[Unit]
Description=AgilePanel Web GUI Daemon
After=network.target caddy.service mariadb.service

[Service]
Type=simple
User=root
WorkingDirectory=/usr/local/bin
ExecStart=/usr/local/bin/agilepanel-gui
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
	servicePath := "/etc/systemd/system/agilepanel-gui.service"
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write systemd service file: %w", err)
	}

	fmt.Println("Tools: Enabling and starting agilepanel-gui service...")
	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = exec.Command("systemctl", "enable", "agilepanel-gui").Run()
	if err := exec.Command("systemctl", "restart", "agilepanel-gui").Run(); err != nil {
		return fmt.Errorf("failed to start agilepanel-gui service: %w", err)
	}

	// Configure firewall rules for port 8889
	if _, err := exec.LookPath("ufw"); err == nil {
		fmt.Println("Tools: Opening port 8889 in UFW...")
		_ = exec.Command("ufw", "allow", "8889/tcp").Run()
	} else if _, err := exec.LookPath("firewall-cmd"); err == nil {
		fmt.Println("Tools: Opening port 8889 in firewalld...")
		_ = exec.Command("firewall-cmd", "--permanent", "--add-port=8889/tcp").Run()
		_ = exec.Command("firewall-cmd", "--reload").Run()
	}

	fmt.Println("Tools: AgilePanel Web GUI successfully installed and started.")
	return nil
}

// DisableGui stops and disables the agilepanel-gui systemd service and blocks port 8889.
func DisableGui() error {
	if runtime.GOOS != "linux" {
		fmt.Println("Tools (Mock): Disabling AgilePanel Web GUI addon...")
		return nil
	}

	fmt.Println("Tools: Stopping agilepanel-gui daemon...")
	_ = exec.Command("systemctl", "stop", "agilepanel-gui").Run()

	fmt.Println("Tools: Disabling agilepanel-gui service...")
	_ = exec.Command("systemctl", "disable", "agilepanel-gui").Run()

	// Block/remove firewall rules for port 8889
	if _, err := exec.LookPath("ufw"); err == nil {
		fmt.Println("Tools: Removing port 8889 from UFW...")
		_ = exec.Command("ufw", "deny", "8889/tcp").Run()
		_ = exec.Command("ufw", "delete", "allow", "8889/tcp").Run()
	} else if _, err := exec.LookPath("firewall-cmd"); err == nil {
		fmt.Println("Tools: Removing port 8889 from firewalld...")
		_ = exec.Command("firewall-cmd", "--permanent", "--remove-port=8889/tcp").Run()
		_ = exec.Command("firewall-cmd", "--reload").Run()
	}

	fmt.Println("Tools: AgilePanel Web GUI successfully disabled.")
	return nil
}

// EnableGui starts and enables the agilepanel-gui systemd service and opens port 8889.
func EnableGui() error {
	destPath := "/usr/local/bin/agilepanel-gui"

	if runtime.GOOS != "linux" {
		fmt.Println("Tools (Mock): Enabling AgilePanel Web GUI addon...")
		return nil
	}

	// Check if already installed / binary exists
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		return fmt.Errorf("AgilePanel Web GUI is not installed. Run 'ap install gui' first")
	}

	// Ensure systemd service is configured
	servicePath := "/etc/systemd/system/agilepanel-gui.service"
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		fmt.Println("Tools: Re-configuring systemd service 'agilepanel-gui.service'...")
		serviceContent := `[Unit]
Description=AgilePanel Web GUI Daemon
After=network.target caddy.service mariadb.service

[Service]
Type=simple
User=root
WorkingDirectory=/usr/local/bin
ExecStart=/usr/local/bin/agilepanel-gui
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
		if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
			return fmt.Errorf("failed to write systemd service file: %w", err)
		}
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}

	fmt.Println("Tools: Enabling agilepanel-gui service...")
	_ = exec.Command("systemctl", "enable", "agilepanel-gui").Run()

	fmt.Println("Tools: Starting agilepanel-gui service...")
	if err := exec.Command("systemctl", "restart", "agilepanel-gui").Run(); err != nil {
		return fmt.Errorf("failed to start agilepanel-gui service: %w", err)
	}

	// Configure firewall rules for port 8889
	if _, err := exec.LookPath("ufw"); err == nil {
		fmt.Println("Tools: Opening port 8889 in UFW...")
		_ = exec.Command("ufw", "allow", "8889/tcp").Run()
	} else if _, err := exec.LookPath("firewall-cmd"); err == nil {
		fmt.Println("Tools: Opening port 8889 in firewalld...")
		_ = exec.Command("firewall-cmd", "--permanent", "--add-port=8889/tcp").Run()
		_ = exec.Command("firewall-cmd", "--reload").Run()
	}

	fmt.Println("Tools: AgilePanel Web GUI successfully enabled.")
	return nil
}

// UpdateGui downloads the latest AgilePanel Web GUI binary from GitHub, replaces the existing one, and restarts the service.
func UpdateGui() error {
	destPath := "/usr/local/bin/agilepanel-gui"

	if runtime.GOOS != "linux" {
		fmt.Printf("Tools (Mock): Updating AgilePanel Web GUI to latest version...\n")
		return nil
	}

	// Check if already installed / binary exists
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		return fmt.Errorf("AgilePanel Web GUI is not installed. Run 'ap install gui' first")
	}

	// Stop service if running and remove old binary to prevent "Text file busy"
	fmt.Println("Tools: Stopping existing agilepanel-gui daemon...")
	_ = exec.Command("systemctl", "stop", "agilepanel-gui").Run()

	_ = os.Remove(destPath)

	fmt.Println("Tools: Downloading latest AgilePanel GUI companion binary from GitHub...")
	branch := os.Getenv("AGILEPANEL_UPDATE_BRANCH")
	var guiURL string
	if branch != "" {
		guiURL = fmt.Sprintf("https://raw.githubusercontent.com/webtechj/agilepanel-gui/%s/agilepanel-gui-linux-amd64", branch)
		fmt.Printf("Tools: Using branch override '%s'...\n", branch)
	} else {
		guiURL = "https://github.com/webtechj/agilepanel-gui/releases/latest/download/agilepanel-gui-linux-amd64"
	}
	downloadCmd := exec.Command("curl", "-L", "-o", destPath, guiURL)
	if err := downloadCmd.Run(); err != nil {
		return fmt.Errorf("failed to download agilepanel-gui binary: %w", err)
	}

	fmt.Println("Tools: Setting executable permissions on GUI binary...")
	if err := exec.Command("chmod", "+x", destPath).Run(); err != nil {
		return fmt.Errorf("failed to set execution permission on GUI binary: %w", err)
	}

	// Re-create service file if it doesn't exist (safety fallback)
	servicePath := "/etc/systemd/system/agilepanel-gui.service"
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		serviceContent := `[Unit]
Description=AgilePanel Web GUI Daemon
After=network.target caddy.service mariadb.service

[Service]
Type=simple
User=root
WorkingDirectory=/usr/local/bin
ExecStart=/usr/local/bin/agilepanel-gui
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
		if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
			return fmt.Errorf("failed to write systemd service file: %w", err)
		}
		_ = exec.Command("systemctl", "daemon-reload").Run()
		_ = exec.Command("systemctl", "enable", "agilepanel-gui").Run()
	}

	fmt.Println("Tools: Starting updated agilepanel-gui service...")
	if err := exec.Command("systemctl", "restart", "agilepanel-gui").Run(); err != nil {
		return fmt.Errorf("failed to restart agilepanel-gui service: %w", err)
	}

	fmt.Println("Tools: AgilePanel Web GUI successfully updated.")
	return nil
}

// GetGuiInfo returns whether the GUI addon is installed and its version.
func GetGuiInfo() (bool, string) {
	guiPath := "/usr/local/bin/agilepanel-gui"
	if runtime.GOOS == "windows" {
		// Try to find the executable in the parent directory for development/mocking
		devPath := filepath.Join("..", "agilepanel-gui", "agilepanel-gui.exe")
		if _, err := os.Stat(devPath); err == nil {
			guiPath = devPath
		} else {
			guiPath = "./agilepanel-gui.exe"
		}
	}

	if _, err := os.Stat(guiPath); err != nil {
		return false, ""
	}

	// Try executing with --version flag
	cmd := exec.Command(guiPath, "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		version := strings.TrimSpace(out.String())
		version = strings.TrimPrefix(version, "agilepanel-gui version ")
		version = strings.TrimPrefix(version, "version ")
		if version != "" {
			return true, version
		}
	}

	return true, "1.0.2"
}



