package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

type GlobalConfig struct {
	DefaultPHPVersion    string   `json:"default_php_version"`
	SupportedPHPVersions []string `json:"supported_php_versions"`
	CaddyPath            string   `json:"caddy_path"`
	CaddyConfigPath      string   `json:"caddy_config_path"`
	RedisSocketPath      string   `json:"redis_socket_path"`
	AdminUser            string   `json:"admin_user"`
	AdminPasswordHash    string   `json:"admin_password_hash"`
	AdminName            string   `json:"admin_name,omitempty"`
	AdminEmail           string   `json:"admin_email,omitempty"`
}

type SiteConfig struct {
	Domain       string `json:"domain"`
	PHPVersion   string `json:"php_version"`
	PublicDir    string `json:"public_dir"`
	DatabaseName string `json:"database_name"`
	DatabaseUser string `json:"db_user"`
	DatabasePass string `json:"db_pass,omitempty"`
	SystemUser   string `json:"system_user"`
	IsLocked     bool   `json:"is_locked"`
	WPAdminUser  string `json:"wp_admin_user,omitempty"`
	WPAdminEmail string `json:"wp_admin_email,omitempty"`
}

type State struct {
	Global GlobalConfig `json:"global"`
	Sites  []SiteConfig `json:"sites"`
}

// GetStatePath retrieves the state JSON path, allowing environment override.
func GetStatePath() string {
	if val := os.Getenv("AGILEPANEL_STATE_PATH"); val != "" {
		return val
	}
	return "/etc/agilepanel/state.json"
}

// DefaultState returns a populated State struct with baseline values.
func DefaultState() *State {
	return &State{
		Global: GlobalConfig{
			DefaultPHPVersion:    "8.3",
			SupportedPHPVersions: []string{"8.1", "8.2", "8.3"},
			CaddyPath:            "/usr/sbin/caddy",
			CaddyConfigPath:      "/etc/caddy/Caddyfile",
			RedisSocketPath:      "/var/run/redis/redis-server.sock",
			AdminUser:            "",
			AdminPasswordHash:    "",
			AdminName:            "",
			AdminEmail:           "",
		},
		Sites: []SiteConfig{},
	}
}

// WithLockedState locks the state lockfile, reads state, invokes fn, writes modified state, and unlocks.
func WithLockedState(statePath string, fn func(*State) error) error {
	lockPath := statePath + ".lock"
	
	// Ensure parent directory exists
	dir := filepath.Dir(statePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	fileLock := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	locked, err := fileLock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire state lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("could not acquire state lock: timeout")
	}
	defer func() {
		_ = fileLock.Unlock()
	}()

	state, err := readState(statePath)
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	if err := fn(state); err != nil {
		return err
	}

	if err := writeState(statePath, state); err != nil {
		return fmt.Errorf("write error: %w", err)
	}

	return nil
}

// ReadState reads the state with a shared read-lock.
func ReadState(statePath string) (*State, error) {
	lockPath := statePath + ".lock"
	
	// Ensure parent directory exists
	dir := filepath.Dir(statePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	fileLock := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	locked, err := fileLock.TryRLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire read state lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("could not acquire read state lock: timeout")
	}
	defer func() {
		_ = fileLock.Unlock()
	}()

	return readState(statePath)
}

func readState(statePath string) (*State, error) {
	file, err := os.OpenFile(statePath, os.O_RDWR|os.O_CREATE, 0660)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Get file size
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if info.Size() == 0 {
		// State file is empty or new, initialize it
		state := DefaultState()
		data, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return nil, err
		}
		if _, err := file.Write(data); err != nil {
			return nil, err
		}
		return state, nil
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

func writeState(statePath string, state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	// Write to temporary file first, then rename (atomic swap)
	tmpPath := statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0660); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, statePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}
