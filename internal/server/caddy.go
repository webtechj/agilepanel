package server

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"text/template"

	"agilepanel/internal/config"
)

const caddyfileTemplateStr = `# Global Options Block
{
    servers {
        protocols h1 h2 h3
    }
    # Global Souin cache configuration
    cache {
        api {
            souin
        }
    }
}

# Default Welcome Page catch-all
:80 {
    root * /var/www/default
    file_server

    # Response Compression
    encode gzip zstd

    # Strict Security Headers
    header {
        X-Content-Type-Options "nosniff"
        X-Frame-Options "SAMEORIGIN"
        Referrer-Policy "no-referrer-when-downgrade"
    }
}

{{range .Sites}}
{{.Domain}} {
    root * {{.PublicDir}}
    file_server

    # Response Compression
    encode gzip zstd

    # Strict Security Headers
    header {
        Strict-Transport-Security "max-age=63072000; includeSubDomains; preload"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "SAMEORIGIN"
        Referrer-Policy "no-referrer-when-downgrade"
    }

    # Block access to hidden files/directories except .well-known
    @hidden-files {
        path */.*
        not path */.well-known/*
    }
    respond @hidden-files "Access Denied" 403

    # Block PHP execution in uploads directory
    @uploads-php {
        path_regexp (?i)^/wp-content/uploads/.*\.php$
    }
    respond @uploads-php "Access Denied" 403

    # Block access to xmlrpc.php and install.php
    @blocked-php {
        path /xmlrpc.php /wp-admin/install.php
    }
    respond @blocked-php "Access Denied" 403

    # Souin Caching for WordPress (bypasses cache for admin/login sessions & dynamic requests)
    @authorized-cache {
        not header_regexp Cookie "comment_author|wordpress_[a-f0-9]+|wp-postpass|wordpress_logged_in"
        not path_regexp "(/wp-admin/|/xmlrpc.php|/wp-(app|cron|login|register|mail).php|wp-.*.php|/feed/|index.php|wp-comments-popup.php|wp-links-opml.php|wp-locations.php|sitemap(index)?.xml|[a-z0-9-]+-sitemap([0-9]+)?.xml)"
        not method POST
        not expression {query} != ''
    }

    cache @authorized-cache {
        ttl 300s
    }

    {{if and $.Global.AdminUser $.Global.AdminPasswordHash}}
    # WordOps HTTP Basic Authentication security for phpMyAdmin and tools
    @admin-tools {
        path /phpmyadmin* /adminer* /pma*
    }
    basic_auth @admin-tools {
        {{$.Global.AdminUser}} {{$.Global.AdminPasswordHash}}
    }

    # phpMyAdmin redirect route
    route /phpmyadmin* {
        root * /usr/share
        file_server
        php_fastcgi unix//run/php/php{{.PHPVersion}}-fpm-{{.Domain}}.sock
    }
    {{end}}

    # PHP-FPM FastCGI pool coupling
    php_fastcgi unix//run/php/php{{.PHPVersion}}-fpm-{{.Domain}}.sock

    # WordPress Rewrite Rules (implicit in php_fastcgi, fallback below)
    @wp-uploaded-files {
        path_regexp wp-uploads ^/wp-content/uploads/(.*)$
    }
    rewrite @wp-uploaded-files /wp-content/uploads/{re.wp-uploads.1}
}
{{end}}
`

// GenerateCaddyfile programmatically generates the full Caddyfile configuration.
func GenerateCaddyfile(state *config.State) (string, error) {
	tmpl, err := template.New("caddyfile").Parse(caddyfileTemplateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse Caddyfile template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, state); err != nil {
		return "", fmt.Errorf("failed to execute Caddyfile template: %w", err)
	}

	return buf.String(), nil
}

// WriteCaddyfile writes the generated Caddyfile configuration to disk.
func WriteCaddyfile(state *config.State) error {
	content, err := GenerateCaddyfile(state)
	if err != nil {
		return err
	}

	caddyfilePath := state.Global.CaddyConfigPath
	
	// Create parent directories if they don't exist
	dir := filepath.Dir(caddyfilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create Caddy config folder: %w", err)
	}

	if err := os.WriteFile(caddyfilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write Caddyfile to %s: %w", caddyfilePath, err)
	}

	fmt.Printf("Caddy: Generated configuration written to %s successfully.\n", caddyfilePath)
	return nil
}

// ReloadCaddy triggers a reload of the Caddy configuration.
func ReloadCaddy(state *config.State) error {
	caddyPath := state.Global.CaddyPath
	caddyConfig := state.Global.CaddyConfigPath

	if runtime.GOOS != "linux" {
		fmt.Printf("Caddy (Mock): reload command: %s reload --config %s\n", caddyPath, caddyConfig)
		return nil
	}

	cmd := exec.Command(caddyPath, "reload", "--config", caddyConfig)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload Caddy: %v (stderr: %s)", err, stderr.String())
	}

	fmt.Println("Caddy: Reloaded configuration successfully.")
	return nil
}
