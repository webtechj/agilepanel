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
	os.Setenv("AGILEPANEL_TEST_MODE", "true")
	defer os.Unsetenv("AGILEPANEL_STATE_PATH")
	defer os.Unsetenv("AGILEPANEL_TEST_MODE")

	// 1. Create site
	err := Create("test.com", "8.3", "wp", "default", "", "")
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
	err = Create("TEST.com", "8.3", "php", "default", "", "")
	if err == nil {
		t.Error("expected duplicate error but got nil")
	}

	// 3. Invalid PHP version
	err = Create("another.com", "7.4", "php", "default", "", "")
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
	// (AGILEPANEL_TEST_MODE is already set at the top of this test)
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

func TestSyncImport(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create mock webroot
	webRoot := filepath.Join(tempDir, "var", "www")
	err := os.MkdirAll(webRoot, 0755)
	if err != nil {
		t.Fatalf("failed to create mock webroot: %v", err)
	}

	// Create mock site directory
	domain := "mocksite.com"
	siteDir := filepath.Join(webRoot, domain, "htdocs")
	err = os.MkdirAll(siteDir, 0755)
	if err != nil {
		t.Fatalf("failed to create site htdocs dir: %v", err)
	}

	// Create mock wp-config.php
	wpConfigContent := `<?php
define( 'DB_NAME', 'db_mock_db' );
define( 'DB_USER', 'usr_mock_user' );
define( 'DB_PASSWORD', 'pass_mock_secret' );
`
	err = os.WriteFile(filepath.Join(siteDir, "wp-config.php"), []byte(wpConfigContent), 0644)
	if err != nil {
		t.Fatalf("failed to write wp-config.php: %v", err)
	}

	// Setup state path
	statePath := filepath.Join(tempDir, "state.json")
	os.Setenv("AGILEPANEL_STATE_PATH", statePath)
	os.Setenv("AGILEPANEL_WEBROOT", webRoot)
	os.Setenv("AGILEPANEL_TEST_MODE", "true")
	defer os.Unsetenv("AGILEPANEL_STATE_PATH")
	defer os.Unsetenv("AGILEPANEL_WEBROOT")
	defer os.Unsetenv("AGILEPANEL_TEST_MODE")

	// Initialize state
	_, err = config.ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to init state: %v", err)
	}

	// Run Sync
	err = Sync()
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Verify imported site is in state
	state, err := config.ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}

	found := false
	for _, site := range state.Sites {
		if site.Domain == domain {
			found = true
			if site.DatabaseName != "db_mock_db" {
				t.Errorf("expected DB Name 'db_mock_db', got '%s'", site.DatabaseName)
			}
			if site.DatabaseUser != "usr_mock_user" {
				t.Errorf("expected DB User 'usr_mock_user', got '%s'", site.DatabaseUser)
			}
			if site.DatabasePass != "pass_mock_secret" {
				t.Errorf("expected DB Pass 'pass_mock_secret', got '%s'", site.DatabasePass)
			}
		}
	}

	if !found {
		t.Errorf("mocksite.com was not imported during sync")
	}
}

func TestLaravelAndHTMLSiteOrchestration(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")
	os.Setenv("AGILEPANEL_STATE_PATH", statePath)
	os.Setenv("AGILEPANEL_TEST_MODE", "true")
	defer os.Unsetenv("AGILEPANEL_STATE_PATH")
	defer os.Unsetenv("AGILEPANEL_TEST_MODE")

	// Initialize state
	_, _ = config.ReadState(statePath)

	// 1. Create static HTML site
	err := Create("static-html.com", "8.3", "html", "default", "", "")
	if err != nil {
		t.Fatalf("failed to create static html site: %v", err)
	}

	state, _ := config.ReadState(statePath)
	if len(state.Sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(state.Sites))
	}
	htmlSite := state.Sites[0]
	if htmlSite.Type != "html" {
		t.Errorf("expected type html, got %s", htmlSite.Type)
	}
	if htmlSite.DatabaseName != "" {
		t.Errorf("expected empty database name for html site, got %s", htmlSite.DatabaseName)
	}

	// 2. Create Laravel site
	err = Create("my-laravel.com", "8.3", "laravel", "default", "", "")
	if err != nil {
		t.Fatalf("failed to create laravel site: %v", err)
	}

	state, _ = config.ReadState(statePath)
	if len(state.Sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(state.Sites))
	}
	laravelSite := state.Sites[1]
	if laravelSite.Type != "laravel" {
		t.Errorf("expected type laravel, got %s", laravelSite.Type)
	}
	if !strings.HasSuffix(laravelSite.PublicDir, "public") {
		t.Errorf("expected public dir to end with public, got %s", laravelSite.PublicDir)
	}
	if laravelSite.DatabaseName == "" {
		t.Error("expected database name for laravel site, got empty")
	}

	// 3. Test backup of HTML site (skips db, only zips htdocs)
	err = Backup("static-html.com")
	if err != nil {
		t.Fatalf("failed to backup html site: %v", err)
	}

	// 4. Test backup of Laravel site (runs mysqldump fallback mock, zips files)
	err = Backup("my-laravel.com")
	if err != nil {
		t.Fatalf("failed to backup laravel site: %v", err)
	}

	// 5. Reinstall Laravel site
	err = Reinstall("my-laravel.com")
	if err != nil {
		t.Fatalf("failed to reinstall laravel site: %v", err)
	}

	// 6. Test static HTML site with explicit database
	err = Create("html-with-db.com", "8.3", "html", "true", "", "")
	if err != nil {
		t.Fatalf("failed to create static html site with db: %v", err)
	}

	state, _ = config.ReadState(statePath)
	var htmlWithDBSite config.SiteConfig
	for _, s := range state.Sites {
		if s.Domain == "html-with-db.com" {
			htmlWithDBSite = s
			break
		}
	}
	if htmlWithDBSite.DatabaseName == "" {
		t.Error("expected database name for html site created with db=true, got empty")
	}

	// 7. Test static HTML site with explicit no database
	err = Create("html-no-db.com", "8.3", "html", "false", "", "")
	if err != nil {
		t.Fatalf("failed to create static html site with no db: %v", err)
	}

	state, _ = config.ReadState(statePath)
	var htmlNoDBSite config.SiteConfig
	for _, s := range state.Sites {
		if s.Domain == "html-no-db.com" {
			htmlNoDBSite = s
			break
		}
	}
	if htmlNoDBSite.DatabaseName != "" {
		t.Errorf("expected empty database name for html site created with db=false, got %s", htmlNoDBSite.DatabaseName)
	}
}

