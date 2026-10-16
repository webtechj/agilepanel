package site

import (
	"os"
	"path/filepath"
	"testing"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
)

func TestCheckDomainDNS(t *testing.T) {
	os.Setenv("AGILEPANEL_TEST_MODE", "true")
	defer os.Unsetenv("AGILEPANEL_TEST_MODE")

	ip, matches, err := CheckDomainDNS("example.com")
	if err != nil {
		t.Fatalf("unexpected error from CheckDomainDNS in test mode: %v", err)
	}
	if ip != "127.0.0.1" || !matches {
		t.Errorf("expected 127.0.0.1 and matches=true, got ip=%s matches=%v", ip, matches)
	}
}

func TestEaseHostingFeatures(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "state.json")
	os.Setenv("AGILEPANEL_STATE_PATH", statePath)
	os.Setenv("AGILEPANEL_TEST_MODE", "true")
	defer os.Unsetenv("AGILEPANEL_STATE_PATH")
	defer os.Unsetenv("AGILEPANEL_TEST_MODE")

	domain := "ease.test.com"

	// 1. Create a WordPress test site
	err := Create(domain, "8.3", "wp", "default", "", "", "easeadmin", "initpass123", "admin@ease.test.com", "Admin")
	if err != nil {
		t.Fatalf("failed to create site: %v", err)
	}

	state, err := config.ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}
	if len(state.Sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(state.Sites))
	}
	targetSite := state.Sites[0]

	// 2. Test Doctor Diagnostics
	err = Doctor(domain)
	if err != nil {
		t.Fatalf("Doctor diagnostic failed: %v", err)
	}

	err = Doctor("nonexistent.com")
	if err == nil {
		t.Fatal("expected error for nonexistent site in Doctor, got nil")
	}

	// 3. Test Safe Mode
	err = SafeMode(domain, "on")
	if err != nil {
		t.Fatalf("SafeMode on failed: %v", err)
	}

	// Idempotent call to on should not error
	err = SafeMode(domain, "on")
	if err != nil {
		t.Fatalf("SafeMode duplicate on should not error: %v", err)
	}

	err = SafeMode(domain, "off")
	if err != nil {
		t.Fatalf("SafeMode off failed: %v", err)
	}

	// Calling off again should error because marker file is gone
	err = SafeMode(domain, "off")
	if err == nil {
		t.Fatal("expected error when disabling safe mode that is not active, got nil")
	}

	err = SafeMode(domain, "invalid_action")
	if err == nil {
		t.Fatal("expected error for invalid action, got nil")
	}

	// 4. Test Reset Password
	err = ResetPassword(domain, "easeadmin", "brandNewPassword99#")
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}

	// Auto-generated password
	err = ResetPassword(domain, "", "")
	if err != nil {
		t.Fatalf("ResetPassword with auto-generated password failed: %v", err)
	}

	err = ResetPassword("nonexistent.com", "admin", "pass")
	if err == nil {
		t.Fatal("expected error for nonexistent site in ResetPassword, got nil")
	}

	// 5. Test Switch PHP
	err = SwitchPHP(domain, "8.2")
	if err != nil {
		t.Fatalf("SwitchPHP to 8.2 failed: %v", err)
	}

	state, _ = config.ReadState(statePath)
	if state.Sites[0].PHPVersion != "8.2" {
		t.Errorf("expected PHP version 8.2, got %s", state.Sites[0].PHPVersion)
	}

	// Switch to same version should error
	err = SwitchPHP(domain, "8.2")
	if err == nil {
		t.Fatal("expected error when switching to identical PHP version, got nil")
	}

	// Switch to unsupported version should error
	err = SwitchPHP(domain, "7.4")
	if err == nil {
		t.Fatal("expected error for unsupported PHP version, got nil")
	}

	// 6. Test Optimize
	err = Optimize(domain)
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	err = Optimize("nonexistent.com")
	if err == nil {
		t.Fatal("expected error for nonexistent site in Optimize, got nil")
	}

	// 7. Test Restore
	parentDir := server.GetSiteRootDir(targetSite.PublicDir)
	backupDir := filepath.Join(parentDir, "backup")
	_ = os.MkdirAll(backupDir, 0755)

	mockFilesZip := filepath.Join(backupDir, domain+"-files-20261013-120000.zip")
	mockDbZip := filepath.Join(backupDir, domain+"-db-20261013-120000.zip")
	_ = os.WriteFile(mockFilesZip, []byte("mock zip content"), 0644)
	_ = os.WriteFile(mockDbZip, []byte("mock db zip content"), 0644)

	err = Restore(domain, "latest")
	if err != nil {
		t.Fatalf("Restore latest failed: %v", err)
	}

	err = Restore(domain, "20261013-120000")
	if err != nil {
		t.Fatalf("Restore specific timestamp failed: %v", err)
	}

	err = Restore(domain, "99999999-000000")
	if err == nil {
		t.Fatal("expected error for nonexistent backup timestamp in Restore, got nil")
	}

	// 8. Cleanup
	err = Delete(domain)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}
