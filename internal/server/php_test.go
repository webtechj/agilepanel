package server

import (
	"strings"
	"testing"

	"agilepanel/internal/config"
)

func TestGeneratePHPPool(t *testing.T) {
	site := &config.SiteConfig{
		Domain:       "phptest.org",
		PHPVersion:   "8.3",
		PublicDir:    "/var/www/phptest.org/public",
		DatabaseName: "db_phptest_org",
		DatabaseUser: "wp_phptest_org",
		SystemUser:   "wp_phptest_org",
		IsLocked:     false,
	}

	content, err := GeneratePHPPool(site)
	if err != nil {
		t.Fatalf("failed to generate PHP pool: %v", err)
	}

	// Verify pool config elements
	if !strings.Contains(content, "[phptest.org]") {
		t.Error("expected pool block name [phptest.org]")
	}
	if !strings.Contains(content, "user = wp_phptest_org") {
		t.Error("expected pool user configuration")
	}
	if !strings.Contains(content, "group = wp_phptest_org") {
		t.Error("expected pool group configuration")
	}
	if !strings.Contains(content, "listen = /run/php/php8.3-fpm-phptest.org.sock") {
		t.Error("expected unix socket path setup")
	}
	if !strings.Contains(content, "listen.owner = caddy") {
		t.Error("expected listen owner to be caddy")
	}
	if !strings.Contains(content, "pm = ondemand") {
		t.Error("expected default pm = ondemand for standard site")
	}
	if !strings.Contains(content, "pm.process_idle_timeout = 15s") {
		t.Error("expected process_idle_timeout = 15s")
	}
	if !strings.Contains(content, "pm.max_requests = 500") {
		t.Error("expected max_requests = 500")
	}
	if !strings.Contains(content, "pm.status_path = /fpm-status") {
		t.Error("expected status_path to be configured")
	}
	if !strings.Contains(content, "php_admin_value[opcache.memory_consumption] = 128") {
		t.Errorf("expected opcache memory configuration 128 for 2GB mock environment, got:\n%s", content)
	}
	if !strings.Contains(content, "php_admin_value[opcache.validate_timestamps] = 1") {
		t.Error("expected validate timestamps to be enabled")
	}
	if !strings.Contains(content, "php_admin_value[opcache.revalidate_freq] = 2") {
		t.Error("expected opcache revalidate_freq to be 2")
	}
	if !strings.Contains(content, "php_admin_value[disable_functions] = exec,shell_exec,system,passthru,popen,proc_open,show_source") {
		t.Error("expected disable_functions configuration in PHP pool config")
	}
	if !strings.Contains(content, "php_admin_value[open_basedir] = /var/www/phptest.org/:/tmp/:/usr/share/php/") {
		t.Error("expected open_basedir configuration in PHP pool config")
	}

	// Verify performance optimizations
	if !strings.Contains(content, "php_admin_value[opcache.interned_strings_buffer] = 16") {
		t.Error("expected opcache interned strings buffer configuration")
	}
	if !strings.Contains(content, "php_admin_value[opcache.max_accelerated_files] = 20000") {
		t.Error("expected opcache max accelerated files configuration")
	}
	if !strings.Contains(content, "php_admin_value[memory_limit] = 128M") {
		t.Error("expected PHP memory limit configuration")
	}
	if !strings.Contains(content, "php_admin_value[upload_max_filesize] = 100M") {
		t.Error("expected PHP upload max filesize configuration")
	}
	if !strings.Contains(content, "php_admin_value[post_max_size] = 100M") {
		t.Error("expected PHP post max size configuration")
	}
}

func TestGeneratePHPPoolCustomMemory(t *testing.T) {
	site := &config.SiteConfig{
		Domain:       "heavy-store.com",
		PHPVersion:   "8.3",
		PublicDir:    "/var/www/heavy-store.com/htdocs",
		DatabaseName: "db_heavy_store",
		DatabaseUser: "usr_heavy_store",
		SystemUser:   "wp_heavy_store",
		Type:         "woocommerce",
		MemoryLimit:  "256M",
	}

	content, err := GeneratePHPPool(site)
	if err != nil {
		t.Fatalf("failed to generate PHP pool: %v", err)
	}

	if !strings.Contains(content, "pm = dynamic") {
		t.Errorf("expected pm = dynamic for woocommerce/heavy site, got:\n%s", content)
	}
	if !strings.Contains(content, "pm.start_servers =") {
		t.Errorf("expected pm.start_servers in dynamic mode, got:\n%s", content)
	}
	if !strings.Contains(content, "pm.max_requests = 1000") {
		t.Errorf("expected pm.max_requests = 1000 for woocommerce, got:\n%s", content)
	}
	if !strings.Contains(content, "php_admin_value[memory_limit] = 256M") {
		t.Errorf("expected memory_limit = 256M for woocommerce/heavy site, got:\n%s", content)
	}
	if !strings.Contains(content, "php_admin_value[max_input_vars] = 5000") {
		t.Errorf("expected max_input_vars = 5000, got:\n%s", content)
	}
	if !strings.Contains(content, "php_admin_value[max_execution_time] = 300") {
		t.Errorf("expected max_execution_time = 300, got:\n%s", content)
	}
}

func TestCalculatePHPMaxChildren(t *testing.T) {
	tests := []struct {
		name       string
		ramKB      int64
		siteType   string
		minAllowed int
		maxAllowed int
	}{
		{"1GB Standard", 1024 * 1024, "wp", 4, 10},
		{"1GB WooCommerce", 1024 * 1024, "woocommerce", 4, 8},
		{"2GB Standard", 2048 * 1024, "wp", 10, 20},
		{"2GB WooCommerce", 2048 * 1024, "woocommerce", 6, 15},
		{"4GB Standard", 4096 * 1024, "wp", 25, 45},
		{"4GB WooCommerce", 4096 * 1024, "woocommerce", 15, 30},
		{"8GB Standard", 8192 * 1024, "wp", 50, 80},
		{"8GB WooCommerce", 8192 * 1024, "woocommerce", 30, 55},
		{"16GB Standard", 16384 * 1024, "wp", 100, 120},
		{"16GB WooCommerce", 16384 * 1024, "woocommerce", 75, 110},
		{"Fallback Windows", 0, "wp", 4, 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculatePHPMaxChildren(tt.ramKB, tt.siteType)
			if got < tt.minAllowed || got > tt.maxAllowed {
				t.Errorf("CalculatePHPMaxChildren(%d, %s) = %d; want between %d and %d",
					tt.ramKB, tt.siteType, got, tt.minAllowed, tt.maxAllowed)
			}
		})
	}
}

func TestCalculateMultiSitePHPMaxChildren(t *testing.T) {
	ram4GB := int64(4096 * 1024)

	// Single site on 4GB
	singleSite := CalculateMultiSitePHPMaxChildren(ram4GB, "wp", 1)
	if singleSite < 25 || singleSite > 45 {
		t.Errorf("expected 25-45 workers for 1 site on 4GB RAM, got %d", singleSite)
	}

	// 5 sites on 4GB
	fiveSites := CalculateMultiSitePHPMaxChildren(ram4GB, "wp", 5)
	if fiveSites >= singleSite {
		t.Errorf("expected 5 sites to have lower max_children than 1 site, got %d vs %d", fiveSites, singleSite)
	}
	if fiveSites < 4 || fiveSites > 16 {
		t.Errorf("expected 4-16 workers per site for 5 sites on 4GB RAM, got %d", fiveSites)
	}

	// 20 sites on 4GB
	twentySites := CalculateMultiSitePHPMaxChildren(ram4GB, "wp", 20)
	if twentySites < 2 || twentySites > 5 {
		t.Errorf("expected 2-5 workers per site for 20 sites on 4GB RAM, got %d", twentySites)
	}
}

func TestCalculateOpcacheSettings(t *testing.T) {
	mem1GB, str1GB := CalculateOpcacheSettings(1024 * 1024)
	if mem1GB != 64 || str1GB != 8 {
		t.Errorf("1GB tier: got mem=%d, str=%d; want 64, 8", mem1GB, str1GB)
	}

	mem2GB, str2GB := CalculateOpcacheSettings(2048 * 1024)
	if mem2GB != 128 || str2GB != 16 {
		t.Errorf("2GB tier: got mem=%d, str=%d; want 128, 16", mem2GB, str2GB)
	}

	mem8GB, str8GB := CalculateOpcacheSettings(8192 * 1024)
	if mem8GB != 256 || str8GB != 32 {
		t.Errorf("8GB tier: got mem=%d, str=%d; want 256, 32", mem8GB, str8GB)
	}

	mem16GB, str16GB := CalculateOpcacheSettings(16384 * 1024)
	if mem16GB != 512 || str16GB != 64 {
		t.Errorf("16GB tier: got mem=%d, str=%d; want 512, 64", mem16GB, str16GB)
	}
}

func TestReadPHPPoolConfigAndOrphaned(t *testing.T) {
	site := &config.SiteConfig{
		Domain:      "orphantest.com",
		PHPVersion:  "8.3",
		PublicDir:   "/var/www/orphantest.com/htdocs",
		SystemUser:  "wp_orphantest",
		Type:        "woocommerce",
		MemoryLimit: "256M",
	}

	if err := WritePHPPool(site); err != nil {
		t.Fatalf("WritePHPPool failed: %v", err)
	}
	defer DeletePHPPool("8.3", "orphantest.com")

	cfg, err := ReadPHPPoolConfig("8.3", "orphantest.com")
	if err != nil {
		t.Fatalf("ReadPHPPoolConfig failed: %v", err)
	}

	if cfg.Domain != "orphantest.com" {
		t.Errorf("expected domain orphantest.com, got %s", cfg.Domain)
	}
	if cfg.MemoryLimit != "256M" {
		t.Errorf("expected MemoryLimit 256M, got %s", cfg.MemoryLimit)
	}
	if cfg.OpcacheValidateTimestamps != 1 {
		t.Errorf("expected OpcacheValidateTimestamps = 1, got %d", cfg.OpcacheValidateTimestamps)
	}
	if cfg.OpcacheRevalidateFreq != 2 {
		t.Errorf("expected OpcacheRevalidateFreq = 2, got %d", cfg.OpcacheRevalidateFreq)
	}
	if cfg.PM != "dynamic" {
		t.Errorf("expected PM = dynamic, got %s", cfg.PM)
	}

	// Create a dummy pool in 8.2 to simulate an orphaned pool
	site82 := *site
	site82.PHPVersion = "8.2"
	if err := WritePHPPool(&site82); err != nil {
		t.Fatalf("WritePHPPool 8.2 failed: %v", err)
	}
	defer DeletePHPPool("8.2", "orphantest.com")

	orphans := FindOrphanedPHPPools("orphantest.com", "8.3", []string{"8.1", "8.2", "8.3"})
	if len(orphans) == 0 || orphans[0] != "8.2" {
		t.Errorf("expected orphaned pool in 8.2, got %v", orphans)
	}

	cleaned, err := DeleteOrphanedPHPPools("orphantest.com", "8.3", []string{"8.1", "8.2", "8.3"})
	if err != nil {
		t.Fatalf("DeleteOrphanedPHPPools failed: %v", err)
	}
	if len(cleaned) == 0 || cleaned[0] != "8.2" {
		t.Errorf("expected cleaned version 8.2, got %v", cleaned)
	}

	// Check that orphans are now empty
	postOrphans := FindOrphanedPHPPools("orphantest.com", "8.3", []string{"8.1", "8.2", "8.3"})
	if len(postOrphans) != 0 {
		t.Errorf("expected 0 orphans after delete, got %v", postOrphans)
	}
}

