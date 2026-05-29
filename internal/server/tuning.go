package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"agilepanel/internal/config"
)

// TuneServer auto-tunes system swap memory and database parameters based on hardware resources.
func TuneServer() error {
	fmt.Println("AgilePanel: Starting server optimization checklist...")

	// 1. Optimize Swap File
	if err := TuneSwap(); err != nil {
		fmt.Printf("Warning: Swap optimization failed: %v\n", err)
	}

	// 2. Optimize Database (MariaDB/MySQL)
	if err := TuneDatabase(); err != nil {
		fmt.Printf("Warning: Database optimization failed: %v\n", err)
	}

	// 3. Optimize Redis Socket Permissions
	if err := TuneRedis(); err != nil {
		fmt.Printf("Warning: Redis optimization failed: %v\n", err)
	}

	// 4. Setup Default Webserver and Landing Page
	fmt.Println("AgilePanel: Initializing default webserver configuration...")
	if err := SetupDefaultWebserver(); err != nil {
		fmt.Printf("Warning: Default webserver initialization failed: %v\n", err)
	}

	// 5. Install Metrics Cron Job
	if err := installMetricsCron(); err != nil {
		fmt.Printf("Warning: Metrics cron job installation failed: %v\n", err)
	}

	fmt.Println("AgilePanel: Server optimization tuning completed successfully.")

	// Trigger telemetry check-in
	statePath := config.GetStatePath()
	if s, err := config.ReadState(statePath); err == nil {
		config.PingAsync(s)
	}

	return nil
}

// TuneSwap checks swap settings and sets up a 2GB swap file + sysctl tweaks if swap is missing.
func TuneSwap() error {
	if runtime.GOOS != "linux" {
		fmt.Println("Swap (Mock): Configure 2GB swapfile and vm.swappiness=10")
		return nil
	}

	// Check if swap is already active
	swaponCheck := exec.Command("swapon", "--show")
	var stdout bytes.Buffer
	swaponCheck.Stdout = &stdout
	_ = swaponCheck.Run()

	hasSwap := false
	output := stdout.String()
	lines := strings.Split(output, "\n")
	if len(lines) > 1 {
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) != "" {
				hasSwap = true
				break
			}
		}
	}

	if hasSwap {
		fmt.Println("Swap: Swap memory is already active. Skipping swapfile creation.")
	} else {
		fmt.Println("Swap: No swap space detected. Creating 2GB swap file...")
		
		// 1. Allocate file
		falloc := exec.Command("fallocate", "-l", "2G", "/swapfile")
		if err := falloc.Run(); err != nil {
			// fallback to dd if fallocate fails
			dd := exec.Command("dd", "if=/dev/zero", "of=/swapfile", "bs=1M", "count=2048")
			if err := dd.Run(); err != nil {
				return fmt.Errorf("failed to allocate swapfile space: %w", err)
			}
		}
		
		// 2. Set permissions
		if err := exec.Command("chmod", "600", "/swapfile").Run(); err != nil {
			return fmt.Errorf("failed to chmod swapfile: %w", err)
		}
		
		// 3. Make swap
		if err := exec.Command("mkswap", "/swapfile").Run(); err != nil {
			return fmt.Errorf("failed to mkswap swapfile: %w", err)
		}
		
		// 4. Enable swap
		if err := exec.Command("swapon", "/swapfile").Run(); err != nil {
			return fmt.Errorf("failed to enable swapfile: %w", err)
		}

		// 5. Make swap persistent in fstab
		fstabBytes, err := ioutil.ReadFile("/etc/fstab")
		if err == nil && !strings.Contains(string(fstabBytes), "/swapfile") {
			fstabStr := string(fstabBytes)
			if !strings.HasSuffix(fstabStr, "\n") {
				fstabStr += "\n"
			}
			fstabStr += "/swapfile none swap sw 0 0\n"
			_ = ioutil.WriteFile("/etc/fstab", []byte(fstabStr), 0644)
		}
		fmt.Println("Swap: 2GB Swapfile successfully created and activated.")
	}

	// Optimize Sysctl parameters
	fmt.Println("Swap: Tuning kernel swappiness and cache settings...")
	_ = exec.Command("sysctl", "vm.swappiness=10").Run()
	_ = exec.Command("sysctl", "vm.vfs_cache_pressure=50").Run()

	sysctlConfPath := "/etc/sysctl.d/99-agilepanel.conf"
	sysctlContent := "vm.swappiness=10\nvm.vfs_cache_pressure=50\n"
	_ = ioutil.WriteFile(sysctlConfPath, []byte(sysctlContent), 0644)

	return nil
}

// TuneDatabase configures MySQL/MariaDB tuning parameters.
func TuneDatabase() error {
	totalMemKB, _, err := readLinuxMemory()
	
	// Default to 1GB if error or non-linux
	var memorySizeGB float64 = 1.0
	if err == nil && totalMemKB > 0 {
		memorySizeGB = float64(totalMemKB) / (1024 * 1024)
	}

	// Calculate InnoDB Buffer Pool size (approx. 30% of system RAM)
	bufferPoolMB := int(memorySizeGB * 1024 * 0.30)
	if bufferPoolMB < 128 {
		bufferPoolMB = 128 // Minimum standard buffer size
	}
	
	// Log file size is 25% of buffer pool size, capped at 256M
	logFileMB := bufferPoolMB / 4
	if logFileMB < 48 {
		logFileMB = 48
	} else if logFileMB > 256 {
		logFileMB = 256
	}

	cnfContent := fmt.Sprintf(`# AgilePanel Custom MariaDB Optimizations
[mysqld]
innodb_buffer_pool_size = %dM
innodb_log_file_size = %dM
innodb_flush_log_at_trx_commit = 2
innodb_flush_method = O_DIRECT
query_cache_type = 0
query_cache_size = 0
max_connections = 100
`, bufferPoolMB, logFileMB)

	if runtime.GOOS != "linux" {
		fmt.Printf("DB (Mock): Generate tuning configuration:\n%s", cnfContent)
		return nil
	}

	// Paths for MariaDB conf dropins
	cnfDirs := []string{
		"/etc/mysql/mariadb.conf.d",
		"/etc/mysql/conf.d",
	}

	written := false
	for _, dir := range cnfDirs {
		if _, err := os.Stat(dir); err == nil {
			filePath := filepath.Join(dir, "99-agilepanel-tune.cnf")
			if err := ioutil.WriteFile(filePath, []byte(cnfContent), 0644); err == nil {
				fmt.Printf("DB: Database optimization profile written to %s.\n", filePath)
				written = true
				break
			}
		}
	}

	if !written {
		// Fallback to /etc/mysql/conf.d/99-agilepanel-tune.cnf
		filePath := "/etc/mysql/conf.d/99-agilepanel-tune.cnf"
		_ = os.MkdirAll(filepath.Dir(filePath), 0755)
		if err := ioutil.WriteFile(filePath, []byte(cnfContent), 0644); err == nil {
			fmt.Printf("DB: Database optimization profile written to fallback path %s.\n", filePath)
		} else {
			return fmt.Errorf("failed to write database tuning file")
		}
	}

	// Trigger restart of MariaDB to apply
	fmt.Println("DB: Restarting MariaDB service to apply changes...")
	restartCmd := exec.Command("systemctl", "restart", "mariadb")
	if err := restartCmd.Run(); err != nil {
		_ = exec.Command("systemctl", "restart", "mysql").Run()
	}
	return nil
}

// TuneRedis configures Redis socket paths and permissions.
func TuneRedis() error {
	if runtime.GOOS != "linux" {
		fmt.Println("Redis (Mock): Configure UNIX socket in redis.conf")
		return nil
	}

	confPath := "/etc/redis/redis.conf"
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		return nil // Redis not installed or different path
	}

	data, err := ioutil.ReadFile(confPath)
	if err != nil {
		return err
	}

	content := string(data)
	modified := false

	// Enable unixsocket
	if strings.Contains(content, "# unixsocket ") || strings.Contains(content, "#unixsocket ") {
		content = replaceLine(content, "unixsocket ", "unixsocket /var/run/redis/redis-server.sock")
		modified = true
	} else if !strings.Contains(content, "unixsocket /var/run/redis/redis-server.sock") {
		content += "\nunixsocket /var/run/redis/redis-server.sock\n"
		modified = true
	}

	// Enable unixsocketperm 777
	if strings.Contains(content, "# unixsocketperm ") || strings.Contains(content, "#unixsocketperm ") {
		content = replaceLine(content, "unixsocketperm ", "unixsocketperm 777")
		modified = true
	} else if !strings.Contains(content, "unixsocketperm 777") {
		content += "\nunixsocketperm 777\n"
		modified = true
	}

	if modified {
		if err := ioutil.WriteFile(confPath, []byte(content), 0644); err != nil {
			return err
		}
		fmt.Println("Redis: UNIX socket enabled with permissions 777 in redis.conf.")
		
		// Restart service
		_ = exec.Command("systemctl", "restart", "redis-server").Run()
		_ = exec.Command("systemctl", "restart", "redis").Run()
	}

	return nil
}

func replaceLine(content string, prefix string, newLine string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			uncommented := strings.TrimSpace(trimmed[1:])
			if strings.HasPrefix(uncommented, prefix) {
				lines[i] = newLine
			}
		}
	}
	return strings.Join(lines, "\n")
}

const welcomeHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Welcome to AgilePanel</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-color: #030712;
            --card-bg: rgba(255, 255, 255, 0.03);
            --card-border: rgba(255, 255, 255, 0.08);
            --text-primary: #f3f4f6;
            --text-secondary: #9ca3af;
            --accent-gradient: linear-gradient(135deg, #a78bfa, #3b82f6, #06b6d4);
            --card-hover-bg: rgba(255, 255, 255, 0.06);
            --card-hover-border: rgba(255, 255, 255, 0.15);
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: 'Outfit', sans-serif;
            background-color: var(--bg-color);
            color: var(--text-primary);
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            justify-content: center;
            align-items: center;
            overflow-x: hidden;
            position: relative;
            padding: 2rem 1rem;
        }

        /* Subtle glowing background blobs */
        body::before, body::after {
            content: '';
            position: absolute;
            width: 400px;
            height: 400px;
            border-radius: 50%;
            background: var(--accent-gradient);
            filter: blur(150px);
            opacity: 0.15;
            z-index: 0;
            pointer-events: none;
        }

        body::before {
            top: -100px;
            left: -100px;
        }

        body::after {
            bottom: -100px;
            right: -100px;
        }

        .container {
            max-width: 900px;
            width: 100%;
            z-index: 1;
            text-align: center;
            animation: fadeIn 1s ease-out;
        }

        header {
            margin-bottom: 3.5rem;
        }

        .logo-container {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            margin-bottom: 1.5rem;
            position: relative;
        }

        .logo-icon {
            font-size: 3rem;
            background: var(--accent-gradient);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            font-weight: 700;
            letter-spacing: -1px;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }

        .logo-badge {
            background: rgba(59, 130, 246, 0.15);
            border: 1px solid rgba(59, 130, 246, 0.3);
            color: #60a5fa;
            font-size: 0.75rem;
            padding: 0.2rem 0.6rem;
            border-radius: 50px;
            margin-left: 0.75rem;
            font-weight: 500;
            letter-spacing: 0.5px;
            text-transform: uppercase;
        }

        h1 {
            font-size: 2.75rem;
            font-weight: 600;
            line-height: 1.2;
            margin-bottom: 1rem;
            background: linear-gradient(to right, #ffffff, #d1d5db);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }

        .subtitle {
            font-size: 1.15rem;
            color: var(--text-secondary);
            max-width: 600px;
            margin: 0 auto;
            font-weight: 300;
            line-height: 1.6;
        }

        /* Glassmorphism main card */
        .main-card {
            background: var(--card-bg);
            backdrop-filter: blur(16px);
            -webkit-backdrop-filter: blur(16px);
            border: 1px solid var(--card-border);
            border-radius: 24px;
            padding: 3rem;
            margin-bottom: 3rem;
            box-shadow: 0 20px 50px rgba(0, 0, 0, 0.3);
            position: relative;
            overflow: hidden;
        }

        .main-card::before {
            content: '';
            position: absolute;
            top: 0;
            left: 0;
            width: 100%;
            height: 4px;
            background: var(--accent-gradient);
        }

        .status-badge {
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
            background: rgba(16, 185, 129, 0.1);
            border: 1px solid rgba(16, 185, 129, 0.2);
            color: #34d399;
            padding: 0.5rem 1rem;
            border-radius: 50px;
            font-weight: 500;
            font-size: 0.9rem;
            margin-bottom: 2rem;
            animation: pulse 2s infinite;
        }

        .status-dot {
            width: 8px;
            height: 8px;
            background-color: #10b981;
            border-radius: 50%;
        }

        /* Cards grid */
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 1.5rem;
            margin-top: 1rem;
        }

        .step-card {
            background: rgba(255, 255, 255, 0.01);
            border: 1px solid var(--card-border);
            border-radius: 16px;
            padding: 2rem 1.5rem;
            text-align: left;
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
            position: relative;
            cursor: default;
        }

        .step-card:hover {
            background: var(--card-hover-bg);
            border-color: var(--card-hover-border);
            transform: translateY(-5px);
            box-shadow: 0 10px 25px rgba(0, 0, 0, 0.2);
        }

        .step-icon {
            width: 48px;
            height: 48px;
            border-radius: 12px;
            background: rgba(255, 255, 255, 0.03);
            display: flex;
            align-items: center;
            justify-content: center;
            margin-bottom: 1.25rem;
            font-size: 1.5rem;
            color: #60a5fa;
            border: 1px solid rgba(255, 255, 255, 0.05);
            transition: all 0.3s ease;
        }

        .step-card:hover .step-icon {
            background: var(--accent-gradient);
            color: #ffffff;
            border-color: transparent;
            box-shadow: 0 0 15px rgba(59, 130, 246, 0.4);
        }

        .step-card h3 {
            font-size: 1.2rem;
            font-weight: 500;
            margin-bottom: 0.5rem;
            color: #ffffff;
        }

        .step-card p {
            font-size: 0.9rem;
            color: var(--text-secondary);
            line-height: 1.5;
            font-weight: 300;
        }

        .code-block {
            background: #090d16;
            border: 1px solid rgba(255, 255, 255, 0.05);
            border-radius: 8px;
            padding: 0.75rem 1rem;
            font-family: 'Courier New', Courier, monospace;
            font-size: 0.85rem;
            color: #38bdf8;
            margin-top: 1rem;
            display: flex;
            justify-content: space-between;
            align-items: center;
            position: relative;
        }

        footer {
            margin-top: auto;
            color: #4b5563;
            font-size: 0.85rem;
            font-weight: 300;
        }

        footer a {
            color: #6b7280;
            text-decoration: none;
            transition: color 0.2s;
        }

        footer a:hover {
            color: #9ca3af;
        }

        /* Animations */
        @keyframes fadeIn {
            from {
                opacity: 0;
                transform: translateY(20px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }

        @keyframes pulse {
            0% {
                box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.4);
            }
            70% {
                box-shadow: 0 0 0 10px rgba(16, 185, 129, 0);
            }
            100% {
                box-shadow: 0 0 0 0 rgba(16, 185, 129, 0);
            }
        }

        @keyframes float {
            0% {
                transform: translateY(0px);
            }
            50% {
                transform: translateY(-10px);
            }
            100% {
                transform: translateY(0px);
            }
        }

        .floating {
            animation: float 6s ease-in-out infinite;
        }

        @media (max-width: 640px) {
            h1 {
                font-size: 2rem;
            }
            .main-card {
                padding: 1.5rem;
            }
            .grid {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div class="logo-container floating">
                <span class="logo-icon">▲ AgilePanel</span>
                <span class="logo-badge">v1.0.0</span>
            </div>
            <h1>Web Stack Successfully Initialized</h1>
            <p class="subtitle">Your High-Performance VPS is fully optimized and ready to serve next-generation web experiences.</p>
        </header>

        <main class="main-card">
            <div class="status-badge">
                <span class="status-dot"></span>
                <span>Server Stack Online & Tuned</span>
            </div>

            <div class="grid">
                <!-- Card 1 -->
                <div class="step-card">
                    <div class="step-icon">✦</div>
                    <h3>Create a Website</h3>
                    <p>Deploy a new lightning-fast WordPress site with Redis cache & full PHP-FPM optimization.</p>
                    <div class="code-block">
                        <span>ap site create example.com --wp</span>
                    </div>
                </div>

                <!-- Card 2 -->
                <div class="step-card">
                    <div class="step-icon">⚙</div>
                    <h3>Monitor Performance</h3>
                    <p>Check active services, real-time memory stats, and disk utilization instantly.</p>
                    <div class="code-block">
                        <span>ap server status</span>
                    </div>
                </div>

                <!-- Card 3 -->
                <div class="step-card">
                    <div class="step-icon">⚡</div>
                    <h3>Tuned to Perfection</h3>
                    <p>Redis sockets, OPcache, MariaDB buffers, and swap parameters are already configured.</p>
                    <div class="code-block">
                        <span>ap server tune</span>
                    </div>
                </div>
            </div>
        </main>

        <footer>
            <p>Powered by <a href="https://github.com/webtechj/agilepanel" target="_blank">AgilePanel</a> • High Performance VPS Hosting</p>
        </footer>
    </div>
</body>
</html>
`

func writeDefaultWelcomePage() error {
	var welcomeDir string
	if runtime.GOOS == "windows" {
		// Use a local path for testing on Windows
		welcomeDir = filepath.Join(os.TempDir(), "agilepanel-default")
	} else {
		welcomeDir = "/var/www/default"
	}

	if err := os.MkdirAll(welcomeDir, 0755); err != nil {
		return fmt.Errorf("failed to create default welcome page directory: %w", err)
	}

	welcomeFile := filepath.Join(welcomeDir, "index.html")
	if err := os.WriteFile(welcomeFile, []byte(welcomeHTML), 0644); err != nil {
		return fmt.Errorf("failed to write index.html to %s: %w", welcomeFile, err)
	}

	fmt.Printf("Default Page: Welcome page written to %s successfully.\n", welcomeFile)
	return nil
}

// SetupDefaultWebserver initializes the default welcome page and Caddy catch-all configuration.
func SetupDefaultWebserver() error {
	// 1. Write the default welcome page
	if err := writeDefaultWelcomePage(); err != nil {
		return fmt.Errorf("failed to write default welcome page: %w", err)
	}

	// 2. Read and lock the state to write Caddy config and reload
	statePath := config.GetStatePath()
	err := config.WithLockedState(statePath, func(s *config.State) error {
		// 3. Write Caddyfile (contains :80 welcome block and any configured sites)
		if err := WriteCaddyfile(s); err != nil {
			return fmt.Errorf("failed to write Caddyfile: %w", err)
		}
		// 4. Reload Caddy
		if err := ReloadCaddy(s); err != nil {
			return fmt.Errorf("failed to reload Caddy: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func installMetricsCron() error {
	if runtime.GOOS != "linux" {
		fmt.Println("Cron (Mock): Write metrics cron job to /etc/cron.d/agilepanel-metrics")
		return nil
	}

	cronPath := "/etc/cron.d/agilepanel-metrics"
	cronContent := "*/5 * * * * root /usr/local/bin/ap server log-metrics >/dev/null 2>&1\n"

	err := os.WriteFile(cronPath, []byte(cronContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write metrics cron job: %w", err)
	}

	fmt.Println("Cron: Metrics recording job installed to /etc/cron.d/agilepanel-metrics.")
	return nil
}
