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

const phpPoolTemplateStr = `[{{.Domain}}]
user = {{.SystemUser}}
group = {{.SystemUser}}

listen = /run/php/php{{.PHPVersion}}-fpm-{{.Domain}}.sock
listen.owner = caddy
listen.group = caddy
listen.mode = 0660

pm = dynamic
pm.max_children = 10
pm.start_servers = 2
pm.min_spare_servers = 1
pm.max_spare_servers = 3

php_admin_value[opcache.memory_consumption] = 256
php_admin_value[opcache.validate_timestamps] = 0
php_admin_value[opcache.interned_strings_buffer] = 16
php_admin_value[opcache.max_accelerated_files] = 10000
php_admin_value[memory_limit] = 256M
php_admin_value[upload_max_filesize] = 100M
php_admin_value[post_max_size] = 100M
php_admin_value[disable_functions] = exec,shell_exec,system,passthru,popen,proc_open,show_source
php_admin_value[open_basedir] = /var/www/{{.Domain}}/:/tmp/:/usr/share/phpmyadmin/:/etc/phpmyadmin/:/var/lib/phpmyadmin/:/usr/share/php/
`

// GeneratePHPPool generates the PHP-FPM pool configuration string.
func GeneratePHPPool(site *config.SiteConfig) (string, error) {
	tmpl, err := template.New("phppool").Parse(phpPoolTemplateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse PHP pool template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, site); err != nil {
		return "", fmt.Errorf("failed to execute PHP pool template: %w", err)
	}

	return buf.String(), nil
}

// GetPHPPoolPath returns the target filepath of the pool configuration.
func GetPHPPoolPath(version string, domain string) string {
	if runtime.GOOS != "linux" {
		// Mock folder in current workspace for local debug/verification on Windows
		return filepath.Join("etc", "php", version, "fpm", "pool.d", domain+".conf")
	}
	return fmt.Sprintf("/etc/php/%s/fpm/pool.d/%s.conf", version, domain)
}

// WritePHPPool writes the generated PHP pool configuration to disk.
func WritePHPPool(site *config.SiteConfig) error {
	content, err := GeneratePHPPool(site)
	if err != nil {
		return err
	}

	poolPath := GetPHPPoolPath(site.PHPVersion, site.Domain)
	
	dir := filepath.Dir(poolPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create PHP pool folder: %w", err)
	}

	if err := os.WriteFile(poolPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write PHP pool to %s: %w", poolPath, err)
	}

	fmt.Printf("PHP: Generated pool config written to %s successfully.\n", poolPath)
	return nil
}

// DeletePHPPool deletes the PHP pool configuration.
func DeletePHPPool(version string, domain string) error {
	poolPath := GetPHPPoolPath(version, domain)
	if _, err := os.Stat(poolPath); err == nil {
		if err := os.Remove(poolPath); err != nil {
			return fmt.Errorf("failed to delete PHP pool file %s: %w", poolPath, err)
		}
		fmt.Printf("PHP: Removed pool config at %s.\n", poolPath)
	}
	return nil
}

// ReloadPHP triggers a systemctl reload for the given PHP version.
func ReloadPHP(version string) error {
	serviceName := fmt.Sprintf("php%s-fpm", version)
	
	if runtime.GOOS != "linux" {
		fmt.Printf("PHP (Mock): systemctl reload %s\n", serviceName)
		return nil
	}

	cmd := exec.Command("systemctl", "reload", serviceName)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload PHP-FPM service: %v (stderr: %s)", err, stderr.String())
	}

	fmt.Printf("PHP: Reloaded %s successfully.\n", serviceName)
	return nil
}
