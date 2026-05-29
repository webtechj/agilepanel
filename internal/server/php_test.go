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
	if !strings.Contains(content, "php_admin_value[opcache.memory_consumption] = 256") {
		t.Error("expected custom opcache memory configuration")
	}
	if !strings.Contains(content, "php_admin_value[opcache.validate_timestamps] = 0") {
		t.Error("expected validate timestamps to be disabled")
	}
	if !strings.Contains(content, "php_admin_value[disable_functions] = exec,shell_exec,system,passthru,popen,proc_open,show_source") {
		t.Error("expected disable_functions configuration in PHP pool config")
	}
	if !strings.Contains(content, "php_admin_value[open_basedir] = /var/www/phptest.org/:/tmp/:/usr/share/phpmyadmin/:/etc/phpmyadmin/:/var/lib/phpmyadmin/:/usr/share/php/") {
		t.Error("expected open_basedir configuration in PHP pool config")
	}

	// Verify performance optimizations
	if !strings.Contains(content, "php_admin_value[opcache.interned_strings_buffer] = 16") {
		t.Error("expected opcache interned strings buffer configuration")
	}
	if !strings.Contains(content, "php_admin_value[opcache.max_accelerated_files] = 10000") {
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
