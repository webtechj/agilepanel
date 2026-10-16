package server

import (
	"testing"
)

func TestAuditHardwareProfile(t *testing.T) {
	profile := AuditHardwareProfile()
	if profile == nil {
		t.Fatal("expected non-nil profile from AuditHardwareProfile")
	}

	if profile.CPUCores < 1 {
		t.Errorf("expected CPUCores >= 1, got %d", profile.CPUCores)
	}

	if profile.TotalRAMMB <= 0 {
		t.Errorf("expected TotalRAMMB > 0, got %d", profile.TotalRAMMB)
	}

	if profile.DiskTotalGB < 0 {
		t.Errorf("expected DiskTotalGB >= 0, got %f", profile.DiskTotalGB)
	}
}

func TestCalculateBufferPoolMB(t *testing.T) {
	tests := []struct {
		ramMB    int64
		expected int
	}{
		{ramMB: 512, expected: 128},    // Bounded by minimum 128M
		{ramMB: 1024, expected: 256},   // 25% of 1024
		{ramMB: 2048, expected: 614},   // 30% of 2048
		{ramMB: 4096, expected: 1228},  // 30% of 4096
		{ramMB: 8192, expected: 3276},  // 40% of 8192
		{ramMB: 32768, expected: 16384},// 50% of 32768
	}

	for _, tt := range tests {
		actual := CalculateBufferPoolMB(tt.ramMB)
		if actual != tt.expected {
			t.Errorf("CalculateBufferPoolMB(%d): expected %d, got %d", tt.ramMB, tt.expected, actual)
		}
	}
}

func TestCalculatePoolInstances(t *testing.T) {
	tests := []struct {
		poolMB   int
		cores    int
		expected int
	}{
		{poolMB: 512, cores: 4, expected: 1},
		{poolMB: 2048, cores: 4, expected: 2},
		{poolMB: 8192, cores: 4, expected: 4}, // capped at cores
		{poolMB: 8192, cores: 16, expected: 8},
	}

	for _, tt := range tests {
		actual := CalculatePoolInstances(tt.poolMB, tt.cores)
		if actual != tt.expected {
			t.Errorf("CalculatePoolInstances(%d, %d): expected %d, got %d", tt.poolMB, tt.cores, tt.expected, actual)
		}
	}
}

func TestCalculateLogFileMB(t *testing.T) {
	if actual := CalculateLogFileMB(128); actual != 48 {
		t.Errorf("expected minimum 48MB for 128MB buffer pool, got %d", actual)
	}
	if actual := CalculateLogFileMB(1024); actual != 256 {
		t.Errorf("expected 256MB for 1024MB buffer pool, got %d", actual)
	}
	if actual := CalculateLogFileMB(4096); actual != 512 { // capped at 512
		t.Errorf("expected 512MB cap for 4096MB buffer pool, got %d", actual)
	}
}

func TestCalculateMaxConnections(t *testing.T) {
	if CalculateMaxConnections(1024) != 30 {
		t.Errorf("expected 30 max connections for 1GB RAM, got %d", CalculateMaxConnections(1024))
	}
	if CalculateMaxConnections(2048) != 50 {
		t.Errorf("expected 50 max connections for 2GB RAM, got %d", CalculateMaxConnections(2048))
	}
	if CalculateMaxConnections(8192) != 80 {
		t.Errorf("expected 80 max connections for 8GB RAM, got %d", CalculateMaxConnections(8192))
	}
	if CalculateMaxConnections(16384) != 150 {
		t.Errorf("expected 150 max connections for 16GB RAM, got %d", CalculateMaxConnections(16384))
	}
}

func TestCalculateRedisMaxMemory(t *testing.T) {
	if CalculateRedisMaxMemory(1024) != "64mb" {
		t.Errorf("expected 64mb for 1GB RAM, got %s", CalculateRedisMaxMemory(1024))
	}
	if CalculateRedisMaxMemory(2048) != "128mb" {
		t.Errorf("expected 128mb for 2GB RAM, got %s", CalculateRedisMaxMemory(2048))
	}
	if CalculateRedisMaxMemory(8192) != "256mb" {
		t.Errorf("expected 256mb for 8GB RAM, got %s", CalculateRedisMaxMemory(8192))
	}
	if CalculateRedisMaxMemory(16384) != "512mb" {
		t.Errorf("expected 512mb for 16GB RAM, got %s", CalculateRedisMaxMemory(16384))
	}
}

func TestCalculateRecommendedSwapMB(t *testing.T) {
	// Insufficient disk space (< 2.0 GB free)
	_, shouldCreate, _ := CalculateRecommendedSwapMB(2048, 1.5)
	if shouldCreate {
		t.Error("expected shouldCreate=false for 1.5GB free disk")
	}

	// Normal disk space
	swapMB, shouldCreate, _ := CalculateRecommendedSwapMB(1024, 20.0)
	if !shouldCreate || swapMB != 1024 {
		t.Errorf("expected 1024MB swap for 1GB RAM, got %d (shouldCreate=%t)", swapMB, shouldCreate)
	}

	swapMB, shouldCreate, _ = CalculateRecommendedSwapMB(2048, 20.0)
	if !shouldCreate || swapMB != 2048 {
		t.Errorf("expected 2048MB swap for 2GB RAM, got %d (shouldCreate=%t)", swapMB, shouldCreate)
	}

	swapMB, shouldCreate, _ = CalculateRecommendedSwapMB(16384, 50.0)
	if !shouldCreate || swapMB != 4096 {
		t.Errorf("expected 4096MB swap for 16GB RAM, got %d (shouldCreate=%t)", swapMB, shouldCreate)
	}

	// Disk constrained (3GB free disk -> 25% is 768MB)
	swapMB, shouldCreate, _ = CalculateRecommendedSwapMB(2048, 3.0)
	if !shouldCreate || swapMB != 768 {
		t.Errorf("expected 768MB constrained swap for 3GB free disk, got %d (shouldCreate=%t)", swapMB, shouldCreate)
	}
}

func TestTuneServerMock(t *testing.T) {
	// Verify mock execution runs smoothly without error
	err := TuneServer()
	if err != nil {
		t.Fatalf("expected nil error on mock environment, got %v", err)
	}
}
