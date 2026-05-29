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
	"time"

	"agilepanel/internal/config"
)

type ProcessStatus struct {
	PID  int     `json:"pid"`
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Comm string  `json:"comm"`
}

type ServerStatus struct {
	Services          map[string]bool
	ActiveSites       int
	TotalMemoryGB     float64
	UsedMemoryGB      float64
	FreeMemoryGB      float64
	MemoryPercentage  float64
	TotalSwapGB       float64
	UsedSwapGB        float64
	FreeSwapGB        float64
	SwapPercentage    float64
	TotalDiskGB       float64
	UsedDiskGB        float64
	FreeDiskGB        float64
	DiskPercentage    float64
	Load1m            float64
	Load5m            float64
	Load15m           float64
	RealtimeCPU       float64
	TCPConnections    int
	TopProcesses      []ProcessStatus
	HasHistorical     bool
	PeakCPU24h        float64
	PeakMemory24h     float64
	PeakSwap24h       float64
	TopProcesses24h   []ProcessStatus
}

// GetStatus checks services and compiles system resource metrics.
func GetStatus() (*ServerStatus, error) {
	status := &ServerStatus{
		Services: make(map[string]bool),
	}

	// 1. Check services
	services := []string{"caddy", "mariadb", "redis-server"}
	
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err == nil {
		status.ActiveSites = len(state.Sites)
		for _, v := range state.Global.SupportedPHPVersions {
			services = append(services, fmt.Sprintf("php%s-fpm", v))
		}
	} else {
		services = append(services, "php8.3-fpm")
	}

	for _, svc := range services {
		if runtime.GOOS == "linux" {
			cmd := exec.Command("systemctl", "is-active", svc)
			err := cmd.Run()
			status.Services[svc] = (err == nil)
		} else {
			status.Services[svc] = false
		}
	}

	// 2. Read memory and swap info
	totalRAM, freeRAM, totalSwap, freeSwap, err := readLinuxMemoryAndSwap()
	if err == nil {
		status.TotalMemoryGB = float64(totalRAM) / (1024 * 1024)
		status.FreeMemoryGB = float64(freeRAM) / (1024 * 1024)
		status.UsedMemoryGB = status.TotalMemoryGB - status.FreeMemoryGB
		if totalRAM > 0 {
			status.MemoryPercentage = (status.UsedMemoryGB / status.TotalMemoryGB) * 100
		}

		status.TotalSwapGB = float64(totalSwap) / (1024 * 1024)
		status.FreeSwapGB = float64(freeSwap) / (1024 * 1024)
		status.UsedSwapGB = status.TotalSwapGB - status.FreeSwapGB
		if totalSwap > 0 {
			status.SwapPercentage = (status.UsedSwapGB / status.TotalSwapGB) * 100
		}
	}

	// 3. Read disk usage
	totalDisk, usedDisk, freeDisk, err := readDiskUsage()
	if err == nil {
		status.TotalDiskGB = float64(totalDisk) / (1024 * 1024 * 1024)
		status.UsedDiskGB = float64(usedDisk) / (1024 * 1024 * 1024)
		status.FreeDiskGB = float64(freeDisk) / (1024 * 1024 * 1024)
		if totalDisk > 0 {
			status.DiskPercentage = (status.UsedDiskGB / status.TotalDiskGB) * 100
		}
	}

	// 4. Read loadavg and CPU load
	m1, m5, m15, err := readLoadAvg()
	if err == nil {
		status.Load1m = m1
		status.Load5m = m5
		status.Load15m = m15
	}
	realCPU, err := getCPUPercentage()
	if err == nil {
		status.RealtimeCPU = realCPU
	}

	// 5. TCP Connections
	tcpCount, err := getTCPConnectionsCount()
	if err == nil {
		status.TCPConnections = tcpCount
	}

	// 6. Top 5 CPU Processes
	topProc, err := getTopProcesses("cpu")
	if err == nil {
		status.TopProcesses = topProc
	}

	// 7. Load historical data if present
	hStats, err := GetHistoricalMetrics(24 * time.Hour)
	if err == nil && hStats != nil && hStats.SampleCount > 0 {
		status.HasHistorical = true
		status.PeakCPU24h = hStats.PeakCPU
		status.PeakMemory24h = hStats.PeakMemory
		status.PeakSwap24h = hStats.PeakSwap
		status.TopProcesses24h = hStats.TopProcesses
	}

	return status, nil
}

func readLinuxMemoryAndSwap() (totalRAM, freeRAM, totalSwap, freeSwap int64, err error) {
	if runtime.GOOS != "linux" {
		return 2048576, 1024576, 2097152, 1997152, nil // Mock values
	}
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		val, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			totalRAM = val
		case "MemAvailable":
			freeRAM = val
		case "SwapTotal":
			totalSwap = val
		case "SwapFree":
			freeSwap = val
		}
	}
	return totalRAM, freeRAM, totalSwap, freeSwap, nil
}

func readLinuxMemory() (totalKB, freeKB int64, err error) {
	totalRAM, freeRAM, _, _, err := readLinuxMemoryAndSwap()
	return totalRAM, freeRAM, err
}

func readDiskUsage() (totalBytes, usedBytes, freeBytes uint64, err error) {
	if runtime.GOOS != "linux" {
		return 100 * 1024 * 1024 * 1024, 42 * 1024 * 1024 * 1024, 58 * 1024 * 1024 * 1024, nil // Mock 100GB
	}
	cmd := exec.Command("df", "-B1", "/")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, 0, err
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0, 0, 0, fmt.Errorf("invalid df output")
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return 0, 0, 0, fmt.Errorf("invalid df fields")
	}
	totalBytes, _ = strconv.ParseUint(fields[1], 10, 64)
	usedBytes, _ = strconv.ParseUint(fields[2], 10, 64)
	freeBytes, _ = strconv.ParseUint(fields[3], 10, 64)
	return totalBytes, usedBytes, freeBytes, nil
}

func readLoadAvg() (m1, m5, m15 float64, err error) {
	if runtime.GOOS != "linux" {
		return 0.25, 0.35, 0.45, nil // Mock values
	}
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("invalid loadavg format")
	}
	m1, _ = strconv.ParseFloat(fields[0], 64)
	m5, _ = strconv.ParseFloat(fields[1], 64)
	m15, _ = strconv.ParseFloat(fields[2], 64)
	return m1, m5, m15, nil
}

type cpuStats struct {
	user, nice, system, idle, iowait, irq, softirq, steal uint64
}

func readCPUStats() (cpuStats, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuStats{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 9 || fields[0] != "cpu" {
			return cpuStats{}, fmt.Errorf("invalid stat format")
		}
		u, _ := strconv.ParseUint(fields[1], 10, 64)
		n, _ := strconv.ParseUint(fields[2], 10, 64)
		s, _ := strconv.ParseUint(fields[3], 10, 64)
		id, _ := strconv.ParseUint(fields[4], 10, 64)
		io, _ := strconv.ParseUint(fields[5], 10, 64)
		irq, _ := strconv.ParseUint(fields[6], 10, 64)
		sirq, _ := strconv.ParseUint(fields[7], 10, 64)
		st, _ := strconv.ParseUint(fields[8], 10, 64)
		return cpuStats{u, n, s, id, io, irq, sirq, st}, nil
	}
	return cpuStats{}, fmt.Errorf("empty stat")
}

func getCPUPercentage() (float64, error) {
	if runtime.GOOS != "linux" {
		return 12.5, nil // Mock
	}
	s1, err := readCPUStats()
	if err != nil {
		return 0, err
	}
	time.Sleep(100 * time.Millisecond)
	s2, err := readCPUStats()
	if err != nil {
		return 0, err
	}

	idle1 := s1.idle + s1.iowait
	idle2 := s2.idle + s2.iowait
	nonIdle1 := s1.user + s1.nice + s1.system + s1.irq + s1.softirq + s1.steal
	nonIdle2 := s2.user + s2.nice + s2.system + s2.irq + s2.softirq + s2.steal

	total1 := idle1 + nonIdle1
	total2 := idle2 + nonIdle2

	totalDiff := total2 - total1
	idleDiff := idle2 - idle1

	if totalDiff == 0 {
		return 0.0, nil
	}
	return float64(totalDiff-idleDiff) / float64(totalDiff) * 100.0, nil
}

func getTCPConnectionsCount() (int, error) {
	if runtime.GOOS != "linux" {
		return 35, nil // Mock
	}
	count := 0
	for _, file := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "  sl") || strings.HasPrefix(line, "sl") {
				continue
			}
			count++
		}
	}
	return count, nil
}

func getTopProcesses(sortBy string) ([]ProcessStatus, error) {
	if runtime.GOOS != "linux" {
		return []ProcessStatus{
			{PID: 1001, CPU: 12.4, Mem: 3.5, Comm: "mariadbd"},
			{PID: 1002, CPU: 8.2, Mem: 2.1, Comm: "php-fpm8.3"},
			{PID: 1003, CPU: 4.1, Mem: 1.0, Comm: "caddy"},
			{PID: 1004, CPU: 1.5, Mem: 0.8, Comm: "redis-server"},
			{PID: 1005, CPU: 0.8, Mem: 0.5, Comm: "systemd"},
		}, nil
	}

	sortFlag := "-%cpu"
	if sortBy == "mem" {
		sortFlag = "-%mem"
	}

	cmd := exec.Command("ps", "-eo", "pid,%cpu,%mem,comm", "--sort="+sortFlag)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var list []ProcessStatus
	scanner := bufio.NewScanner(bytes.NewReader(output))
	isHeader := true
	for scanner.Scan() {
		if isHeader {
			isHeader = false
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		cpu, _ := strconv.ParseFloat(fields[1], 64)
		mem, _ := strconv.ParseFloat(fields[2], 64)
		comm := fields[3]

		if comm == "ps" || comm == "head" {
			continue
		}

		list = append(list, ProcessStatus{PID: pid, CPU: cpu, Mem: mem, Comm: comm})
		if len(list) >= 5 {
			break
		}
	}
	return list, nil
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
