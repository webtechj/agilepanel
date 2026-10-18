package server

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"agilepanel/internal/config"
	"agilepanel/internal/ui"
)

// SystemHardwareProfile captures audited specifications of the host machine.
type SystemHardwareProfile struct {
	TotalRAMMB     int64
	FreeRAMMB      int64
	TotalSwapMB    int64
	FreeSwapMB     int64
	CPUCores       int
	DiskTotalGB    float64
	DiskFreeGB     float64
	DiskUsedGB     float64
	IsSSD          bool
	FilesystemType string
	Virtualization string
	IsContainer    bool
}

// AuditHardwareProfile inspects CPU, RAM, disk, virtualization, and storage type.
func AuditHardwareProfile() *SystemHardwareProfile {
	totalRAM, freeRAM, totalSwap, freeSwap, _ := readLinuxMemoryAndSwap()
	totalDisk, usedDisk, freeDisk, _ := readDiskUsage()

	cores := runtime.NumCPU()
	if cores < 1 {
		cores = 1
	}

	virtType, isContainer := detectVirtualization()
	isSSD := detectStorageType()
	fsType := detectRootFilesystem()

	return &SystemHardwareProfile{
		TotalRAMMB:     totalRAM / 1024,
		FreeRAMMB:      freeRAM / 1024,
		TotalSwapMB:    totalSwap / 1024,
		FreeSwapMB:     freeSwap / 1024,
		CPUCores:       cores,
		DiskTotalGB:    float64(totalDisk) / (1024 * 1024 * 1024),
		DiskFreeGB:     float64(freeDisk) / (1024 * 1024 * 1024),
		DiskUsedGB:     float64(usedDisk) / (1024 * 1024 * 1024),
		IsSSD:          isSSD,
		FilesystemType: fsType,
		Virtualization: virtType,
		IsContainer:    isContainer,
	}
}

func detectVirtualization() (string, bool) {
	if runtime.GOOS != "linux" {
		return "bare-metal", false
	}

	// 1. Try systemd-detect-virt
	cmd := exec.Command("systemd-detect-virt")
	out, err := cmd.Output()
	if err == nil {
		virt := strings.ToLower(strings.TrimSpace(string(out)))
		if virt != "" && virt != "none" {
			containerVirts := map[string]bool{
				"lxc":            true,
				"openvz":         true,
				"docker":         true,
				"podman":         true,
				"containerd":     true,
				"systemd-nspawn": true,
				"wsl":            true,
			}
			return virt, containerVirts[virt]
		}
	}

	// 2. Check container indicators
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker", true
	}
	if _, err := os.Stat("/proc/user_beancounters"); err == nil {
		return "openvz", true
	}
	if cgroupData, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		cgroupStr := string(cgroupData)
		if strings.Contains(cgroupStr, "docker") {
			return "docker", true
		}
		if strings.Contains(cgroupStr, "lxc") {
			return "lxc", true
		}
	}

	// 3. Check DMI product/vendor for VMs
	for _, path := range []string{"/sys/class/dmi/id/product_name", "/sys/class/dmi/id/sys_vendor"} {
		if data, err := os.ReadFile(path); err == nil {
			str := strings.ToLower(string(data))
			if strings.Contains(str, "kvm") {
				return "kvm", false
			}
			if strings.Contains(str, "qemu") {
				return "qemu", false
			}
			if strings.Contains(str, "vmware") {
				return "vmware", false
			}
			if strings.Contains(str, "virtualbox") {
				return "virtualbox", false
			}
			if strings.Contains(str, "xen") {
				return "xen", false
			}
			if strings.Contains(str, "amazon") || strings.Contains(str, "digitalocean") || strings.Contains(str, "linode") || strings.Contains(str, "vultr") || strings.Contains(str, "hetzner") {
				return "cloud-vps", false
			}
		}
	}

	return "bare-metal", false
}

func detectStorageType() bool {
	if runtime.GOOS != "linux" {
		return true // Default to SSD
	}

	entries, err := filepath.Glob("/sys/block/*/queue/rotational")
	if err == nil && len(entries) > 0 {
		for _, entry := range entries {
			if strings.Contains(entry, "loop") || strings.Contains(entry, "ram") || strings.Contains(entry, "sr") {
				continue
			}
			data, err := os.ReadFile(entry)
			if err == nil {
				val := strings.TrimSpace(string(data))
				if val == "0" {
					return true // Non-rotational SSD / NVMe
				} else if val == "1" {
					return false // Rotational HDD
				}
			}
		}
	}
	return true // Cloud/VPS defaults to SSD
}

func detectRootFilesystem() string {
	if runtime.GOOS != "linux" {
		return "ext4"
	}

	data, err := os.ReadFile("/proc/mounts")
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) >= 3 && fields[1] == "/" {
				return fields[2]
			}
		}
	}

	cmd := exec.Command("df", "-T", "/")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 2 {
				return fields[1]
			}
		}
	}
	return "ext4"
}

func printHardwareAuditCard(p *SystemHardwareProfile) {
	storageType := "NVMe/SSD"
	if !p.IsSSD {
		storageType = "Rotational HDD"
	}

	virtDesc := strings.ToUpper(p.Virtualization)
	if p.IsContainer {
		virtDesc += " (Container - Host Managed Kernel)"
	} else if p.Virtualization == "kvm" || p.Virtualization == "qemu" {
		virtDesc += " (Hardware Virtual Machine)"
	}

	fmt.Println()
	fmt.Printf("  %s %s\n", ui.Accent("Hardware Audit:"), ui.Header("Audited Machine Specifications & Virtualization"))
	fmt.Printf("  %s %-15s %s\n", ui.Muted("•"), ui.Label("CPU Cores:"), ui.Value(fmt.Sprintf("%d Cores / vCPUs (%s)", p.CPUCores, runtime.GOARCH)))
	fmt.Printf("  %s %-15s %s\n", ui.Muted("•"), ui.Label("RAM / Memory:"), ui.Value(fmt.Sprintf("%.1f GB Total (%.1f GB Free)", float64(p.TotalRAMMB)/1024.0, float64(p.FreeRAMMB)/1024.0)))
	fmt.Printf("  %s %-15s %s\n", ui.Muted("•"), ui.Label("Disk Storage:"), ui.Value(fmt.Sprintf("%.1f GB Total (%.1f GB Free) • %s [%s]", p.DiskTotalGB, p.DiskFreeGB, storageType, p.FilesystemType)))
	fmt.Printf("  %s %-15s %s\n", ui.Muted("•"), ui.Label("Environment:"), ui.Value(virtDesc))
	fmt.Println()
}

// CalculateBufferPoolMB dynamically allocates MariaDB buffer pool based on RAM.
func CalculateBufferPoolMB(ramMB int64) int {
	var ratio float64
	if ramMB <= 1024 {
		ratio = 0.25 // 25% on low-memory servers (<= 1GB)
	} else if ramMB <= 4096 {
		ratio = 0.30 // 30% on standard servers (1GB-4GB)
	} else if ramMB <= 16384 {
		ratio = 0.40 // 40% on mid-tier servers (4GB-16GB)
	} else {
		ratio = 0.50 // 50% on large enterprise servers (> 16GB)
	}

	mb := int(float64(ramMB) * ratio)
	if mb < 128 {
		mb = 128
	}
	return mb
}

// CalculatePoolInstances determines InnoDB buffer pool instances based on size and CPU cores.
func CalculatePoolInstances(bufferPoolMB int, cpuCores int) int {
	if bufferPoolMB < 1024 {
		return 1
	}
	instances := bufferPoolMB / 1024
	if instances > cpuCores {
		instances = cpuCores
	}
	if instances < 1 {
		instances = 1
	}
	return instances
}

// CalculateLogFileMB bounds InnoDB log file size between 48MB and 512MB (25% of buffer pool).
func CalculateLogFileMB(bufferPoolMB int) int {
	mb := bufferPoolMB / 4
	if mb < 48 {
		return 48
	}
	if mb > 512 {
		return 512
	}
	return mb
}

// CalculateMaxConnections scales maximum database client connections with memory tiers.
func CalculateMaxConnections(ramMB int64) int {
	if ramMB <= 1024 {
		return 30
	} else if ramMB <= 4096 {
		return 50
	} else if ramMB <= 8192 {
		return 80
	}
	return 150
}

// CalculateRedisMaxMemory dynamically allocates Redis RAM cache limits.
func CalculateRedisMaxMemory(ramMB int64) string {
	if ramMB <= 1024 {
		return "64mb"
	} else if ramMB <= 4096 {
		return "128mb"
	} else if ramMB <= 8192 {
		return "256mb"
	}
	return "512mb"
}

// CalculateRecommendedSwapMB determines optimal swap size and checks disk capacity.
func CalculateRecommendedSwapMB(ramMB int64, freeDiskGB float64) (swapMB int, shouldCreate bool, reason string) {
	if freeDiskGB < 2.0 {
		return 0, false, fmt.Sprintf("low disk space (%.1f GB available, minimum 2.0 GB required)", freeDiskGB)
	}

	targetMB := 2048
	if ramMB <= 1024 {
		targetMB = 1024
	} else if ramMB <= 2048 {
		targetMB = 2048
	} else if ramMB <= 8192 {
		targetMB = 2048
	} else {
		targetMB = 4096
	}

	// Never consume more than 25% of remaining free disk space
	maxDiskMB := int(freeDiskGB * 1024 * 0.25)
	if targetMB > maxDiskMB {
		if maxDiskMB >= 512 {
			targetMB = maxDiskMB
		} else {
			return 0, false, fmt.Sprintf("allocating swap would exceed 25%% of free disk space (%.1f GB free)", freeDiskGB)
		}
	}

	return targetMB, true, ""
}

// TuneServer auto-tunes system swap memory and database parameters based on hardware resources.
func TuneServer() error {
	profile := AuditHardwareProfile()
	return TuneServerWithProfile(profile)
}

// TuneServerWithProfile runs the tuning checklist using the audited hardware profile.
func TuneServerWithProfile(profile *SystemHardwareProfile) error {
	ui.PrintInfo("Starting AgilePanel Server Optimization checklist...")
	printHardwareAuditCard(profile)

	// 1. Optimize Swap File & Kernel Sysctl
	ui.PrintStep(1, "Configuring system swap memory and kernel cache swappiness parameters...")
	ui.PrintInfo("Explanatory Note: Adequate swap space prevents Linux out-of-memory (OOM) crashes. Tweaking swappiness and cache pressure keeps the server highly responsive under load.")
	if err := TuneSwapWithProfile(profile); err != nil {
		ui.PrintWarning(fmt.Sprintf("Swap optimization encountered an issue: %v", err))
	}

	// 2. Optimize Database (MariaDB/MySQL)
	ui.PrintStep(2, "Auditing hardware memory resources and auto-tuning MariaDB configuration...")
	poolMB := CalculateBufferPoolMB(profile.TotalRAMMB)
	instances := CalculatePoolInstances(poolMB, profile.CPUCores)
	ui.PrintInfo(fmt.Sprintf("Explanatory Note: Dynamically allocating %dMB to InnoDB buffer pool (%d instances across %d cores) for fast query resolution and resource safety.", poolMB, instances, profile.CPUCores))
	if err := TuneDatabaseWithProfile(profile); err != nil {
		ui.PrintWarning(fmt.Sprintf("Database optimization failed: %v", err))
	}

	// 3. Optimize Redis Socket Permissions
	ui.PrintStep(3, "Configuring Redis Socket permissions and scaling policies...")
	ui.PrintInfo(fmt.Sprintf("Explanatory Note: Using local UNIX socket paths instead of TCP loopbacks cuts down latency. Memory limit dynamically set to %s.", CalculateRedisMaxMemory(profile.TotalRAMMB)))
	if err := TuneRedisWithProfile(profile); err != nil {
		ui.PrintWarning(fmt.Sprintf("Redis optimization failed: %v", err))
	}

	// 4. Setup Default Webserver and Landing Page
	ui.PrintStep(4, "Initializing Caddy default webserver block and security headers...")
	ui.PrintInfo("Explanatory Note: Caddy is configured with strict security headers (no-referrer, X-Frame-Options) and default port 80 catch-all filters.")
	if err := SetupDefaultWebserver(); err != nil {
		ui.PrintWarning(fmt.Sprintf("Default webserver initialization failed: %v", err))
	}

	// 5. Auto-tune PHP-FPM Pools for hosted sites
	ui.PrintStep(5, "Auditing and auto-tuning PHP-FPM worker pools according to available RAM...")
	ui.PrintInfo("Explanatory Note: PHP-FPM pools are scaled using net available RAM after subtracting database and cache footprints, dynamically balanced across total site density.")
	if err := TunePHPPoolsWithProfile(profile); err != nil {
		ui.PrintWarning(fmt.Sprintf("PHP pool tuning encountered an issue: %v", err))
	}

	// 6. Configure systemd user slice resource limits & daemon priorities
	ui.PrintStep(6, "Configuring systemd multi-tenant user slices and daemon priorities...")
	ui.PrintInfo("Explanatory Note: User slices enforce task limits (TasksMax=150) per site tenant, while guaranteeing scheduling priority (CPUWeight=1000) for MariaDB and Caddy.")
	if err := TuneSystemdResourceSlices(); err != nil {
		ui.PrintWarning(fmt.Sprintf("Systemd slice configuration encountered an issue: %v", err))
	}

	// 7. Install Metrics Cron Job
	ui.PrintStep(7, "Installing real-time system metrics logging daemon...")
	ui.PrintInfo("Explanatory Note: A cron job runs every 5 minutes under root to capture performance snapshots, helping you visualize real-time resource trends.")
	if err := installMetricsCron(); err != nil {
		ui.PrintWarning(fmt.Sprintf("Metrics cron job installation failed: %v", err))
	}

	ui.PrintSuccess("Server Performance Tuning Completed Successfully")

	// Trigger telemetry check-in
	statePath := config.GetStatePath()
	if s, err := config.ReadState(statePath); err == nil {
		config.PingAsync(s)
	}

	return nil
}

// TuneSystemdResourceSlices provisions user slice constraints and daemon priorities.
func TuneSystemdResourceSlices() error {
	if runtime.GOOS != "linux" {
		fmt.Println("Systemd (Mock): Configured user slice resource limits and daemon priorities.")
		return nil
	}

	// 1. User slice configuration: limits per system user (wp_*)
	userSliceDir := "/etc/systemd/system/user-.slice.d"
	_ = os.MkdirAll(userSliceDir, 0755)
	sliceConf := `[Slice]
TasksMax=150
CPUWeight=100
MemoryHigh=75%
`
	_ = os.WriteFile(filepath.Join(userSliceDir, "50-agilepanel.conf"), []byte(sliceConf), 0644)

	// 2. High scheduling priority for MariaDB
	mariaDropinDir := "/etc/systemd/system/mariadb.service.d"
	_ = os.MkdirAll(mariaDropinDir, 0755)
	priorityConf := `[Service]
CPUWeight=1000
IOWeight=1000
`
	_ = os.WriteFile(filepath.Join(mariaDropinDir, "50-agilepanel-priority.conf"), []byte(priorityConf), 0644)

	// 3. High scheduling priority for Caddy
	caddyDropinDir := "/etc/systemd/system/caddy.service.d"
	_ = os.MkdirAll(caddyDropinDir, 0755)
	_ = os.WriteFile(filepath.Join(caddyDropinDir, "50-agilepanel-priority.conf"), []byte(priorityConf), 0644)

	// Reload systemd daemons to apply slice and dropin updates
	_ = exec.Command("systemctl", "daemon-reload").Run()
	fmt.Println("Systemd: Applied multi-tenant user slice resource limits and daemon priorities.")
	return nil
}

// TuneSwap checks swap settings and sets up an optimal swap file + sysctl tweaks if swap is missing.
func TuneSwap() error {
	return TuneSwapWithProfile(AuditHardwareProfile())
}

// TuneSwapWithProfile configures swap and kernel parameters based on hardware profile.
func TuneSwapWithProfile(profile *SystemHardwareProfile) error {
	if runtime.GOOS != "linux" {
		fmt.Println("Swap (Mock): Configure swapfile and vm.swappiness=10")
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
		fmt.Printf("Swap: Swap memory is already active (%d MB). Skipping swapfile creation.\n", profile.TotalSwapMB)
	} else if profile.IsContainer {
		fmt.Printf("Swap: Container virtualization detected (%s). Host node kernel manages memory limits; skipping guest swapfile creation.\n", profile.Virtualization)
	} else {
		swapMB, shouldCreate, reason := CalculateRecommendedSwapMB(profile.TotalRAMMB, profile.DiskFreeGB)
		if !shouldCreate {
			ui.PrintWarning(fmt.Sprintf("Skipping swapfile creation: %s.", reason))
		} else {
			fmt.Printf("Swap: No swap space detected. Creating %dMB swap file...\n", swapMB)
			if err := createAndEnableSwapfile(swapMB, profile.FilesystemType); err != nil {
				ui.PrintWarning(fmt.Sprintf("Swap creation encountered an issue: %v", err))
			}
		}
	}

	// Always optimize Sysctl parameters regardless of swap creation outcome
	return TuneSysctlWithProfile(profile)
}

func createAndEnableSwapfile(swapMB int, fsType string) error {
	swapPath := "/swapfile"

	// If Btrfs, ensure CoW is disabled before writing
	if fsType == "btrfs" {
		_ = exec.Command("truncate", "-s", "0", swapPath).Run()
		_ = exec.Command("chattr", "+C", swapPath).Run()
		_ = exec.Command("btrfs", "property", "set", swapPath, "compression", "none").Run()
	}

	// Step 1: Try fallocate first for fast allocation
	fallocateSuccess := false
	fallocCmd := exec.Command("fallocate", "-l", fmt.Sprintf("%dM", swapMB), swapPath)
	if err := fallocCmd.Run(); err == nil {
		_ = exec.Command("chmod", "600", swapPath).Run()
		if mkErr := exec.Command("mkswap", swapPath).Run(); mkErr == nil {
			var swaponErr bytes.Buffer
			swaponCmd := exec.Command("swapon", swapPath)
			swaponCmd.Stderr = &swaponErr
			if err := swaponCmd.Run(); err == nil {
				fallocateSuccess = true
			} else {
				// fallocate left sparse extents (exit status 255 on XFS/Btrfs/ext4)
				fmt.Printf("Swap: fallocate created sparse extents (%s). Retrying with dd zero-fill allocation...\n", strings.TrimSpace(swaponErr.String()))
				_ = exec.Command("swapoff", swapPath).Run()
				_ = os.Remove(swapPath)
			}
		}
	}

	// Step 2: Fallback to dd if fallocate did not result in an active swapfile
	if !fallocateSuccess {
		_ = exec.Command("swapoff", swapPath).Run()
		_ = os.Remove(swapPath)

		if fsType == "btrfs" {
			_ = exec.Command("truncate", "-s", "0", swapPath).Run()
			_ = exec.Command("chattr", "+C", swapPath).Run()
		}

		ddCmd := exec.Command("dd", "if=/dev/zero", fmt.Sprintf("of=%s", swapPath), "bs=1M", fmt.Sprintf("count=%d", swapMB), "status=none")
		var ddErr bytes.Buffer
		ddCmd.Stderr = &ddErr
		if err := ddCmd.Run(); err != nil {
			_ = os.Remove(swapPath)
			return fmt.Errorf("failed to allocate swapfile space with dd: %w (stderr: %s)", err, ddErr.String())
		}

		if err := exec.Command("chmod", "600", swapPath).Run(); err != nil {
			_ = os.Remove(swapPath)
			return fmt.Errorf("failed to chmod swapfile: %w", err)
		}

		var mkswapErr bytes.Buffer
		mkswapCmd := exec.Command("mkswap", swapPath)
		mkswapCmd.Stderr = &mkswapErr
		if err := mkswapCmd.Run(); err != nil {
			_ = os.Remove(swapPath)
			return fmt.Errorf("failed to mkswap swapfile: %w (stderr: %s)", err, mkswapErr.String())
		}

		var swaponErr bytes.Buffer
		swaponCmd := exec.Command("swapon", swapPath)
		swaponCmd.Stderr = &swaponErr
		if err := swaponCmd.Run(); err != nil {
			_ = os.Remove(swapPath)
			return fmt.Errorf("failed to enable swapfile: %w (stderr: %s)", err, strings.TrimSpace(swaponErr.String()))
		}
	}

	// Step 3: Make swap persistent in fstab
	fstabBytes, err := os.ReadFile("/etc/fstab")
	if err == nil && !strings.Contains(string(fstabBytes), swapPath) {
		fstabStr := string(fstabBytes)
		if !strings.HasSuffix(fstabStr, "\n") {
			fstabStr += "\n"
		}
		fstabStr += fmt.Sprintf("%s none swap sw 0 0\n", swapPath)
		_ = os.WriteFile("/etc/fstab", []byte(fstabStr), 0644)
	}

	fmt.Printf("Swap: %dMB Swapfile successfully created and activated.\n", swapMB)
	return nil
}

// TuneSysctlWithProfile applies kernel network and cache parameters.
func TuneSysctlWithProfile(profile *SystemHardwareProfile) error {
	fmt.Println("Swap: Tuning kernel network stack and cache settings...")

	swappiness := "10"
	if profile.TotalRAMMB <= 1500 {
		swappiness = "20"
	}

	cachePressure := "50"
	if !profile.IsSSD {
		cachePressure = "100"
	}

	fileMax := "200000"
	if profile.TotalRAMMB > 4096 {
		fileMax = "1048576"
	}

	sysctlSettings := map[string]string{
		"vm.swappiness":                   swappiness,
		"vm.vfs_cache_pressure":           cachePressure,
		"vm.dirty_background_ratio":       "5",
		"vm.dirty_ratio":                  "10",
		"net.core.default_qdisc":          "fq",
		"net.ipv4.tcp_congestion_control": "bbr",
		"net.core.somaxconn":              "65535",
		"net.ipv4.tcp_max_syn_backlog":    "65535",
		"net.ipv4.tcp_tw_reuse":           "1",
		"net.ipv4.tcp_fin_timeout":        "15",
		"net.core.netdev_max_backlog":      "5000",
		"fs.file-max":                     fileMax,
	}

	var confBuilder strings.Builder
	for k, v := range sysctlSettings {
		_ = exec.Command("sysctl", fmt.Sprintf("%s=%s", k, v)).Run()
		confBuilder.WriteString(fmt.Sprintf("%s=%s\n", k, v))
	}

	sysctlConfPath := "/etc/sysctl.d/99-agilepanel.conf"
	_ = os.WriteFile(sysctlConfPath, []byte(confBuilder.String()), 0644)

	// Set file descriptor limits in limits.d
	limitsDir := "/etc/security/limits.d"
	if _, err := os.Stat(limitsDir); err == nil {
		limitsContent := "* soft nofile 65535\n* hard nofile 65535\nroot soft nofile 65535\nroot hard nofile 65535\n"
		_ = os.WriteFile(filepath.Join(limitsDir, "99-agilepanel.conf"), []byte(limitsContent), 0644)
	}

	return nil
}

// TunePHPPools reconfigures PHP-FPM pools for all sites according to audited hardware.
func TunePHPPools() error {
	return TunePHPPoolsWithProfile(AuditHardwareProfile())
}

// TunePHPPoolsWithProfile auto-tunes PHP-FPM pools for all configured sites based on system profile.
func TunePHPPoolsWithProfile(profile *SystemHardwareProfile) error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		// No state file or uninitialized panel; nothing to tune yet
		return nil
	}

	reloadedVersions := make(map[string]bool)
	count := 0
	siteCount := len(state.Sites)
	if siteCount < 1 {
		siteCount = 1
	}
	for _, site := range state.Sites {
		if site.PHPVersion == "" || site.Type == "html" {
			continue
		}
		if err := WritePHPPoolWithSiteCount(&site, siteCount); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to update pool for %s: %v", site.Domain, err))
		} else {
			reloadedVersions[site.PHPVersion] = true
			count++
		}
	}

	for ver := range reloadedVersions {
		_ = ReloadPHP(ver)
	}

	if count > 0 {
		fmt.Printf("PHP: Successfully auto-tuned %d PHP-FPM pool(s) for hardware profile.\n", count)
	}
	return nil
}

// TuneDatabase configures MySQL/MariaDB tuning parameters.
func TuneDatabase() error {
	return TuneDatabaseWithProfile(AuditHardwareProfile())
}

// TuneDatabaseWithProfile configures MySQL/MariaDB parameters tuned to CPU cores, RAM, and disk drive type.
func TuneDatabaseWithProfile(profile *SystemHardwareProfile) error {
	bufferPoolMB := CalculateBufferPoolMB(profile.TotalRAMMB)
	poolInstances := CalculatePoolInstances(bufferPoolMB, profile.CPUCores)
	logFileMB := CalculateLogFileMB(bufferPoolMB)
	maxConnections := CalculateMaxConnections(profile.TotalRAMMB)

	tableOpenCache := maxConnections * 8
	if tableOpenCache < 400 {
		tableOpenCache = 400
	} else if tableOpenCache > 4000 {
		tableOpenCache = 4000
	}

	ioCapacity := 2000
	ioCapacityMax := 4000
	flushNeighbors := 0
	if !profile.IsSSD {
		ioCapacity = 200
		ioCapacityMax = 400
		flushNeighbors = 1
	}

	ioThreads := profile.CPUCores * 2
	if ioThreads < 4 {
		ioThreads = 4
	} else if ioThreads > 8 {
		ioThreads = 8
	}

	cnfContent := fmt.Sprintf(`# AgilePanel Custom MariaDB Optimizations
# Auto-tuned for: %d CPU cores, %d MB RAM, SSD=%t
[mysqld]
innodb_buffer_pool_size = %dM
innodb_buffer_pool_instances = %d
innodb_log_file_size = %dM
innodb_flush_log_at_trx_commit = 2
innodb_flush_method = O_DIRECT
innodb_io_capacity = %d
innodb_io_capacity_max = %d
innodb_flush_neighbors = %d
innodb_read_io_threads = %d
innodb_write_io_threads = %d
query_cache_type = 0
query_cache_size = 0
key_buffer_size = 32M
thread_cache_size = 8
table_open_cache = %d
tmp_table_size = 32M
max_heap_table_size = 32M
max_connections = %d
max_allowed_packet = 64M
join_buffer_size = 256K
sort_buffer_size = 256K
read_rnd_buffer_size = 512K
`, profile.CPUCores, profile.TotalRAMMB, profile.IsSSD,
		bufferPoolMB, poolInstances, logFileMB,
		ioCapacity, ioCapacityMax, flushNeighbors,
		ioThreads, ioThreads,
		tableOpenCache, maxConnections)

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
			if err := os.WriteFile(filePath, []byte(cnfContent), 0644); err == nil {
				fmt.Printf("DB: Database optimization profile written to %s.\n", filePath)
				written = true
				break
			}
		}
	}

	if !written {
		filePath := "/etc/mysql/conf.d/99-agilepanel-tune.cnf"
		_ = os.MkdirAll(filepath.Dir(filePath), 0755)
		if err := os.WriteFile(filePath, []byte(cnfContent), 0644); err == nil {
			fmt.Printf("DB: Database optimization profile written to fallback path %s.\n", filePath)
		} else {
			return fmt.Errorf("failed to write database tuning file")
		}
	}

	// Trigger restart of MariaDB to apply
	fmt.Println("DB: Restarting MariaDB service to apply changes...")
	restartCmd := exec.Command("systemctl", "restart", "mariadb")
	var restartErr bytes.Buffer
	restartCmd.Stderr = &restartErr
	if err := restartCmd.Run(); err != nil {
		fallbackCmd := exec.Command("systemctl", "restart", "mysql")
		fallbackCmd.Stderr = &restartErr
		if fbErr := fallbackCmd.Run(); fbErr != nil {
			return fmt.Errorf("failed to restart MariaDB/MySQL service: %w (stderr: %s)", fbErr, restartErr.String())
		}
	}
	return nil
}

// TuneRedis configures Redis socket paths and permissions.
func TuneRedis() error {
	return TuneRedisWithProfile(AuditHardwareProfile())
}

// TuneRedisWithProfile configures Redis socket paths and permissions tuned to hardware profile.
func TuneRedisWithProfile(profile *SystemHardwareProfile) error {
	maxMem := CalculateRedisMaxMemory(profile.TotalRAMMB)
	if runtime.GOOS != "linux" {
		fmt.Printf("Redis (Mock): Configure UNIX socket and %s RAM optimizations in redis.conf\n", maxMem)
		return nil
	}

	confPath := "/etc/redis/redis.conf"
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		return nil // Redis not installed or different path
	}

	data, err := os.ReadFile(confPath)
	if err != nil {
		return err
	}

	content := string(data)
	modified := false

	// Enable unixsocket
	if strings.Contains(content, "# unixsocket ") || strings.Contains(content, "#unixsocket ") {
		content = replaceLine(content, "unixsocket ", "unixsocket /var/run/redis/redis-server.sock")
		modified = true
	} else if !strings.Contains(content, "unixsocket /var/run/redis/redis-server.sock") {
		content += "\nunixsocket /var/run/redis/redis-server.sock\n"
		modified = true
	}

	// Enable unixsocketperm 770 (secure permissions for socket group)
	if strings.Contains(content, "unixsocketperm 777") {
		content = strings.ReplaceAll(content, "unixsocketperm 777", "unixsocketperm 770")
		modified = true
	} else if strings.Contains(content, "# unixsocketperm ") || strings.Contains(content, "#unixsocketperm ") {
		content = replaceLine(content, "unixsocketperm ", "unixsocketperm 770")
		modified = true
	} else if !strings.Contains(content, "unixsocketperm 770") {
		content += "\nunixsocketperm 770\n"
		modified = true
	}

	// Dynamic RAM/eviction configurations
	redisConfigs := map[string]string{
		"maxmemory":              maxMem,
		"maxmemory-policy":      "allkeys-lru",
		"save":                   `""`,
		"appendonly":             "no",
		"lazyfree-lazy-eviction": "yes",
		"hz":                     "15",
		"dynamic-hz":             "yes",
	}

	for key, val := range redisConfigs {
		var changed bool
		content, changed = setOrAppendRedisConfig(content, key, val)
		if changed {
			modified = true
		}
	}

	// Multi-tenant Redis security hardening: disable destructive commands on shared socket
	dangerousCommands := []string{"FLUSHALL", "FLUSHDB", "CONFIG", "SHUTDOWN", "DEBUG"}
	for _, cmd := range dangerousCommands {
		directive := fmt.Sprintf(`rename-command %s ""`, cmd)
		if !strings.Contains(content, directive) {
			content += fmt.Sprintf("\n%s\n", directive)
			modified = true
		}
	}

	if modified {
		if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
			return err
		}
		fmt.Printf("Redis: Socket and dynamic %s RAM optimizations applied to redis.conf.\n", maxMem)
		
		// Restart service
		_ = exec.Command("systemctl", "restart", "redis-server").Run()
		_ = exec.Command("systemctl", "restart", "redis").Run()
	}

	return nil
}

func setOrAppendRedisConfig(content string, key string, value string) (string, bool) {
	lines := strings.Split(content, "\n")
	found := false
	newLineStr := fmt.Sprintf("%s %s", key, value)
	
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) > 0 && fields[0] == key {
			if trimmed == newLineStr {
				return content, false
			}
			lines[i] = newLineStr
			found = true
			break
		}
	}
	
	if !found {
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				uncommented := strings.TrimSpace(trimmed[1:])
				fields := strings.Fields(uncommented)
				if len(fields) > 0 && fields[0] == key {
					lines[i] = newLineStr
					found = true
					break
				}
			}
		}
	}
	
	if found {
		return strings.Join(lines, "\n"), true
	}
	
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += newLineStr + "\n"
	return content, true
}

func replaceLine(content string, prefix string, newLine string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			uncommented := strings.TrimSpace(trimmed[1:])
			if strings.HasPrefix(uncommented, prefix) {
				lines[i] = newLine
			}
		}
	}
	return strings.Join(lines, "\n")
}

const welcomeHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Welcome to AgilePanel</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-color: #030712;
            --card-bg: rgba(255, 255, 255, 0.03);
            --card-border: rgba(255, 255, 255, 0.08);
            --text-primary: #f3f4f6;
            --text-secondary: #9ca3af;
            --accent-gradient: linear-gradient(135deg, #a78bfa, #3b82f6, #06b6d4);
            --card-hover-bg: rgba(255, 255, 255, 0.06);
            --card-hover-border: rgba(255, 255, 255, 0.15);
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: 'Outfit', sans-serif;
            background-color: var(--bg-color);
            color: var(--text-primary);
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            justify-content: center;
            align-items: center;
            overflow-x: hidden;
            position: relative;
            padding: 2rem 1rem;
        }

        /* Subtle glowing background blobs */
        body::before, body::after {
            content: '';
            position: absolute;
            width: 400px;
            height: 400px;
            border-radius: 50%;
            background: var(--accent-gradient);
            filter: blur(150px);
            opacity: 0.15;
            z-index: 0;
            pointer-events: none;
        }

        body::before {
            top: -100px;
            left: -100px;
        }

        body::after {
            bottom: -100px;
            right: -100px;
        }

        .container {
            max-width: 900px;
            width: 100%;
            z-index: 1;
            text-align: center;
            animation: fadeIn 1s ease-out;
        }

        header {
            margin-bottom: 3.5rem;
        }

        .logo-container {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            margin-bottom: 1.5rem;
            position: relative;
        }

        .logo-icon {
            font-size: 3rem;
            background: var(--accent-gradient);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            font-weight: 700;
            letter-spacing: -1px;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }

        .logo-badge {
            background: rgba(59, 130, 246, 0.15);
            border: 1px solid rgba(59, 130, 246, 0.3);
            color: #60a5fa;
            font-size: 0.75rem;
            padding: 0.2rem 0.6rem;
            border-radius: 50px;
            margin-left: 0.75rem;
            font-weight: 500;
            letter-spacing: 0.5px;
            text-transform: uppercase;
        }

        h1 {
            font-size: 2.75rem;
            font-weight: 600;
            line-height: 1.2;
            margin-bottom: 1rem;
            background: linear-gradient(to right, #ffffff, #d1d5db);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }

        .subtitle {
            font-size: 1.15rem;
            color: var(--text-secondary);
            max-width: 600px;
            margin: 0 auto;
            font-weight: 300;
            line-height: 1.6;
        }

        /* Glassmorphism main card */
        .main-card {
            background: var(--card-bg);
            backdrop-filter: blur(16px);
            -webkit-backdrop-filter: blur(16px);
            border: 1px solid var(--card-border);
            border-radius: 24px;
            padding: 3rem;
            margin-bottom: 3rem;
            box-shadow: 0 20px 50px rgba(0, 0, 0, 0.3);
            position: relative;
            overflow: hidden;
        }

        .main-card::before {
            content: '';
            position: absolute;
            top: 0;
            left: 0;
            width: 100%;
            height: 4px;
            background: var(--accent-gradient);
        }

        .status-badge {
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
            background: rgba(16, 185, 129, 0.1);
            border: 1px solid rgba(16, 185, 129, 0.2);
            color: #34d399;
            padding: 0.5rem 1rem;
            border-radius: 50px;
            font-weight: 500;
            font-size: 0.9rem;
            margin-bottom: 2rem;
            animation: pulse 2s infinite;
        }

        .status-dot {
            width: 8px;
            height: 8px;
            background-color: #10b981;
            border-radius: 50%;
        }

        /* Cards grid */
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 1.5rem;
            margin-top: 1rem;
        }

        .step-card {
            background: rgba(255, 255, 255, 0.01);
            border: 1px solid var(--card-border);
            border-radius: 16px;
            padding: 2rem 1.5rem;
            text-align: left;
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
            position: relative;
            cursor: default;
        }

        .step-card:hover {
            background: var(--card-hover-bg);
            border-color: var(--card-hover-border);
            transform: translateY(-5px);
            box-shadow: 0 10px 25px rgba(0, 0, 0, 0.2);
        }

        .step-icon {
            width: 48px;
            height: 48px;
            border-radius: 12px;
            background: rgba(255, 255, 255, 0.03);
            display: flex;
            align-items: center;
            justify-content: center;
            margin-bottom: 1.25rem;
            font-size: 1.5rem;
            color: #60a5fa;
            border: 1px solid rgba(255, 255, 255, 0.05);
            transition: all 0.3s ease;
        }

        .step-card:hover .step-icon {
            background: var(--accent-gradient);
            color: #ffffff;
            border-color: transparent;
            box-shadow: 0 0 15px rgba(59, 130, 246, 0.4);
        }

        .step-card h3 {
            font-size: 1.2rem;
            font-weight: 500;
            margin-bottom: 0.5rem;
            color: #ffffff;
        }

        .step-card p {
            font-size: 0.9rem;
            color: var(--text-secondary);
            line-height: 1.5;
            font-weight: 300;
        }

        .code-block {
            background: #090d16;
            border: 1px solid rgba(255, 255, 255, 0.05);
            border-radius: 8px;
            padding: 0.75rem 1rem;
            font-family: 'Courier New', Courier, monospace;
            font-size: 0.85rem;
            color: #38bdf8;
            margin-top: 1rem;
            display: flex;
            justify-content: space-between;
            align-items: center;
            position: relative;
        }

        footer {
            margin-top: auto;
            color: #4b5563;
            font-size: 0.85rem;
            font-weight: 300;
        }

        footer a {
            color: #6b7280;
            text-decoration: none;
            transition: color 0.2s;
        }

        footer a:hover {
            color: #9ca3af;
        }

        /* Animations */
        @keyframes fadeIn {
            from {
                opacity: 0;
                transform: translateY(20px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }

        @keyframes pulse {
            0% {
                box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.4);
            }
            70% {
                box-shadow: 0 0 0 10px rgba(16, 185, 129, 0);
            }
            100% {
                box-shadow: 0 0 0 0 rgba(16, 185, 129, 0);
            }
        }

        @keyframes float {
            0% {
                transform: translateY(0px);
            }
            50% {
                transform: translateY(-10px);
            }
            100% {
                transform: translateY(0px);
            }
        }

        .floating {
            animation: float 6s ease-in-out infinite;
        }

        @media (max-width: 640px) {
            h1 {
                font-size: 2rem;
            }
            .main-card {
                padding: 1.5rem;
            }
            .grid {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div class="logo-container floating">
                <span class="logo-icon">▲ AgilePanel</span>
                <span class="logo-badge">v1.1.8</span>
            </div>
            <h1>Web Stack Successfully Initialized</h1>
            <p class="subtitle">Your High-Performance VPS is fully optimized and ready to serve next-generation web experiences.</p>
        </header>

        <main class="main-card">
            <div class="status-badge">
                <span class="status-dot"></span>
                <span>Server Stack Online & Tuned</span>
            </div>

            <div class="grid">
                <!-- Card 1 -->
                <div class="step-card">
                    <div class="step-icon">✦</div>
                    <h3>Create a Website</h3>
                    <p>Deploy a new lightning-fast WordPress site with Redis cache & full PHP-FPM optimization.</p>
                    <div class="code-block">
                        <span>ap site create example.com --wp</span>
                    </div>
                </div>

                <!-- Card 2 -->
                <div class="step-card">
                    <div class="step-icon">⚙</div>
                    <h3>Monitor Performance</h3>
                    <p>Check active services, real-time memory stats, and disk utilization instantly.</p>
                    <div class="code-block">
                        <span>ap server status</span>
                    </div>
                </div>

                <!-- Card 3 -->
                <div class="step-card">
                    <div class="step-icon">⚡</div>
                    <h3>Tuned to Perfection</h3>
                    <p>Redis sockets, OPcache, MariaDB buffers, and swap parameters are already configured.</p>
                    <div class="code-block">
                        <span>ap server tune</span>
                    </div>
                </div>
            </div>
        </main>

        <footer>
            <p>Powered by <a href="https://github.com/webtechj/agilepanel" target="_blank">AgilePanel</a> • High Performance VPS Hosting</p>
        </footer>
    </div>
</body>
</html>
`

func writeDefaultWelcomePage() error {
	var welcomeDir string
	if runtime.GOOS == "windows" {
		// Use a local path for testing on Windows
		welcomeDir = filepath.Join(os.TempDir(), "agilepanel-default")
	} else {
		welcomeDir = "/var/www/default"
	}

	if err := os.MkdirAll(welcomeDir, 0755); err != nil {
		return fmt.Errorf("failed to create default welcome page directory: %w", err)
	}

	welcomeFile := filepath.Join(welcomeDir, "index.html")
	if err := os.WriteFile(welcomeFile, []byte(welcomeHTML), 0644); err != nil {
		return fmt.Errorf("failed to write index.html to %s: %w", welcomeFile, err)
	}

	fmt.Printf("Default Page: Welcome page written to %s successfully.\n", welcomeFile)
	return nil
}

// SetupDefaultWebserver initializes the default welcome page and Caddy catch-all configuration.
func SetupDefaultWebserver() error {
	// 1. Write the default welcome page
	if err := writeDefaultWelcomePage(); err != nil {
		return fmt.Errorf("failed to write default welcome page: %w", err)
	}

	// 2. Read and lock the state to write Caddy config and reload
	statePath := config.GetStatePath()
	err := config.WithLockedState(statePath, func(s *config.State) error {
		// 3. Write Caddyfile (contains :80 welcome block and any configured sites)
		if err := WriteCaddyfile(s); err != nil {
			return fmt.Errorf("failed to write Caddyfile: %w", err)
		}
		// 4. Reload Caddy
		if err := ReloadCaddy(s); err != nil {
			return fmt.Errorf("failed to reload Caddy: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func installMetricsCron() error {
	if runtime.GOOS != "linux" {
		fmt.Println("Cron (Mock): Write metrics cron job to /etc/cron.d/agilepanel-metrics")
		return nil
	}

	cronPath := "/etc/cron.d/agilepanel-metrics"
	cronContent := "*/5 * * * * root /usr/local/bin/ap server log-metrics >/dev/null 2>&1\n"

	err := os.WriteFile(cronPath, []byte(cronContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write metrics cron job: %w", err)
	}

	fmt.Println("Cron: Metrics recording job installed to /etc/cron.d/agilepanel-metrics.")
	return nil
}
