package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agilepanel/internal/config"
)

func TestValidateDomain(t *testing.T) {
	validDomains := []string{
		"example.com",
		"sub.example.co.uk",
		"my-domain.io",
		"wordpress123.org",
	}

	invalidDomains := []string{
		"example",
		"http://example.com",
		"example.com/path",
		"invalid_domain.com",
		"example..com",
	}

	for _, d := range validDomains {
		if err := ValidateDomain(d); err != nil {
			t.Errorf("expected domain %s to be valid, got error: %v", d, err)
		}
	}

	for _, d := range invalidDomains {
		if err := ValidateDomain(d); err == nil {
			t.Errorf("expected domain %s to be invalid, but got no error", d)
		}
	}
}

func TestSanitizeUser(t *testing.T) {
	tests := []struct {
		domain   string
		expected string
	}{
		{"example.com", "wp_example_com"},
		{"my-awesome-domain.co.uk", "wp_my_awesome_domain_co_uk"},
		{"extremelylongdomainnameforwordpressprojecttesting.org", "wp_extremelylongdomainnameforw"},
	}

	for _, tc := range tests {
		actual := SanitizeUser(tc.domain)
		if actual != tc.expected {
			t.Errorf("for domain %s, expected sanitized user %s, got %s", tc.domain, tc.expected, actual)
		}
	}
}

func TestSiteOrchestration(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")
	os.Setenv("AGILEPANEL_STATE_PATH", statePath)
	defer os.Unsetenv("AGILEPANEL_STATE_PATH")

	// 1. Create site
	err := Create("test.com", "8.3", true)
	if err != nil {
		t.Fatalf("failed to create site: %v", err)
	}

	// Verify state
	state, err := config.ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}
	if len(state.Sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(state.Sites))
	}
	site := state.Sites[0]
	if site.Domain != "test.com" {
		t.Errorf("expected domain test.com, got %s", site.Domain)
	}
	if site.PHPVersion != "8.3" {
		t.Errorf("expected php version 8.3, got %s", site.PHPVersion)
	}
	if site.SystemUser != "wp_test_com" {
		t.Errorf("expected system user wp_test_com, got %s", site.SystemUser)
	}

	// Verify database naming and user prefixing
	if !strings.HasPrefix(site.DatabaseName, "db_") || !strings.Contains(site.DatabaseName, "_test_com") {
		t.Errorf("unexpected database name format: %s", site.DatabaseName)
	}
	if !strings.HasPrefix(site.DatabaseUser, "usr_") || !strings.Contains(site.DatabaseUser, "_test_") {
		t.Errorf("unexpected database user format: %s", site.DatabaseUser)
	}
	if len(site.DatabaseUser) != 16 {
		t.Errorf("expected database user length to be truncated to 16, got %d (%s)", len(site.DatabaseUser), site.DatabaseUser)
	}

	// 2. Duplicate detection
	err = Create("TEST.com", "8.3", false)
	if err == nil {
		t.Error("expected duplicate error but got nil")
	}

	// 3. Invalid PHP version
	err = Create("another.com", "7.4", false)
	if err == nil {
		t.Error("expected invalid PHP version error but got nil")
	}

	// 4. Lock site
	err = Lock("test.com")
	if err != nil {
		t.Fatalf("failed to lock site: %v", err)
	}

	state, _ = config.ReadState(statePath)
	if !state.Sites[0].IsLocked {
		t.Error("expected site to be locked")
	}

	// 5. Unlock site
	err = Unlock("test.com")
	if err != nil {
		t.Fatalf("failed to unlock site: %v", err)
	}

	state, _ = config.ReadState(statePath)
	if state.Sites[0].IsLocked {
		t.Error("expected site to be unlocked")
	}

	// 5.1 CacheClean
	err = CacheClean("test.com", true, true, true, true)
	if err != nil {
		t.Fatalf("failed to clean cache: %v", err)
	}

	// 5.2 SSLRenew
	err = SSLRenew("test.com")
	if err != nil {
		t.Fatalf("failed to renew ssl: %v", err)
	}

	// 5.3 Reinstall
	err = Reinstall("test.com")
	if err != nil {
		t.Fatalf("failed to reinstall: %v", err)
	}

	// 5.4 FixPermissions
	err = FixPermissions("test.com")
	if err != nil {
		t.Fatalf("failed to fix permissions: %v", err)
	}

	// 5.5 BackupDB
	err = BackupDB("test.com")
	if err != nil {
		t.Fatalf("failed to backup db: %v", err)
	}

	// 5.6 Test List, Info, and Edit in test mode
	os.Setenv("AGILEPANEL_TEST_MODE", "true")
	err = List()
	if err != nil {
		t.Fatalf("failed to list sites: %v", err)
	}
	err = Info("test.com")
	if err != nil {
		t.Fatalf("failed to show site info: %v", err)
	}
	err = Edit("test.com")
	if err != nil {
		t.Fatalf("failed to edit site: %v", err)
	}
	err = Sync()
	if err != nil {
		t.Fatalf("failed to sync: %v", err)
	}
	os.Unsetenv("AGILEPANEL_TEST_MODE")

	// 6. Delete site
	err = Delete("test.com")
	if err != nil {
		t.Fatalf("failed to delete site: %v", err)
	}

	state, _ = config.ReadState(statePath)
	if len(state.Sites) != 0 {
		t.Errorf("expected 0 sites after deletion, got %d", len(state.Sites))
	}
}
