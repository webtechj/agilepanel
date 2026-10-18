package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
)

func TestDoctorDiagnosticsAndAutoFix(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")
	caddyPath := filepath.Join(tempDir, "Caddyfile")
	os.Setenv("AGILEPANEL_STATE_PATH", statePath)
	os.Setenv("AGILEPANEL_TEST_MODE", "true")
	defer os.Unsetenv("AGILEPANEL_STATE_PATH")
	defer os.Unsetenv("AGILEPANEL_TEST_MODE")

	domain := "missbaha.com"
	_ = server.DeletePHPPool("8.2", domain)
	defer server.DeletePHPPool("8.2", domain)

	publicDir := filepath.Join(tempDir, "htdocs")
	_ = os.MkdirAll(publicDir, 0755)

	// Create test state
	state := &config.State{
		Global: config.GlobalConfig{
			CaddyConfigPath:   caddyPath,
			DefaultPHPVersion: "8.2",
		},
		Sites: []config.SiteConfig{
			{
				Domain:       domain,
				PHPVersion:   "8.2",
				PublicDir:    publicDir,
				SystemUser:   "wp_missbaha",
				Type:         "wp",
				MemoryLimit:  "128M",
				DatabaseName: "missbaha_db",
				DatabaseUser: "missbaha_user",
			},
		},
	}
	_ = config.WithLockedState(statePath, func(s *config.State) error {
		*s = *state
		return nil
	})

	// 1. Verify Caddyfile template generates WWW canonical redirect for apex domain (prevents Error 525)
	caddyContent, err := server.GenerateCaddyfile(state)
	if err != nil {
		t.Fatalf("GenerateCaddyfile failed: %v", err)
	}
	if !strings.Contains(caddyContent, "www.missbaha.com {") {
		t.Errorf("expected Caddyfile to contain 'www.missbaha.com {' redirect block, got:\n%s", caddyContent)
	}
	if !strings.Contains(caddyContent, "redir https://missbaha.com{uri} permanent") {
		t.Errorf("expected Caddyfile to contain redirect to https://missbaha.com{uri}, got:\n%s", caddyContent)
	}

	// 2. Verify subdomains (e.g. trends.rusicaa.com) do NOT generate double www
	subdomainState := &config.State{
		Global: config.GlobalConfig{CaddyConfigPath: caddyPath},
		Sites: []config.SiteConfig{
			{Domain: "trends.rusicaa.com", PublicDir: publicDir, Type: "wp", PHPVersion: "8.2"},
		},
	}
	subContent, err := server.GenerateCaddyfile(subdomainState)
	if err != nil {
		t.Fatalf("GenerateCaddyfile for subdomain failed: %v", err)
	}
	if strings.Contains(subContent, "www.trends.rusicaa.com {") {
		t.Errorf("did not expect subdomain to generate www.trends.rusicaa.com redirect block, got:\n%s", subContent)
	}

	// 3. Test Doctor diagnostic run (read-only)
	err = Doctor(domain)
	if err != nil {
		t.Fatalf("Doctor(domain) failed: %v", err)
	}

	// 4. Test DoctorAll run
	err = DoctorAll(false)
	if err != nil {
		t.Fatalf("DoctorAll(false) failed: %v", err)
	}

	// 5. Test DoctorWithFix auto-remediation
	// Simulate an Elementor plugin installed in wp-content/plugins/elementor
	pluginsDir := filepath.Join(publicDir, "wp-content", "plugins", "elementor")
	_ = os.MkdirAll(pluginsDir, 0755)

	err = DoctorWithFix(domain, true)
	if err != nil {
		t.Fatalf("DoctorWithFix(domain, true) failed: %v", err)
	}

	// Verify state was upgraded to 256M memory limit for heavy plugin
	updatedState, err := config.ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to read updated state: %v", err)
	}
	if updatedState.Sites[0].MemoryLimit != "256M" {
		t.Errorf("expected site MemoryLimit to be auto-upgraded to 256M, got '%s'", updatedState.Sites[0].MemoryLimit)
	}
}

func TestDoctorWPConfigMemoryConflictAndOrphanCleanup(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")
	caddyPath := filepath.Join(tempDir, "Caddyfile")
	os.Setenv("AGILEPANEL_STATE_PATH", statePath)
	os.Setenv("AGILEPANEL_TEST_MODE", "true")
	defer os.Unsetenv("AGILEPANEL_STATE_PATH")
	defer os.Unsetenv("AGILEPANEL_TEST_MODE")

	domain := "tamras-conflict.in"
	publicDir := filepath.Join(tempDir, domain, "htdocs")
	_ = os.MkdirAll(publicDir, 0755)

	// Create wp-config.php in parent dir requesting 256M
	wpConfigContent := `<?php
define('DB_NAME', 'tamras_db');
define('WP_MEMORY_LIMIT', '256M');
define('WP_MAX_MEMORY_LIMIT', '512M');
`
	_ = os.WriteFile(filepath.Join(tempDir, domain, "wp-config.php"), []byte(wpConfigContent), 0644)

	state := &config.State{
		Global: config.GlobalConfig{
			CaddyConfigPath:      caddyPath,
			DefaultPHPVersion:    "8.3",
			SupportedPHPVersions: []string{"8.1", "8.2", "8.3"},
		},
		Sites: []config.SiteConfig{
			{
				Domain:       domain,
				PHPVersion:   "8.3",
				PublicDir:    publicDir,
				SystemUser:   "wp_tamras",
				Type:         "wp",
				MemoryLimit:  "128M",
				DatabaseName: "tamras_db",
				DatabaseUser: "tamras_user",
			},
		},
	}
	_ = config.WithLockedState(statePath, func(s *config.State) error {
		*s = *state
		return nil
	})

	// 1. Write active 8.3 pool with 128M limit
	if err := server.WritePHPPool(&state.Sites[0]); err != nil {
		t.Fatalf("WritePHPPool 8.3 failed: %v", err)
	}
	defer server.DeletePHPPool("8.3", domain)

	// 2. Create orphaned pool in PHP 8.2
	site82 := state.Sites[0]
	site82.PHPVersion = "8.2"
	if err := server.WritePHPPool(&site82); err != nil {
		t.Fatalf("WritePHPPool 8.2 failed: %v", err)
	}
	defer server.DeletePHPPool("8.2", domain)

	// 3. Run Doctor diagnostic
	if err := Doctor(domain); err != nil {
		t.Fatalf("Doctor diagnostic failed: %v", err)
	}

	// 4. Run DoctorWithFix
	if err := DoctorWithFix(domain, true); err != nil {
		t.Fatalf("DoctorWithFix failed: %v", err)
	}

	// 5. Verify MemoryLimit was scaled to 256M
	updatedState, err := config.ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to read updated state: %v", err)
	}
	if updatedState.Sites[0].MemoryLimit != "256M" {
		t.Errorf("expected MemoryLimit to be upgraded to 256M, got '%s'", updatedState.Sites[0].MemoryLimit)
	}

	// 6. Verify pool on disk is now 256M and validate_timestamps is 1
	poolCfg, err := server.ReadPHPPoolConfig("8.3", domain)
	if err != nil {
		t.Fatalf("ReadPHPPoolConfig 8.3 failed: %v", err)
	}
	if poolCfg.MemoryLimit != "256M" {
		t.Errorf("expected pool MemoryLimit 256M, got '%s'", poolCfg.MemoryLimit)
	}
	if poolCfg.OpcacheValidateTimestamps != 1 {
		t.Errorf("expected OpcacheValidateTimestamps 1, got %d", poolCfg.OpcacheValidateTimestamps)
	}

	// 7. Verify orphaned 8.2 pool was deleted
	orphans := server.FindOrphanedPHPPools(domain, "8.3", state.Global.SupportedPHPVersions)
	if len(orphans) > 0 {
		t.Errorf("expected 0 orphaned pools, but found: %v", orphans)
	}
}

