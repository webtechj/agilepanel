package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogMetricsAndRetention(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")
	
	t.Setenv("AGILEPANEL_TEST_MODE", "true")
	t.Setenv("AGILEPANEL_STATE_PATH", stateFile)

	// Verify LogMetrics writes the file
	err := LogMetrics()
	if err != nil {
		t.Fatalf("LogMetrics failed: %v", err)
	}

	metricsFile := filepath.Join(tempDir, "metrics.json")
	if _, err := os.Stat(metricsFile); os.IsNotExist(err) {
		t.Fatal("metrics.json was not created")
	}

	// Read output file
	data, err := os.ReadFile(metricsFile)
	if err != nil {
		t.Fatalf("failed to read metrics.json: %v", err)
	}

	var samples []MetricSample
	if err := json.Unmarshal(data, &samples); err != nil {
		t.Fatalf("failed to unmarshal metrics JSON: %v", err)
	}

	if len(samples) != 1 {
		t.Errorf("expected 1 sample, got %d", len(samples))
	}

	// Now manually write multiple samples: one recent and one older than 7 days
	now := time.Now()
	testSamples := []MetricSample{
		{
			Timestamp:     now.Add(-8 * 24 * time.Hour), // 8 days old
			CPUPercentage: 10.0,
		},
		{
			Timestamp:     now.Add(-2 * 24 * time.Hour), // 2 days old
			CPUPercentage: 20.0,
		},
	}

	testData, err := json.Marshal(testSamples)
	if err != nil {
		t.Fatalf("failed to marshal test samples: %v", err)
	}

	if err := os.WriteFile(metricsFile, testData, 0644); err != nil {
		t.Fatalf("failed to write test samples: %v", err)
	}

	// Run LogMetrics() again
	err = LogMetrics()
	if err != nil {
		t.Fatalf("LogMetrics failed: %v", err)
	}

	// Read updated file
	data, err = os.ReadFile(metricsFile)
	if err != nil {
		t.Fatalf("failed to read metrics.json: %v", err)
	}

	var updatedSamples []MetricSample
	if err := json.Unmarshal(data, &updatedSamples); err != nil {
		t.Fatalf("failed to unmarshal updated metrics: %v", err)
	}

	// The 8 days old sample must have been purged.
	// The 2 days old sample and the new sample should be kept.
	if len(updatedSamples) != 2 {
		t.Errorf("expected 2 samples after sweep, got %d", len(updatedSamples))
	}

	for _, s := range updatedSamples {
		if s.Timestamp.Before(now.Add(-7 * 24 * time.Hour)) {
			t.Errorf("found sample older than 7 days: %v", s.Timestamp)
		}
	}
}

func TestGetHistoricalMetrics(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "state.json")

	t.Setenv("AGILEPANEL_TEST_MODE", "true")
	t.Setenv("AGILEPANEL_STATE_PATH", stateFile)

	metricsFile := filepath.Join(tempDir, "metrics.json")

	// Write mock historical samples
	now := time.Now()
	mockSamples := []MetricSample{
		{
			Timestamp:     now.Add(-30 * time.Hour), // Older than 24h
			CPUPercentage: 99.0,
			MemoryTotalGB: 10.0,
			MemoryUsedGB:  9.5, // 95%
			SwapTotalGB:   4.0,
			SwapUsedGB:    3.8, // 95%
			TopProcesses: []ProcessStatus{
				{PID: 999, CPU: 95.0, Mem: 10.0, Comm: "heavy-proc"},
			},
		},
		{
			Timestamp:     now.Add(-10 * time.Hour), // 10h ago
			CPUPercentage: 50.0,
			MemoryTotalGB: 8.0,
			MemoryUsedGB:  4.8, // 60%
			SwapTotalGB:   2.0,
			SwapUsedGB:    0.4, // 20%
			TopProcesses: []ProcessStatus{
				{PID: 123, CPU: 40.0, Mem: 10.0, Comm: "test-proc"},
			},
		},
		{
			Timestamp:     now.Add(-5 * time.Hour), // 5h ago
			CPUPercentage: 80.0,
			MemoryTotalGB: 8.0,
			MemoryUsedGB:  3.2, // 40%
			SwapTotalGB:   2.0,
			SwapUsedGB:    0.6, // 30%
			TopProcesses: []ProcessStatus{
				{PID: 123, CPU: 70.0, Mem: 8.0, Comm: "test-proc"},
				{PID: 456, CPU: 30.0, Mem: 5.0, Comm: "other-proc"},
			},
		},
		{
			Timestamp:     now.Add(-2 * time.Hour), // 2h ago
			CPUPercentage: 30.0,
			MemoryTotalGB: 8.0,
			MemoryUsedGB:  6.0, // 75%
			SwapTotalGB:   2.0,
			SwapUsedGB:    0.2, // 10%
			TopProcesses: []ProcessStatus{
				{PID: 456, CPU: 20.0, Mem: 15.0, Comm: "other-proc"},
			},
		},
	}

	testData, err := json.Marshal(mockSamples)
	if err != nil {
		t.Fatalf("failed to marshal mock samples: %v", err)
	}

	if err := os.WriteFile(metricsFile, testData, 0644); err != nil {
		t.Fatalf("failed to write mock metrics file: %v", err)
	}

	// Call GetHistoricalMetrics
	stats, err := GetHistoricalMetrics(24 * time.Hour)
	if err != nil {
		t.Fatalf("GetHistoricalMetrics failed: %v", err)
	}

	if stats.SampleCount != 3 {
		t.Errorf("expected 3 samples, got %d", stats.SampleCount)
	}

	if stats.PeakCPU != 80.0 {
		t.Errorf("expected PeakCPU 80.0, got %.1f", stats.PeakCPU)
	}

	if stats.PeakMemory != 75.0 {
		t.Errorf("expected PeakMemory 75.0, got %.1f", stats.PeakMemory)
	}

	if stats.PeakSwap != 30.0 {
		t.Errorf("expected PeakSwap 30.0, got %.1f", stats.PeakSwap)
	}

	if len(stats.TopProcesses) != 2 {
		t.Fatalf("expected 2 top processes, got %d", len(stats.TopProcesses))
	}

	// Sorted by CPU descending: test-proc (70.0) first, then other-proc (30.0)
	if stats.TopProcesses[0].Comm != "test-proc" {
		t.Errorf("expected first top process to be 'test-proc', got %s", stats.TopProcesses[0].Comm)
	}
	if stats.TopProcesses[0].CPU != 70.0 {
		t.Errorf("expected 'test-proc' peak CPU to be 70.0, got %.1f", stats.TopProcesses[0].CPU)
	}
	if stats.TopProcesses[0].Mem != 10.0 {
		t.Errorf("expected 'test-proc' peak Mem to be 10.0 (from 10h ago), got %.1f", stats.TopProcesses[0].Mem)
	}

	if stats.TopProcesses[1].Comm != "other-proc" {
		t.Errorf("expected second top process to be 'other-proc', got %s", stats.TopProcesses[1].Comm)
	}
	if stats.TopProcesses[1].CPU != 30.0 {
		t.Errorf("expected 'other-proc' peak CPU to be 30.0, got %.1f", stats.TopProcesses[1].CPU)
	}
	if stats.TopProcesses[1].Mem != 15.0 {
		t.Errorf("expected 'other-proc' peak Mem to be 15.0 (from 2h ago), got %.1f", stats.TopProcesses[1].Mem)
	}
}
