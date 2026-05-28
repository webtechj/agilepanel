package server

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"agilepanel/internal/config"
)

type ServerStatus struct {
	Services   map[string]bool
	ActiveSites int
	TotalMemory string
	FreeMemory  string
}

// GetStatus checks services and compiles system resource metrics.
func GetStatus() (*ServerStatus, error) {
	status := &ServerStatus{
		Services: make(map[string]bool),
	}

	// 1. Check services
	services := []string{"caddy", "mariadb", "redis-server"}
	
	// Add PHP versions to check from state
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err == nil {
		status.ActiveSites = len(state.Sites)
		// Check PHP pools versions
		for _, v := range state.Global.SupportedPHPVersions {
			services = append(services, fmt.Sprintf("php%s-fpm", v))
		}
	} else {
		// fallback php version
		services = append(services, "php8.3-fpm")
	}

	for _, svc := range services {
		if runtime.GOOS == "linux" {
			cmd := exec.Command("systemctl", "is-active", svc)
			err := cmd.Run()
			status.Services[svc] = (err == nil)
		} else {
			// Mocked active status for testing on non-Linux
			status.Services[svc] = false
		}
	}

	// 2. Read memory info
	if runtime.GOOS == "linux" {
		total, free, err := readLinuxMemory()
		if err == nil {
			status.TotalMemory = fmt.Sprintf("%.2f GB", float64(total)/(1024*1024))
			status.FreeMemory = fmt.Sprintf("%.2f GB", float64(free)/(1024*1024))
		} else {
			status.TotalMemory = "Unknown"
			status.FreeMemory = "Unknown"
		}
	} else {
		status.TotalMemory = "N/A (Non-Linux)"
		status.FreeMemory = "N/A (Non-Linux)"
	}

	return status, nil
}

func readLinuxMemory() (totalKB int64, freeKB int64, err error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var memTotal, memAvailable int64
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		val, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}

		if key == "MemTotal" {
			memTotal = val
		} else if key == "MemAvailable" {
			memAvailable = val
			break // we got both
		}
	}

	return memTotal, memAvailable, nil
}

// RestartService restarts target services or all registered dependencies using systemctl.
func RestartService(svc string) error {
	services := []string{"caddy", "mariadb", "redis-server"}
	
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	var phpVersions []string
	if err == nil {
		phpVersions = state.Global.SupportedPHPVersions
	} else {
		phpVersions = []string{"8.3"}
	}

	var targetServices []string
	if svc == "all" {
		targetServices = append(targetServices, services...)
		for _, v := range phpVersions {
			targetServices = append(targetServices, fmt.Sprintf("php%s-fpm", v))
		}
	} else {
		valid := false
		if svc == "redis" {
			svc = "redis-server"
		}
		if svc == "mysql" {
			svc = "mariadb"
		}
		
		for _, s := range services {
			if s == svc {
				valid = true
			}
		}
		for _, v := range phpVersions {
			phpSvc := fmt.Sprintf("php%s-fpm", v)
			if phpSvc == svc || svc == fmt.Sprintf("php%s", v) {
				svc = phpSvc
				valid = true
			}
		}
		if !valid {
			targetServices = []string{svc}
		} else {
			targetServices = []string{svc}
		}
	}

	for _, s := range targetServices {
		if runtime.GOOS != "linux" {
			fmt.Printf("Service (Mock): systemctl restart %s\n", s)
			continue
		}

		fmt.Printf("Service: Restarting %s...\n", s)
		cmd := exec.Command("systemctl", "restart", s)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to restart service %s: %w (stderr: %s)", s, err, stderr.String())
		}
		fmt.Printf("Service: %s restarted successfully.\n", s)
	}

	return nil
}
