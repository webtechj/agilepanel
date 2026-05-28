package server

import (
	"strings"
	"testing"

	"agilepanel/internal/config"
)

func TestGenerateCaddyfile(t *testing.T) {
	// Setup a dummy state
	state := &config.State{
		Global: config.GlobalConfig{
			DefaultPHPVersion:    "8.3",
			SupportedPHPVersions: []string{"8.3"},
			CaddyPath:            "/usr/sbin/caddy",
			CaddyConfigPath:      "/etc/caddy/Caddyfile",
			AdminUser:            "admin",
			AdminPasswordHash:    "$2a$14$somerandombcrypt",
		},
		Sites: []config.SiteConfig{
			{
				Domain:       "siteone.com",
				PHPVersion:   "8.3",
				PublicDir:    "/var/www/siteone.com/public",
				DatabaseName: "db_siteone_com",
				DatabaseUser: "wp_siteone_com",
				IsLocked:     false,
			},
			{
				Domain:       "sitetwo.net",
				PHPVersion:   "8.2",
				PublicDir:    "/var/www/sitetwo.net/public",
				DatabaseName: "db_sitetwo_net",
				DatabaseUser: "wp_sitetwo_net",
				IsLocked:     true,
			},
		},
	}

	content, err := GenerateCaddyfile(state)
	if err != nil {
		t.Fatalf("failed to generate Caddyfile: %v", err)
	}

	// Verify global sections
	if !strings.Contains(content, "protocols h1 h2 h3") {
		t.Error("expected Caddyfile to include HTTP/3 activation protocols h1 h2 h3")
	}
	if !strings.Contains(content, "souin") {
		t.Error("expected Caddyfile to include Souin caching directives")
	}
	if !strings.Contains(content, "basic_auth @admin-tools") {
		t.Error("expected Caddyfile to include basic_auth configuration")
	}
	if !strings.Contains(content, "route /phpmyadmin*") {
		t.Error("expected Caddyfile to include phpMyAdmin route configuration")
	}

	// Verify siteone.com configurations
	if !strings.Contains(content, "siteone.com {") {
		t.Error("expected siteone.com domain block")
	}
	if !strings.Contains(content, "root * /var/www/siteone.com/public") {
		t.Error("expected siteone.com public path setting")
	}
	if !strings.Contains(content, "php_fastcgi unix//run/php/php8.3-fpm-siteone.com.sock") {
		t.Error("expected siteone.com PHP 8.3 FPM socket definition")
	}

	// Verify security hardening blocks
	if !strings.Contains(content, "@hidden-files") || !strings.Contains(content, "path */.*") {
		t.Error("expected hidden files blocking configuration in Caddyfile")
	}
	if !strings.Contains(content, "@uploads-php") || !strings.Contains(content, "path_regexp (?i)^/wp-content/uploads/.*\\.php$") {
		t.Error("expected PHP uploads blocking configuration in Caddyfile")
	}
	if !strings.Contains(content, "@blocked-php") || !strings.Contains(content, "path /xmlrpc.php /wp-admin/install.php") {
		t.Error("expected sensitive PHP paths blocking configuration in Caddyfile")
	}

	// Verify performance response compression
	if !strings.Contains(content, "encode gzip zstd") {
		t.Error("expected response compression configuration in Caddyfile")
	}

	// Verify sitetwo.net configurations
	if !strings.Contains(content, "sitetwo.net {") {
		t.Error("expected sitetwo.net domain block")
	}
	if !strings.Contains(content, "root * /var/www/sitetwo.net/public") {
		t.Error("expected sitetwo.net public path setting")
	}
	if !strings.Contains(content, "php_fastcgi unix//run/php/php8.2-fpm-sitetwo.net.sock") {
		t.Error("expected sitetwo.net PHP 8.2 FPM socket definition")
	}
}
