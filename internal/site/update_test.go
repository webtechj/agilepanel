package site

import (
	"os"
	"path/filepath"
	"testing"

	"agilepanel/internal/config"
)

func TestSiteUpdatePHPAndMemory(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")
	os.Setenv("AGILEPANEL_STATE_PATH", statePath)
	os.Setenv("AGILEPANEL_TEST_MODE", "true")
	defer os.Unsetenv("AGILEPANEL_STATE_PATH")
	defer os.Unsetenv("AGILEPANEL_TEST_MODE")

	// 1. Create a site running PHP 8.2 with standard 128M
	err := Create("update-test.com", "8.2", "wp", "default", "", "", "", "", "", "", "128M")
	if err != nil {
		t.Fatalf("failed to create site: %v", err)
	}

	state, err := config.ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}
	if state.Sites[0].PHPVersion != "8.2" {
		t.Errorf("expected initial PHP version 8.2, got %s", state.Sites[0].PHPVersion)
	}
	if state.Sites[0].MemoryLimit != "128M" {
		t.Errorf("expected initial MemoryLimit 128M, got %s", state.Sites[0].MemoryLimit)
	}

	// 2. Update site to PHP 8.3 and 256M memory (heavy plugins)
	err = Update("update-test.com", "8.3", "256M", true)
	if err != nil {
		t.Fatalf("failed to update site: %v", err)
	}

	state, err = config.ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to read state after update: %v", err)
	}
	if state.Sites[0].PHPVersion != "8.3" {
		t.Errorf("expected updated PHP version 8.3, got %s", state.Sites[0].PHPVersion)
	}
	if state.Sites[0].MemoryLimit != "256M" {
		t.Errorf("expected updated MemoryLimit 256M, got %s", state.Sites[0].MemoryLimit)
	}
	if state.Sites[0].MaxExecutionTime != 300 {
		t.Errorf("expected MaxExecutionTime 300, got %d", state.Sites[0].MaxExecutionTime)
	}
	if state.Sites[0].MaxInputVars != 5000 {
		t.Errorf("expected MaxInputVars 5000, got %d", state.Sites[0].MaxInputVars)
	}
}
