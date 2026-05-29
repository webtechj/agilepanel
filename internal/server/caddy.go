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

    # Response Compression
    encode gzip zstd

    # Strict Security Headers
    header {
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
{{.Domain}}{{if $.ServerIP}}, {{.Domain}}.{{$.ServerIP}}.sslip.io{{end}} {
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
