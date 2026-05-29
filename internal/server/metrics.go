package server

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"agilepanel/internal/config"
)

type MetricSample struct {
	Timestamp      time.Time       `json:"timestamp"`
	CPUPercentage  float64         `json:"cpu_percentage"`
	Load1m         float64         `json:"load_1m"`
	Load5m         float64         `json:"load_5m"`
	Load15m        float64         `json:"load_15m"`
	MemoryTotalGB  float64         `json:"memory_total_gb"`
	MemoryUsedGB   float64         `json:"memory_used_gb"`
	SwapTotalGB    float64         `json:"swap_total_gb"`
	SwapUsedGB     float64         `json:"swap_used_gb"`
	DiskTotalGB    float64         `json:"disk_total_gb"`
	DiskUsedGB     float64         `json:"disk_used_gb"`
	TCPConnections int             `json:"tcp_connections"`
	TopProcesses   []ProcessStatus `json:"top_processes"`
}

type HistoricalStats struct {
	SampleCount  int
	PeakCPU      float64
	PeakMemory   float64
	PeakSwap     float64
	TopProcesses []ProcessStatus
}

func getMetricsPath() string {
	if runtime.GOOS != "linux" || os.Getenv("AGILEPANEL_TEST_MODE") == "true" {
		stateDir := filepath.Dir(config.GetStatePath())
		return filepath.Join(stateDir, "metrics.json")
	}
	return "/var/lib/agilepanel/metrics.json"
}

// LogMetrics collects system resource stats, appends to the log, and retains last 7 days of entries.
func LogMetrics() error {
	path := getMetricsPath()

	// 1. Read existing metrics if file exists
	var samples []MetricSample
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err == nil {
			_ = json.Unmarshal(data, &samples)
		}
	}

	// 2. Collect current stats
	sample := MetricSample{
		Timestamp: time.Now(),
	}

	// CPU Percentage
	cpuPct, err := getCPUPercentage()
	if err == nil {
		sample.CPUPercentage = cpuPct
	}

	// Load average
	m1, m5, m15, err := readLoadAvg()
	if err == nil {
		sample.Load1m = m1
		sample.Load5m = m5
		sample.Load15m = m15
	}

	// Memory and Swap
	totalRAM, freeRAM, totalSwap, freeSwap, err := readLinuxMemoryAndSwap()
	if err == nil {
		sample.MemoryTotalGB = float64(totalRAM) / (1024 * 1024)
		sample.MemoryUsedGB = sample.MemoryTotalGB - (float64(freeRAM) / (1024 * 1024))
		sample.SwapTotalGB = float64(totalSwap) / (1024 * 1024)
		sample.SwapUsedGB = sample.SwapTotalGB - (float64(freeSwap) / (1024 * 1024))
	}

	// Disk
	totalDisk, usedDisk, _, err := readDiskUsage()
	if err == nil {
		sample.DiskTotalGB = float64(totalDisk) / (1024 * 1024 * 1024)
		sample.DiskUsedGB = float64(usedDisk) / (1024 * 1024 * 1024)
	}

	// TCP connections
	tcpConns, err := getTCPConnectionsCount()
	if err == nil {
		sample.TCPConnections = tcpConns
	}

	// Top processes (CPU and memory merged)
	topCPU, err := getTopProcesses("cpu")
	var topMem []ProcessStatus
	if err == nil {
		topMem, _ = getTopProcesses("mem")
	}

	seenPIDs := make(map[int]bool)
	var mergedTop []ProcessStatus
	for _, p := range topCPU {
		if !seenPIDs[p.PID] {
			seenPIDs[p.PID] = true
			mergedTop = append(mergedTop, p)
		}
	}
	for _, p := range topMem {
		if !seenPIDs[p.PID] {
			seenPIDs[p.PID] = true
			mergedTop = append(mergedTop, p)
		}
	}
	sample.TopProcesses = mergedTop

	// 3. Append and sweep old samples (> 7 days)
	samples = append(samples, sample)
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	var retained []MetricSample
	for _, s := range samples {
		if s.Timestamp.After(cutoff) {
			retained = append(retained, s)
		}
	}

	// 4. Atomic write back to JSON
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(retained, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

// GetHistoricalMetrics analyzes samples within the given duration and returns peak utilization.
func GetHistoricalMetrics(duration time.Duration) (*HistoricalStats, error) {
	path := getMetricsPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &HistoricalStats{SampleCount: 0}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var samples []MetricSample
	if err := json.Unmarshal(data, &samples); err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-duration)
	var filtered []MetricSample
	for _, s := range samples {
		if s.Timestamp.After(cutoff) {
			filtered = append(filtered, s)
		}
	}

	if len(filtered) == 0 {
		return &HistoricalStats{SampleCount: 0}, nil
	}

	var peakCPU float64
	var peakMemory float64
	var peakSwap float64

	// Track processes by name to find peak CPU/memory seen for each command
	procPeaks := make(map[string]*ProcessStatus)

	for _, s := range filtered {
		if s.CPUPercentage > peakCPU {
			peakCPU = s.CPUPercentage
		}
		var memPct float64
		if s.MemoryTotalGB > 0 {
			memPct = (s.MemoryUsedGB / s.MemoryTotalGB) * 100
		}
		if memPct > peakMemory {
			peakMemory = memPct
		}
		var swapPct float64
		if s.SwapTotalGB > 0 {
			swapPct = (s.SwapUsedGB / s.SwapTotalGB) * 100
		}
		if swapPct > peakSwap {
			peakSwap = swapPct
		}

		for _, p := range s.TopProcesses {
			if existing, ok := procPeaks[p.Comm]; ok {
				existing.CPU = math.Max(existing.CPU, p.CPU)
				existing.Mem = math.Max(existing.Mem, p.Mem)
			} else {
				procPeaks[p.Comm] = &ProcessStatus{
					PID:  p.PID,
					CPU:  p.CPU,
					Mem:  p.Mem,
					Comm: p.Comm,
				}
			}
		}
	}

	// Convert map to slice and sort by Peak CPU descending
	var sortedProcs []ProcessStatus
	for _, p := range procPeaks {
		sortedProcs = append(sortedProcs, *p)
	}

	sort.Slice(sortedProcs, func(i, j int) bool {
		return sortedProcs[i].CPU > sortedProcs[j].CPU
	})

	// Capped to top 5
	if len(sortedProcs) > 5 {
		sortedProcs = sortedProcs[:5]
	}

	return &HistoricalStats{
		SampleCount:  len(filtered),
		PeakCPU:      peakCPU,
		PeakMemory:   peakMemory,
		PeakSwap:     peakSwap,
		TopProcesses: sortedProcs,
	}, nil
}
