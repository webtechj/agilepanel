package server

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"agilepanel/internal/config"
)

const caddyfileTemplateStr = `# Global Options Block
{
    {{if .Global.AdminEmail}}email {{.Global.AdminEmail}}{{end}}
    servers {
        protocols h1 h2 h3
        timeouts {
            read_body 10s
            read_header 10s
            write 30s
            idle 120s
        }
    }
    auto_https disable_redirects
}

# Default Welcome Page catch-all
:80 {
    root * /var/www/default
    file_server

    {{if and .Global.AdminUser .Global.AdminPasswordHash}}
    basic_auth {
        {{.Global.AdminUser}} "{{.Global.AdminPasswordHash}}"
    }
    {{else}}
    respond "Access Denied: Administrative HTTP Basic Authentication has not been configured yet." 403
    {{end}}

    # Response Compression
    encode gzip zstd

    # Strict Security Headers & Search Engine Blocking
    header {
        X-Robots-Tag "noindex, nofollow, nosnippet, noarchive"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "SAMEORIGIN"
        Referrer-Policy "no-referrer-when-downgrade"
    }
}

# phpMyAdmin secured custom port
:8888 {
    {{if and .Global.AdminUser .Global.AdminPasswordHash}}
    root * /usr/share/phpmyadmin
    file_server

    basic_auth {
        {{.Global.AdminUser}} "{{.Global.AdminPasswordHash}}"
    }

    # Connect to default PHP-FPM socket
    php_fastcgi unix//run/php/php{{.Global.DefaultPHPVersion}}-fpm.sock {
        env DOCUMENT_ROOT /usr/share/phpmyadmin
        index index.php
    }

    # Response Compression
    encode gzip zstd

    # Strict Security Headers
    header {
        X-Content-Type-Options "nosniff"
        X-Frame-Options "SAMEORIGIN"
        Referrer-Policy "no-referrer-when-downgrade"
    }
    {{else}}
    respond "Access Denied: Administrative HTTP Basic Authentication has not been configured yet. Run 'ap server auth' to configure credentials." 403
    {{end}}
}

{{range .Sites}}
# Live website block
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

    # Static Asset Browser Caching
    @static {
        path *.ico *.css *.js *.gif *.webp *.avif *.jpg *.jpeg *.png *.svg *.woff *.woff2
    }
    header @static Cache-Control "public, max-age=31536000, immutable"

    # Block access to hidden files/directories except .well-known
    @hidden-files {
        path */.*
        not path */.well-known/*
    }
    respond @hidden-files "Access Denied" 403

    {{if ne .Type "html"}}
    # Block PHP execution in uploads directory (including double extensions & alternate scripts)
    @uploads-php {
        path_regexp (?i)^/wp-content/uploads/.*(?:\.php[0-9]*|\.phtml|\.phar|\.phps|\.pht|\.inc)(\.|\/|$)
    }
    respond @uploads-php "Access Denied" 403

    # Block access to xmlrpc.php, install.php, and wp-config.php
    @blocked-php {
        path /xmlrpc.php /wp-admin/install.php /wp-config.php
    }
    respond @blocked-php "Access Denied" 403

    # PHP-FPM FastCGI pool coupling
    php_fastcgi unix//run/php/php{{.PHPVersion}}-fpm-{{.Domain}}.sock
    {{end}}

    {{if or (eq .Type "wp") (eq .Type "woocommerce") (eq .Type "")}}
    # WordPress Rewrite Rules (implicit in php_fastcgi, fallback below)
    @wp-uploaded-files {
        path_regexp wp-uploads ^/wp-content/uploads/(.*)$
    }
    rewrite @wp-uploaded-files /wp-content/uploads/{re.wp-uploads.1}
    {{end}}
}

{{if isApex .Domain}}
# Canonical WWW to Apex redirect block (Prevents Cloudflare 525 SSL Handshake Failures)
www.{{.Domain}} {
    redir https://{{.Domain}}{uri} permanent
}
{{else if isWWW .Domain}}
# Canonical Apex to WWW redirect block
{{trimWWW .Domain}} {
    redir https://{{.Domain}}{uri} permanent
}
{{end}}

# Protected Staging website block
{{if $.ServerIP}}
http://{{.Domain}}.{{$.ServerIP}}.sslip.io {
    root * {{.PublicDir}}
    file_server

    # Enforce basic authentication for staging if not unlocked
    {{if not .StagingUnlocked}}
    {{if and $.Global.AdminUser $.Global.AdminPasswordHash}}
    basic_auth {
        {{$.Global.AdminUser}} "{{$.Global.AdminPasswordHash}}"
    }
    {{else}}
    respond "Access Denied: Administrative HTTP Basic Authentication has not been configured yet. Run 'ap server auth' to configure credentials." 403
    {{end}}
    {{end}}

    # Response Compression
    encode gzip zstd

    # Strict Security Headers & Search Engine Blocking
    header {
        X-Robots-Tag "noindex, nofollow, nosnippet, noarchive"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "SAMEORIGIN"
        Referrer-Policy "no-referrer-when-downgrade"
    }

    # Static Asset Browser Caching
    @static {
        path *.ico *.css *.js *.gif *.webp *.avif *.jpg *.jpeg *.png *.svg *.woff *.woff2
    }
    header @static Cache-Control "public, max-age=31536000, immutable"

    # Block access to hidden files/directories except .well-known
    @hidden-files {
        path */.*
        not path */.well-known/*
    }
    respond @hidden-files "Access Denied" 403

    {{if ne .Type "html"}}
    # Block PHP execution in uploads directory (including double extensions & alternate scripts)
    @uploads-php {
        path_regexp (?i)^/wp-content/uploads/.*(?:\.php[0-9]*|\.phtml|\.phar|\.phps|\.pht|\.inc)(\.|\/|$)
    }
    respond @uploads-php "Access Denied" 403

    # Block access to xmlrpc.php, install.php, and wp-config.php
    @blocked-php {
        path /xmlrpc.php /wp-admin/install.php /wp-config.php
    }
    respond @blocked-php "Access Denied" 403

    # PHP-FPM FastCGI pool coupling
    php_fastcgi unix//run/php/php{{.PHPVersion}}-fpm-{{.Domain}}.sock
    {{end}}

    {{if or (eq .Type "wp") (eq .Type "woocommerce") (eq .Type "")}}
    # WordPress Rewrite Rules (implicit in php_fastcgi, fallback below)
    @wp-uploaded-files {
        path_regexp wp-uploads ^/wp-content/uploads/(.*)$
    }
    rewrite @wp-uploaded-files /wp-content/uploads/{re.wp-uploads.1}
    {{end}}
}
{{end}}
{{end}}
`

// IsApexDomain returns true if the domain is a root/apex domain (e.g. missbaha.com, rusicaa.com, example.co.uk)
func IsApexDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" || domain == "localhost" || net.ParseIP(domain) != nil {
		return false
	}
	if strings.HasPrefix(domain, "www.") {
		return false
	}
	parts := strings.Split(domain, ".")
	if len(parts) == 2 {
		return true
	}
	// Common 2-part ccTLDs like .co.uk, .com.au, .org.in, .gov.in, .edu.pk
	if len(parts) == 3 {
		secondLevel := parts[1]
		commonSLDs := map[string]bool{
			"co": true, "com": true, "org": true, "net": true, "gov": true, "edu": true, "ac": true,
		}
		if commonSLDs[secondLevel] && len(parts[2]) == 2 {
			return true
		}
	}
	return false
}

// IsWWWDomain returns true if domain starts with "www."
func IsWWWDomain(domain string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(domain)), "www.")
}

// TrimWWW returns the apex domain without "www." prefix
func TrimWWW(domain string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "www.")
}

// GenerateCaddyfile programmatically generates the full Caddyfile configuration.
func GenerateCaddyfile(state *config.State) (string, error) {
	tmpl, err := template.New("caddyfile").Funcs(template.FuncMap{
		"isApex":  IsApexDomain,
		"isWWW":   IsWWWDomain,
		"trimWWW": TrimWWW,
	}).Parse(caddyfileTemplateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse Caddyfile template: %w", err)
	}

	serverIP := ResolvePublicIP()

	data := struct {
		Global   config.GlobalConfig
		Sites    []config.SiteConfig
		ServerIP string
	}{
		Global:   state.Global,
		Sites:    state.Sites,
		ServerIP: serverIP,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
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

// ReloadCaddy triggers a reload of the Caddy configuration, or starts it if it is not running.
func ReloadCaddy(state *config.State) error {
	caddyPath := state.Global.CaddyPath
	caddyConfig := state.Global.CaddyConfigPath

	if runtime.GOOS != "linux" {
		fmt.Printf("Caddy (Mock): reload command: %s reload --config %s\n", caddyPath, caddyConfig)
		return nil
	}

	// Check if Caddy is currently active
	isActive := false
	statusCmd := exec.Command("systemctl", "is-active", "caddy")
	if err := statusCmd.Run(); err == nil {
		isActive = true
	}

	if isActive {
		// Use caddy reload for zero-downtime config updates
		cmd := exec.Command(caddyPath, "reload", "--config", caddyConfig)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			// If reload fails (e.g. admin port issues), fallback to systemctl restart
			fmt.Printf("Caddy reload failed, falling back to restart: %v (stderr: %s)\n", err, stderr.String())
			restartCmd := exec.Command("systemctl", "restart", "caddy")
			if err := restartCmd.Run(); err != nil {
				return fmt.Errorf("failed to restart Caddy: %w", err)
			}
		} else {
			fmt.Println("Caddy: Reloaded configuration successfully.")
		}
	} else {
		// Caddy is stopped or failed, restart to boot it up
		fmt.Println("Caddy is not running. Starting Caddy service...")
		restartCmd := exec.Command("systemctl", "restart", "caddy")
		if err := restartCmd.Run(); err != nil {
			return fmt.Errorf("failed to start Caddy: %w", err)
		}
	}

	return nil
}
