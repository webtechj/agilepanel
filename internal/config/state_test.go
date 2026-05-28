package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDefaultState(t *testing.T) {
	state := DefaultState()
	if state.Global.DefaultPHPVersion != "8.3" {
		t.Errorf("expected default PHP version to be 8.3, got %s", state.Global.DefaultPHPVersion)
	}
	if len(state.Global.SupportedPHPVersions) != 3 {
		t.Errorf("expected 3 supported PHP versions, got %d", len(state.Global.SupportedPHPVersions))
	}
	if len(state.Sites) != 0 {
		t.Errorf("expected 0 initial sites, got %d", len(state.Sites))
	}
}

func TestReadStateCreatesDefault(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")

	// Read state when file does not exist
	state, err := ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}

	// Verify defaults
	if state.Global.DefaultPHPVersion != "8.3" {
		t.Errorf("expected default PHP version to be 8.3, got %s", state.Global.DefaultPHPVersion)
	}

	// Verify file was written
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Errorf("expected state file to be created at %s, but it does not exist", statePath)
	}
}

func TestWithLockedState(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")

	// Apply an update
	err := WithLockedState(statePath, func(s *State) error {
		s.Sites = append(s.Sites, SiteConfig{
			Domain:       "testsite.com",
			PHPVersion:   "8.3",
			PublicDir:    "/var/www/testsite.com/public",
			DatabaseName: "wp_testsite_com",
			DatabaseUser: "wp_user",
			SystemUser:   "wp_testsite_com",
			IsLocked:     false,
		})
		return nil
	})
	if err != nil {
		t.Fatalf("failed to lock and update state: %v", err)
	}

	// Read and verify
	state, err := ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}

	if len(state.Sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(state.Sites))
	}
	if state.Sites[0].Domain != "testsite.com" {
		t.Errorf("expected site domain testsite.com, got %s", state.Sites[0].Domain)
	}
}

func TestConcurrentStateLocking(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")

	// Initialize state
	_, err := ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to init state: %v", err)
	}

	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)

	// Launch multiple concurrent writers to verify safety
	for i := 0; i < workers; i++ {
		go func(workerID int) {
			defer wg.Done()
			err := WithLockedState(statePath, func(s *State) error {
				s.Sites = append(s.Sites, SiteConfig{
					Domain:     "worker.com",
					PHPVersion: "8.3",
				})
				return nil
			})
			if err != nil {
				t.Errorf("worker %d failed to update state: %v", workerID, err)
			}
		}(i)
	}

	wg.Wait()

	// Read and verify final count
	state, err := ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}

	if len(state.Sites) != workers {
		t.Errorf("expected %d sites after concurrent updates, got %d", workers, len(state.Sites))
	}
}
