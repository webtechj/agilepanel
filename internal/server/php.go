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

{{if eq .PM "dynamic"}}
pm = dynamic
pm.max_children = {{.MaxChildren}}
pm.start_servers = {{.StartServers}}
pm.min_spare_servers = {{.MinSpareServers}}
pm.max_spare_servers = {{.MaxSpareServers}}
pm.max_requests = 1000
{{else}}
pm = ondemand
pm.max_children = {{.MaxChildren}}
pm.process_idle_timeout = 15s
pm.max_requests = 500
{{end}}

pm.status_path = /fpm-status

php_admin_value[opcache.memory_consumption] = {{.OpcacheMemory}}
php_admin_value[opcache.interned_strings_buffer] = {{.OpcacheStrings}}
php_admin_value[opcache.max_accelerated_files] = 20000
php_admin_value[opcache.validate_timestamps] = 1
php_admin_value[opcache.revalidate_freq] = 2
php_admin_value[memory_limit] = {{.MemoryLimit}}
php_admin_value[upload_max_filesize] = 100M
php_admin_value[post_max_size] = 100M
php_admin_value[max_execution_time] = {{.MaxExecutionTime}}
php_admin_value[max_input_vars] = {{.MaxInputVars}}
php_admin_value[disable_functions] = exec,shell_exec,system,passthru,popen,proc_open,show_source
php_admin_value[open_basedir] = /var/www/{{.Domain}}/:/tmp/:/usr/share/php/
`

// CalculatePHPMaxChildren dynamically bounds max_children according to available RAM and workload.
func CalculatePHPMaxChildren(totalRAMKB int64, siteType string) int {
	if totalRAMKB <= 0 {
		if siteType == "woocommerce" {
			return 16
		}
		return 10
	}
	totalRAMMB := totalRAMKB / 1024

	// 1. Operating system and core daemons reserve
	var osReserveMB int64 = 512
	if totalRAMMB <= 1024 {
		osReserveMB = 384
	} else if totalRAMMB <= 2048 {
		osReserveMB = 512
	} else if totalRAMMB <= 4096 {
		osReserveMB = 768
	} else if totalRAMMB <= 8192 {
		osReserveMB = 1024
	} else {
		osReserveMB = 1536
	}

	// 2. Subtract MariaDB buffer pool
	dbBufferMB := int64(CalculateBufferPoolMB(totalRAMMB))

	// 3. Subtract Redis cache memory
	var redisMB int64 = 64
	if totalRAMMB <= 1024 {
		redisMB = 64
	} else if totalRAMMB <= 4096 {
		redisMB = 128
	} else if totalRAMMB <= 8192 {
		redisMB = 256
	} else {
		redisMB = 512
	}

	// Net RAM available for PHP workers
	netPHPRAM := totalRAMMB - osReserveMB - dbBufferMB - redisMB
	if netPHPRAM < 256 {
		netPHPRAM = 256 // Guarantee minimal operational headroom
	}

	// 4. Memory footprint by workload type
	workerMB := 55 // Standard WordPress / PHP process footprint
	if siteType == "woocommerce" {
		workerMB = 85 // WooCommerce requires higher memory per worker
	}

	maxChildren := int(netPHPRAM / int64(workerMB))
	if maxChildren < 4 {
		maxChildren = 4
	}
	// Bound upper ceiling per individual pool to prevent single-site resource starvation
	if maxChildren > 120 {
		maxChildren = 120
	}
	return maxChildren
}

// CalculateOpcacheSettings dynamically scales OPcache memory according to physical RAM.
func CalculateOpcacheSettings(totalRAMKB int64) (memoryMB int, stringsMB int) {
	if totalRAMKB <= 0 {
		return 256, 16
	}
	totalRAMMB := totalRAMKB / 1024
	if totalRAMMB <= 1024 {
		return 64, 8
	} else if totalRAMMB <= 2048 {
		return 128, 16
	} else if totalRAMMB <= 8192 {
		return 256, 32
	}
	return 512, 64
}

// GeneratePHPPool generates the PHP-FPM pool configuration string.
func GeneratePHPPool(site *config.SiteConfig) (string, error) {
	totalRAM, _, _ := readLinuxMemory()
	maxChildren := CalculatePHPMaxChildren(totalRAM, site.Type)
	opcacheMem, opcacheStrings := CalculateOpcacheSettings(totalRAM)

	// Smart Adaptive PM policy:
	// Dynamic for WooCommerce/eCommerce for warm worker availability, On-Demand for standard/staging sites
	pmMode := "ondemand"
	if site.Type == "woocommerce" {
		pmMode = "dynamic"
	}

	// Dynamic PM server allocations with PHP-FPM invariant bounds:
	// min_spare <= start_servers <= max_spare <= max_children
	minSpare := maxChildren / 8
	if minSpare < 2 {
		minSpare = 2
	}
	maxSpare := maxChildren / 3
	if maxSpare < 4 {
		maxSpare = 4
	}
	if maxSpare > maxChildren {
		maxSpare = maxChildren
	}
	startServers := (minSpare + maxSpare) / 2
	if startServers < minSpare {
		startServers = minSpare
	}
	if startServers > maxSpare {
		startServers = maxSpare
	}

	memoryLimit := site.MemoryLimit
	if memoryLimit == "" {
		if site.Type == "woocommerce" {
			memoryLimit = "256M"
		} else {
			memoryLimit = "128M"
		}
	}

	maxExecutionTime := site.MaxExecutionTime
	if maxExecutionTime <= 0 {
		if memoryLimit == "256M" || memoryLimit == "512M" || site.Type == "woocommerce" {
			maxExecutionTime = 300
		} else {
			maxExecutionTime = 60
		}
	}

	maxInputVars := site.MaxInputVars
	if maxInputVars <= 0 {
		if memoryLimit == "256M" || memoryLimit == "512M" || site.Type == "woocommerce" {
			maxInputVars = 5000
		} else {
			maxInputVars = 1000
		}
	}

	tmpl, err := template.New("phppool").Parse(phpPoolTemplateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse PHP pool template: %w", err)
	}

	data := struct {
		Domain           string
		SystemUser       string
		PHPVersion       string
		PM               string
		MaxChildren      int
		StartServers     int
		MinSpareServers  int
		MaxSpareServers  int
		OpcacheMemory    int
		OpcacheStrings   int
		MemoryLimit      string
		MaxExecutionTime int
		MaxInputVars     int
	}{
		Domain:           site.Domain,
		SystemUser:       site.SystemUser,
		PHPVersion:       site.PHPVersion,
		PM:               pmMode,
		MaxChildren:      maxChildren,
		StartServers:     startServers,
		MinSpareServers:  minSpare,
		MaxSpareServers:  maxSpare,
		OpcacheMemory:    opcacheMem,
		OpcacheStrings:   opcacheStrings,
		MemoryLimit:      memoryLimit,
		MaxExecutionTime: maxExecutionTime,
		MaxInputVars:     maxInputVars,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
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
