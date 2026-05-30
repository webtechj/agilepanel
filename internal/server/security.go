package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/exec"
	"runtime"
	"strings"
)

// UnlockGuiPanel disables the secondary session lock layer by writing enabled=false to gui_auth.json.
func UnlockGuiPanel() error {
	path := "/etc/agilepanel/gui_auth.json"
	if runtime.GOOS == "windows" {
		path = "./gui_auth.json"
	}

	type guiAuth struct {
		Enabled      bool   `json:"enabled"`
		Username     string `json:"username"`
		PasswordHash string `json:"password_hash"`
	}

	config := guiAuth{Enabled: false}
	if data, err := ioutil.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &config)
	}

	config.Enabled = false
	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal gui auth: %w", err)
	}

	return ioutil.WriteFile(path, newData, 0644)
}

// SecureServer audits and seals security loopholes on Ubuntu Server.
func SecureServer() error {
	fmt.Println("AgilePanel: Starting server security audit and hardening...")

	// 1. Enforce 30-day SSH password rotation policy for root (using chage)
	if runtime.GOOS == "linux" {
		fmt.Println("Security: Enforcing 30-day password aging policy for 'root'...")
		cmd := exec.Command("chage", "-M", "30", "root")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Warning: Failed to set password aging for root: %v (stderr: %s)\n", err, stderr.String())
		} else {
			fmt.Println("Security: Password rotation set (users must change password every 30 days).")
		}

		// Update /etc/login.defs for future users
		loginDefsPath := "/etc/login.defs"
		if data, err := ioutil.ReadFile(loginDefsPath); err == nil {
			content := string(data)
			lines := strings.Split(content, "\n")
			modified := false
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "PASS_MAX_DAYS") {
					lines[i] = "PASS_MAX_DAYS   30"
					modified = true
				}
			}
			if modified {
				_ = ioutil.WriteFile(loginDefsPath, []byte(strings.Join(lines, "\n")), 0644)
				fmt.Println("Security: Updated /etc/login.defs PASS_MAX_DAYS to 30.")
			}
		}
	} else {
		fmt.Println("Security (Mock): Enforce 30-day password aging policy for root.")
	}

	// 2. Configure UFW Firewall
	if runtime.GOOS == "linux" {
		// Check if ufw command exists
		if _, err := exec.LookPath("ufw"); err == nil {
			fmt.Println("Security: Configuring UFW Firewall rules...")
			
			// Allow SSH, HTTP, HTTPS and AgilePanel custom ports
			_ = exec.Command("ufw", "allow", "22/tcp").Run()
			_ = exec.Command("ufw", "allow", "80/tcp").Run()
			_ = exec.Command("ufw", "allow", "443/tcp").Run()
			_ = exec.Command("ufw", "allow", "8888/tcp").Run()
			
			// Enable UFW
			cmdEnable := exec.Command("ufw", "--force", "enable")
			if err := cmdEnable.Run(); err != nil {
				fmt.Printf("Warning: Failed to enable UFW: %v\n", err)
			} else {
				fmt.Println("Security: UFW Firewall successfully enabled and configured.")
			}
		} else {
			fmt.Println("Security Warning: 'ufw' utility is not installed. Recommended: 'apt install ufw'.")
		}
	} else {
		fmt.Println("Security (Mock): Enabled UFW and opened ports 22, 80, 443, 8888.")
	}

	// 3. Harden SSH Configuration (/etc/ssh/sshd_config)
	if runtime.GOOS == "linux" {
		sshdPath := "/etc/ssh/sshd_config"
		if data, err := ioutil.ReadFile(sshdPath); err == nil {
			content := string(data)
			modified := false

			// Ensure PermitRootLogin is either prohibit-password or we warn user
			// We will set PermitRootLogin to prohibit-password (allows keys, blocks passwords for root)
			if strings.Contains(content, "PermitRootLogin yes") {
				content = strings.ReplaceAll(content, "PermitRootLogin yes", "PermitRootLogin prohibit-password")
				modified = true
			} else if !strings.Contains(content, "PermitRootLogin prohibit-password") && !strings.Contains(content, "PermitRootLogin no") {
				content += "\nPermitRootLogin prohibit-password\n"
				modified = true
			}

			if modified {
				if err := ioutil.WriteFile(sshdPath, []byte(content), 0644); err == nil {
					fmt.Println("Security: SSH Configuration hardened (PermitRootLogin set to prohibit-password).")
					// Reload SSH daemon
					_ = exec.Command("systemctl", "reload", "ssh").Run()
					_ = exec.Command("systemctl", "reload", "sshd").Run()
				}
			}
		}
	} else {
		fmt.Println("Security (Mock): Hardened PermitRootLogin inside sshd_config.")
	}

	return nil
}
