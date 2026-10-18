package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallWordPressMock(t *testing.T) {
	adminPassword, err := InstallWordPress(
		"wp_testsite_com",
		"testsite.com",
		"/var/www/testsite.com/public",
		"db_testsite_com",
		"wp_testsite_com",
		"random_db_password",
		"/var/run/redis/redis-server.sock",
		"siteadmin",
		"siteadmin@testsite.com",
		"",
		"wp",
	)
	if err != nil {
		t.Fatalf("expected no error from mocked InstallWordPress, got %v", err)
	}

	if len(adminPassword) != 16 {
		t.Errorf("expected admin password length to be 16 characters, got %d", len(adminPassword))
	}
}

func TestMemoryParsingAndCompare(t *testing.T) {
	if ParseMemoryMB("128M") != 128 {
		t.Errorf("expected 128, got %d", ParseMemoryMB("128M"))
	}
	if ParseMemoryMB("256M") != 256 {
		t.Errorf("expected 256, got %d", ParseMemoryMB("256M"))
	}
	if ParseMemoryMB("1G") != 1024 {
		t.Errorf("expected 1024, got %d", ParseMemoryMB("1G"))
	}
	if ParseMemoryMB("512") != 512 {
		t.Errorf("expected 512, got %d", ParseMemoryMB("512"))
	}

	if CompareMemory("256M", "128M") <= 0 {
		t.Errorf("expected 256M > 128M")
	}
	if CompareMemory("128M", "256M") >= 0 {
		t.Errorf("expected 128M < 256M")
	}
	if CompareMemory("256M", "256M") != 0 {
		t.Errorf("expected 256M == 256M")
	}
	if CompareMemory("1G", "512M") <= 0 {
		t.Errorf("expected 1G > 512M")
	}
}

func TestReadWordPressMemoryLimit(t *testing.T) {
	tempDir := t.TempDir()
	publicDir := filepath.Join(tempDir, "htdocs")
	_ = os.MkdirAll(publicDir, 0755)

	wpConfig := `<?php
define('DB_NAME', 'test_db');
define( 'WP_MEMORY_LIMIT', '256M' );
define( 'WP_MAX_MEMORY_LIMIT', '512M' );
`
	configPath := filepath.Join(tempDir, "wp-config.php")
	if err := os.WriteFile(configPath, []byte(wpConfig), 0644); err != nil {
		t.Fatalf("failed to write wp-config: %v", err)
	}

	mem, maxMem, err := ReadWordPressMemoryLimit(publicDir)
	if err != nil {
		t.Fatalf("ReadWordPressMemoryLimit failed: %v", err)
	}
	if mem != "256M" {
		t.Errorf("expected WP_MEMORY_LIMIT 256M, got %s", mem)
	}
	if maxMem != "512M" {
		t.Errorf("expected WP_MAX_MEMORY_LIMIT 512M, got %s", maxMem)
	}
}

