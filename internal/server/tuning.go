package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// TuneServer auto-tunes system swap memory and database parameters based on hardware resources.
func TuneServer() error {
	fmt.Println("AgilePanel: Starting server optimization checklist...")

	// 1. Optimize Swap File
	if err := TuneSwap(); err != nil {
		fmt.Printf("Warning: Swap optimization failed: %v\n", err)
	}

	// 2. Optimize Database (MariaDB/MySQL)
	if err := TuneDatabase(); err != nil {
		fmt.Printf("Warning: Database optimization failed: %v\n", err)
	}

	fmt.Println("AgilePanel: Server optimization tuning completed successfully.")
	return nil
}

// TuneSwap checks swap settings and sets up a 2GB swap file + sysctl tweaks if swap is missing.
func TuneSwap() error {
	if runtime.GOOS != "linux" {
		fmt.Println("Swap (Mock): Configure 2GB swapfile and vm.swappiness=10")
		return nil
	}

	// Check if swap is already active
	swaponCheck := exec.Command("swapon", "--show")
	var stdout bytes.Buffer
	swaponCheck.Stdout = &stdout
	_ = swaponCheck.Run()

	hasSwap := false
	output := stdout.String()
	lines := strings.Split(output, "\n")
	if len(lines) > 1 {
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) != "" {
				hasSwap = true
				break
			}
		}
	}

	if hasSwap {
		fmt.Println("Swap: Swap memory is already active. Skipping swapfile creation.")
	} else {
		fmt.Println("Swap: No swap space detected. Creating 2GB swap file...")
		
		// 1. Allocate file
		falloc := exec.Command("fallocate", "-l", "2G", "/swapfile")
		if err := falloc.Run(); err != nil {
			// fallback to dd if fallocate fails
			dd := exec.Command("dd", "if=/dev/zero", "of=/swapfile", "bs=1M", "count=2048")
			if err := dd.Run(); err != nil {
				return fmt.Errorf("failed to allocate swapfile space: %w", err)
			}
		}
		
		// 2. Set permissions
		if err := exec.Command("chmod", "600", "/swapfile").Run(); err != nil {
			return fmt.Errorf("failed to chmod swapfile: %w", err)
		}
		
		// 3. Make swap
		if err := exec.Command("mkswap", "/swapfile").Run(); err != nil {
			return fmt.Errorf("failed to mkswap swapfile: %w", err)
		}
		
		// 4. Enable swap
		if err := exec.Command("swapon", "/swapfile").Run(); err != nil {
			return fmt.Errorf("failed to enable swapfile: %w", err)
		}

		// 5. Make swap persistent in fstab
		fstabBytes, err := ioutil.ReadFile("/etc/fstab")
		if err == nil && !strings.Contains(string(fstabBytes), "/swapfile") {
			fstabStr := string(fstabBytes)
			if !strings.HasSuffix(fstabStr, "\n") {
				fstabStr += "\n"
			}
			fstabStr += "/swapfile none swap sw 0 0\n"
			_ = ioutil.WriteFile("/etc/fstab", []byte(fstabStr), 0644)
		}
		fmt.Println("Swap: 2GB Swapfile successfully created and activated.")
	}

	// Optimize Sysctl parameters
	fmt.Println("Swap: Tuning kernel swappiness and cache settings...")
	_ = exec.Command("sysctl", "vm.swappiness=10").Run()
	_ = exec.Command("sysctl", "vm.vfs_cache_pressure=50").Run()

	sysctlConfPath := "/etc/sysctl.d/99-agilepanel.conf"
	sysctlContent := "vm.swappiness=10\nvm.vfs_cache_pressure=50\n"
	_ = ioutil.WriteFile(sysctlConfPath, []byte(sysctlContent), 0644)

	return nil
}

// TuneDatabase configures MySQL/MariaDB tuning parameters.
func TuneDatabase() error {
	totalMemKB, _, err := readLinuxMemory()
	
	// Default to 1GB if error or non-linux
	var memorySizeGB float64 = 1.0
	if err == nil && totalMemKB > 0 {
		memorySizeGB = float64(totalMemKB) / (1024 * 1024)
	}

	// Calculate InnoDB Buffer Pool size (approx. 30% of system RAM)
	bufferPoolMB := int(memorySizeGB * 1024 * 0.30)
	if bufferPoolMB < 128 {
		bufferPoolMB = 128 // Minimum standard buffer size
	}
	
	// Log file size is 25% of buffer pool size, capped at 256M
	logFileMB := bufferPoolMB / 4
	if logFileMB < 48 {
		logFileMB = 48
	} else if logFileMB > 256 {
		logFileMB = 256
	}

	cnfContent := fmt.Sprintf(`# AgilePanel Custom MariaDB Optimizations
[mysqld]
innodb_buffer_pool_size = %dM
innodb_log_file_size = %dM
innodb_flush_log_at_trx_commit = 2
innodb_flush_method = O_DIRECT
query_cache_type = 0
query_cache_size = 0
max_connections = 100
`, bufferPoolMB, logFileMB)

	if runtime.GOOS != "linux" {
		fmt.Printf("DB (Mock): Generate tuning configuration:\n%s", cnfContent)
		return nil
	}

	// Paths for MariaDB conf dropins
	cnfDirs := []string{
		"/etc/mysql/mariadb.conf.d",
		"/etc/mysql/conf.d",
	}

	written := false
	for _, dir := range cnfDirs {
		if _, err := os.Stat(dir); err == nil {
			filePath := filepath.Join(dir, "99-agilepanel-tune.cnf")
			if err := ioutil.WriteFile(filePath, []byte(cnfContent), 0644); err == nil {
				fmt.Printf("DB: Database optimization profile written to %s.\n", filePath)
				written = true
				break
			}
		}
	}

	if !written {
		// Fallback to /etc/mysql/conf.d/99-agilepanel-tune.cnf
		filePath := "/etc/mysql/conf.d/99-agilepanel-tune.cnf"
		_ = os.MkdirAll(filepath.Dir(filePath), 0755)
		if err := ioutil.WriteFile(filePath, []byte(cnfContent), 0644); err == nil {
			fmt.Printf("DB: Database optimization profile written to fallback path %s.\n", filePath)
		} else {
			return fmt.Errorf("failed to write database tuning file")
		}
	}

	// Trigger restart of MariaDB to apply
	fmt.Println("DB: Restarting MariaDB service to apply changes...")
	restartCmd := exec.Command("systemctl", "restart", "mariadb")
	if err := restartCmd.Run(); err != nil {
		_ = exec.Command("systemctl", "restart", "mysql").Run()
	}
	return nil
}
