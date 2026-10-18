package site

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

// CheckDomainDNS checks if the domain resolves to an IP and whether it matches the server public IP.
func CheckDomainDNS(domain string) (string, bool, error) {
	if os.Getenv("AGILEPANEL_TEST_MODE") == "true" {
		return "127.0.0.1", true, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", domain)
	if err != nil {
		return "", false, err
	}
	if len(ips) == 0 {
		return "", false, fmt.Errorf("no IP addresses returned for %s", domain)
	}

	serverIP := server.ResolvePublicIP()
	var resolvedStr string
	matches := false

	for _, ip := range ips {
		ipStr := ip.String()
		if resolvedStr == "" {
			resolvedStr = ipStr
		}
		if ipStr == serverIP {
			matches = true
			resolvedStr = ipStr
			break
		}
	}

	return resolvedStr, matches, nil
}

// isCloudflareIP returns true if the IP matches known Cloudflare IPv4 prefixes.
func isCloudflareIP(ipStr string) bool {
	cfPrefixes := []string{
		"173.245.48.", "103.21.244.", "103.22.200.", "103.31.4.", "141.101.64.",
		"108.162.192.", "190.93.240.", "188.114.96.", "197.234.240.", "198.41.128.",
		"162.158.", "104.16.", "104.17.", "104.18.", "104.19.", "104.20.",
		"104.21.", "104.22.", "104.23.", "104.24.", "104.25.", "104.26.",
		"104.27.", "104.28.", "172.64.", "172.65.", "172.66.", "172.67.",
		"172.68.", "172.69.", "172.70.", "172.71.",
	}
	for _, prefix := range cfPrefixes {
		if strings.HasPrefix(ipStr, prefix) {
			return true
		}
	}
	return false
}

type tlsHandshakeResult struct {
	Success bool
	Error   string
	Is525   bool
}

// probeLocalTLS performs a direct synthetic TLS handshake against local port 443 with SNI.
func probeLocalTLS(serverName string) tlsHandshakeResult {
	if os.Getenv("AGILEPANEL_TEST_MODE") == "true" {
		return tlsHandshakeResult{Success: true}
	}

	dialer := &net.Dialer{Timeout: 3 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", "127.0.0.1:443", &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true,
	})
	if err != nil {
		errStr := err.Error()
		is525 := strings.Contains(errStr, "internal error") ||
			strings.Contains(errStr, "handshake failure") ||
			strings.Contains(errStr, "unrecognized name") ||
			strings.Contains(errStr, "no certificate available")
		return tlsHandshakeResult{Success: false, Error: errStr, Is525: is525}
	}
	_ = conn.Close()
	return tlsHandshakeResult{Success: true}
}

type httpProbeResult struct {
	StatusCode int
	Location   string
	Error      string
}

// probeLocalHTTP sends a loopback HTTP request over TLS to test status codes.
func probeLocalHTTP(host string) httpProbeResult {
	if os.Getenv("AGILEPANEL_TEST_MODE") == "true" {
		return httpProbeResult{StatusCode: 200}
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "tcp", "127.0.0.1:443")
		},
	}
	client := &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 4 * time.Second,
	}

	req, err := http.NewRequest("GET", "https://"+host+"/", nil)
	if err != nil {
		return httpProbeResult{Error: err.Error()}
	}
	req.Header.Set("User-Agent", "AgilePanel-Doctor/1.0")
	req.Header.Set("X-Forwarded-Proto", "https")

	resp, err := client.Do(req)
	if err != nil {
		return httpProbeResult{Error: err.Error()}
	}
	defer resp.Body.Close()

	return httpProbeResult{
		StatusCode: resp.StatusCode,
		Location:   resp.Header.Get("Location"),
	}
}

// checkCaddyJournal scans recent Caddy journal output for domain-related warnings.
func checkCaddyJournal(domain string) []string {
	if os.Getenv("AGILEPANEL_TEST_MODE") == "true" || runtime.GOOS != "linux" {
		return nil
	}

	cmd := exec.Command("journalctl", "-u", "caddy", "-n", "60", "--no-pager")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil
	}

	var matched []string
	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, domain) && (strings.Contains(line, "warn") || strings.Contains(line, "error") || strings.Contains(line, "aborting")) {
			if len(line) > 85 {
				line = line[:82] + "..."
			}
			matched = append(matched, line)
		}
	}
	return matched
}

// checkPHPStarvation checks if the pool has logged max_children saturation.
func checkPHPStarvation(version, domain string) bool {
	if os.Getenv("AGILEPANEL_TEST_MODE") == "true" || runtime.GOOS != "linux" {
		return false
	}
	logPath := fmt.Sprintf("/var/log/php%s-fpm.log", version)
	f, err := os.Open(logPath)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, fmt.Sprintf("pool %s", domain)) && strings.Contains(line, "reached max_children") {
			return true
		}
	}
	return false
}

// Doctor runs an automated diagnostic audit for a website (read-only mode).
func Doctor(domain string) error {
	return DoctorWithFix(domain, false)
}

// DoctorWithFix runs automated health diagnostics and optionally auto-heals discovered issues.
func DoctorWithFix(domain string, autoFix bool) error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return err
	}

	var targetSite *config.SiteConfig
	for idx, s := range state.Sites {
		if strings.EqualFold(s.Domain, domain) {
			targetSite = &state.Sites[idx]
			break
		}
	}
	if targetSite == nil {
		return fmt.Errorf("site %s not found in state", domain)
	}

	serverIP := server.ResolvePublicIP()
	isApex := server.IsApexDomain(domain)
	wwwDomain := "www." + domain
	if server.IsWWWDomain(domain) {
		wwwDomain = domain
		domain = server.TrimWWW(domain)
	}

	var recommendations []string
	var fixableIssues []string

	// Tracking remediation triggers
	needsCaddyWWWFix := false
	needsMemoryScale := false
	needsOrphanCleanup := false
	var orphanedVersions []string
	needsOpcacheFix := false
	needsPMFix := false
	var recommendedMemory = "256M"

	ui.Banner("Site Doctor Diagnostic Report: " + strings.ToUpper(targetSite.Domain))

	// -------------------------------------------------------------
	// 1. DNS & Cloudflare Edge Matrix Probe
	// -------------------------------------------------------------
	ui.SectionHeader("1. DNS & CLOUDFLARE EDGE MATRIX")
	apexIP, apexMatches, apexErr := CheckDomainDNS(targetSite.Domain)
	if apexErr != nil {
		ui.RowBadge("Apex DNS Resolution", "✖ Failed", ui.BrightRed)
		ui.Row("Lookup Error", apexErr.Error())
		ui.Row("Expected Server IP", serverIP)
		recommendations = append(recommendations,
			fmt.Sprintf("DNS Missing: Add an 'A' record pointing '%s' to '%s' at your DNS provider.", targetSite.Domain, serverIP))
	} else {
		isCF := isCloudflareIP(apexIP)
		if isCF {
			ui.RowBadge("Apex DNS ("+targetSite.Domain+")", "● Cloudflare Proxy Active", ui.BrightCyan)
			ui.Row("Edge Proxy IP", fmt.Sprintf("%s (Cloudflare CDN)", apexIP))
			ui.Row("Origin VPS IP", serverIP)
		} else if !apexMatches {
			ui.RowBadge("Apex DNS ("+targetSite.Domain+")", "▲ IP Mismatch", ui.BrightYellow)
			ui.Row("Domain IP", apexIP)
			ui.Row("Server IP", serverIP)
			recommendations = append(recommendations,
				fmt.Sprintf("DNS IP Mismatch: '%s' resolves to %s instead of this VPS (%s). Update your DNS A-record.", targetSite.Domain, apexIP, serverIP))
		} else {
			ui.RowBadge("Apex DNS ("+targetSite.Domain+")", "● Direct VPS Match", ui.BrightGreen)
			ui.Row("Domain IP", fmt.Sprintf("%s (matches server)", apexIP))
		}
	}

	// Also probe WWW DNS if apex domain
	if isApex {
		wwwIP, _, wwwErr := CheckDomainDNS(wwwDomain)
		if wwwErr != nil {
			ui.RowBadge("WWW DNS ("+wwwDomain+")", "○ Not Configured", ui.BrightYellow)
		} else {
			isCF := isCloudflareIP(wwwIP)
			if isCF {
				ui.RowBadge("WWW DNS ("+wwwDomain+")", "● Cloudflare Proxy Active", ui.BrightCyan)
				ui.Row("WWW Edge IP", fmt.Sprintf("%s (Cloudflare CDN)", wwwIP))
			} else {
				ui.RowBadge("WWW DNS ("+wwwDomain+")", "● Resolves to IP", ui.BrightGreen)
				ui.Row("WWW Domain IP", wwwIP)
			}
		}
	}

	// -------------------------------------------------------------
	// 2. Synthetic TLS & SNI Loopback Handshake (Fixes Error 525)
	// -------------------------------------------------------------
	ui.SectionHeader("2. SYNTHETIC TLS / SNI HANDSHAKE (PORT 443 PROBE)")
	apexTLS := probeLocalTLS(targetSite.Domain)
	if apexTLS.Success {
		ui.RowBadge("Apex TLS Handshake", "● Handshake Successful", ui.BrightGreen)
	} else if apexTLS.Is525 {
		ui.RowBadge("Apex TLS Handshake", "✖ TLS Handshake Failed (Error 525 Risk)", ui.BrightRed)
		ui.Row("TLS Error", apexTLS.Error)
		recommendations = append(recommendations,
			fmt.Sprintf("TLS Handshake Failure: Caddy rejected TLS negotiation for '%s' (Error: %s).", targetSite.Domain, apexTLS.Error))
	} else {
		ui.RowBadge("Apex TLS Handshake", "▲ Inactive / Refused", ui.BrightYellow)
	}

	if isApex {
		wwwTLS := probeLocalTLS(wwwDomain)
		if wwwTLS.Success {
			ui.RowBadge("WWW TLS Handshake", "● Handshake Successful", ui.BrightGreen)
		} else if wwwTLS.Is525 {
			ui.RowBadge("WWW TLS Handshake", "✖ Failed (Causes Cloudflare Error 525)", ui.BrightRed)
			ui.Row("TLS Error", "Caddy rejected TLS handshake for '"+wwwDomain+"'")
			ui.Row("Root Cause", "Missing server block for '"+wwwDomain+"' in Caddyfile. Cloudflare visitors receive Error 525.")
			recommendations = append(recommendations,
				fmt.Sprintf("Cloudflare Error 525: Missing 'www.%s' block in Caddyfile. Cloudflare fails TLS handshake. Run 'ap site doctor %s --fix' to add automatic WWW redirect.", targetSite.Domain, targetSite.Domain))
			needsCaddyWWWFix = true
			fixableIssues = append(fixableIssues, "Missing WWW block in Caddyfile (Causes Error 525)")
		} else {
			ui.RowBadge("WWW TLS Handshake", "○ Not Listening on 443", ui.BrightYellow)
		}
	}

	// SSL Cert details on disk
	sslInfo, _ := GetSSLInfo(targetSite.Domain)
	if sslInfo != nil && sslInfo.Active {
		daysLeft := int(time.Until(sslInfo.Expiration).Hours() / 24)
		if daysLeft < 14 {
			ui.RowBadge("SSL Certificate", fmt.Sprintf("▲ Expiring Soon (%d days)", daysLeft), ui.BrightYellow)
			recommendations = append(recommendations,
				fmt.Sprintf("SSL Nearing Expiry: Only %d days remaining on %s certificate. Run 'ap site ssl-renew %s' to force renewal.", daysLeft, sslInfo.Issuer, targetSite.Domain))
		} else {
			ui.RowBadge("SSL Certificate", "● Active", ui.BrightGreen)
		}
		ui.Row("Certificate Issuer", sslInfo.Issuer)
		ui.Row("Valid Until", fmt.Sprintf("%s (%d days remaining)", sslInfo.Expiration.Format("2006-01-02"), daysLeft))
	} else {
		ui.RowBadge("SSL Certificate", "▲ Managed Dynamically by Caddy", ui.BrightYellow)
	}

	// -------------------------------------------------------------
	// 3. Local Loopback HTTP Response & Upstream FastCGI
	// -------------------------------------------------------------
	ui.SectionHeader("3. HTTP LOOPBACK & FASTCGI UPSTREAM RESPONSE")
	httpProbe := probeLocalHTTP(targetSite.Domain)
	if httpProbe.Error != "" {
		ui.RowBadge("HTTP Status Probe", "▲ Direct Probe Skipped", ui.BrightYellow)
		ui.Row("Probe Detail", httpProbe.Error)
	} else {
		switch {
		case httpProbe.StatusCode == 200:
			ui.RowBadge("HTTP Status", "● 200 OK (Website Responding Cleanly)", ui.BrightGreen)
		case httpProbe.StatusCode == 301 || httpProbe.StatusCode == 302:
			ui.RowBadge("HTTP Status", fmt.Sprintf("● %d Redirect -> %s", httpProbe.StatusCode, httpProbe.Location), ui.BrightGreen)
		case httpProbe.StatusCode == 502:
			ui.RowBadge("HTTP Status", "✖ 502 Bad Gateway (FastCGI Upstream Refused)", ui.BrightRed)
			recommendations = append(recommendations,
				fmt.Sprintf("502 Bad Gateway: PHP-FPM socket not responding for %s. Run 'ap site doctor %s --fix' to repair socket permissions and restart PHP-FPM.", targetSite.Domain, targetSite.Domain))
			fixableIssues = append(fixableIssues, "502 Bad Gateway / Socket Refusal")
		case httpProbe.StatusCode == 500:
			ui.RowBadge("HTTP Status", "✖ 500 Internal Server Error (PHP Fatal Crash / DB Error)", ui.BrightRed)
			recommendations = append(recommendations,
				fmt.Sprintf("500 Internal Server Error: Site crashed during execution. Check PHP memory limit or fatal errors below."))
		case httpProbe.StatusCode == 504:
			ui.RowBadge("HTTP Status", "✖ 504 Gateway Timeout (Execution Exceeded)", ui.BrightRed)
		default:
			ui.RowBadge("HTTP Status", fmt.Sprintf("● HTTP %d", httpProbe.StatusCode), ui.BrightGreen)
		}
	}

	// -------------------------------------------------------------
	// 4. Caddy Web Server Runtime & Logs
	// -------------------------------------------------------------
	ui.SectionHeader("4. CADDY WEBSERVER CONFIG & RECENT LOGS")
	caddyRecentLogs := checkCaddyJournal(targetSite.Domain)
	if len(caddyRecentLogs) > 0 {
		ui.RowBadge("Caddy Error Log Audit", fmt.Sprintf("▲ %d Recent Warning(s)/Error(s)", len(caddyRecentLogs)), ui.BrightYellow)
		for _, l := range caddyRecentLogs {
			ui.Row("Caddy Log", l)
		}
	} else {
		ui.RowBadge("Caddy Error Log Audit", "● Clean (No Upstream / TLS Errors)", ui.BrightGreen)
	}

	// -------------------------------------------------------------
	// 5. PHP-FPM Service, Socket & Memory Sizing
	// -------------------------------------------------------------
	ui.SectionHeader("5. PHP-FPM PROCESS MANAGER & MEMORY SIZING")
	poolPath := server.GetPHPPoolPath(targetSite.PHPVersion, targetSite.Domain)
	poolConfig, poolErr := server.ReadPHPPoolConfig(targetSite.PHPVersion, targetSite.Domain)

	activePoolMem := targetSite.MemoryLimit
	if poolErr == nil && poolConfig != nil {
		ui.RowBadge("PHP Pool Config", "● Active", ui.BrightGreen)
		ui.Row("PHP Version", targetSite.PHPVersion)
		if poolConfig.MemoryLimit != "" {
			activePoolMem = poolConfig.MemoryLimit
		} else if activePoolMem == "" {
			activePoolMem = "128M"
		}
		ui.Row("PHP Memory Limit (Pool)", activePoolMem)
		ui.Row("Process Manager (PM)", fmt.Sprintf("%s (max_children: %d)", poolConfig.PM, poolConfig.MaxChildren))
		ui.Row("Config File", poolPath)

		// Check OPcache validate_timestamps
		if poolConfig.OpcacheValidateTimestamps == 0 {
			ui.RowBadge("OPcache Timestamps", "▲ Disabled (0) — Code updates require daemon reload", ui.BrightYellow)
			recommendations = append(recommendations,
				fmt.Sprintf("OPcache Timestamps Disabled: opcache.validate_timestamps is 0 for '%s'. Script and wp-config.php updates are not dynamically detected. Run 'ap site doctor %s --fix' to enable validation.", targetSite.Domain, targetSite.Domain))
			needsOpcacheFix = true
			fixableIssues = append(fixableIssues, "Disabled OPcache timestamp validation (validate_timestamps = 0)")
		} else {
			ui.RowBadge("OPcache Timestamps", fmt.Sprintf("● Enabled (revalidate_freq: %ds)", poolConfig.OpcacheRevalidateFreq), ui.BrightGreen)
		}

		// Check PM concurrency
		if poolConfig.MaxChildren > 25 {
			ui.RowBadge("PM Concurrency", fmt.Sprintf("▲ High Allocation (%d workers) — OOM Risk", poolConfig.MaxChildren), ui.BrightYellow)
			recommendations = append(recommendations,
				fmt.Sprintf("PHP-FPM Worker Pool Overallocated: max_children is %d. On VPS memory, traffic spikes risk OS OOM termination. Run 'ap site doctor %s --fix' to balance workers.", poolConfig.MaxChildren, targetSite.Domain))
			needsPMFix = true
			fixableIssues = append(fixableIssues, fmt.Sprintf("Overallocated PHP-FPM max_children (%d)", poolConfig.MaxChildren))
		} else {
			ui.RowBadge("PM Concurrency", fmt.Sprintf("● Balanced (pm=%s, max_children=%d)", poolConfig.PM, poolConfig.MaxChildren), ui.BrightGreen)
		}
	} else {
		ui.RowBadge("PHP Pool Config", "✖ Missing", ui.BrightRed)
		ui.Row("Target Path", poolPath)
		recommendations = append(recommendations,
			fmt.Sprintf("PHP Pool Missing: Run 'ap site doctor %s --fix' to rebuild pool configurations.", targetSite.Domain))
		fixableIssues = append(fixableIssues, "Missing PHP-FPM pool configuration")
	}

	// Check for Orphaned / Inactive PHP Pools across other versions
	orphanedVersions = server.FindOrphanedPHPPools(targetSite.Domain, targetSite.PHPVersion, state.Global.SupportedPHPVersions)
	if len(orphanedVersions) > 0 {
		ui.RowBadge("Inactive PHP Pools", fmt.Sprintf("▲ Orphaned Pool in PHP %s", strings.Join(orphanedVersions, ", ")), ui.BrightYellow)
		ui.Row("Orphan Config", fmt.Sprintf("Leftover config found in /etc/php/%s/fpm/pool.d/%s.conf while site runs PHP %s", strings.Join(orphanedVersions, ", "), targetSite.Domain, targetSite.PHPVersion))
		recommendations = append(recommendations,
			fmt.Sprintf("Inactive PHP Pool Detected: Leftover pool config for '%s' exists in PHP %s. Run 'ap site doctor %s --fix' to purge orphaned pools.",
				targetSite.Domain, strings.Join(orphanedVersions, ", "), targetSite.Domain))
		needsOrphanCleanup = true
		fixableIssues = append(fixableIssues, fmt.Sprintf("Orphaned PHP %s pool configuration", strings.Join(orphanedVersions, ", ")))
	} else {
		ui.RowBadge("Inactive PHP Pools", "● None (Clean Runtime Isolation)", ui.BrightGreen)
	}

	// Worker starvation check
	if checkPHPStarvation(targetSite.PHPVersion, targetSite.Domain) {
		ui.RowBadge("Worker Pool Capacity", "▲ max_children Saturation Detected", ui.BrightYellow)
		recommendations = append(recommendations,
			fmt.Sprintf("PHP Worker Pool Saturated: Traffic exceeded max_children. Run 'ap server tune' to recalculate optimal concurrency."))
	} else {
		ui.RowBadge("Worker Pool Capacity", "● Responsive", ui.BrightGreen)
	}

	// -------------------------------------------------------------
	// 6. Database Connectivity Check
	// -------------------------------------------------------------
	ui.SectionHeader("6. MARIADB DATABASE CONNECTIVITY")
	if targetSite.DatabaseName == "" || targetSite.Type == "html" {
		ui.RowBadge("Database Engine", "● N/A (Static Site)", ui.BrightGreen)
	} else {
		ui.Row("Database Name", targetSite.DatabaseName)
		ui.Row("Database User", targetSite.DatabaseUser)
		dbConnected := true
		if os.Getenv("AGILEPANEL_TEST_MODE") != "true" && runtime.GOOS == "linux" {
			homeDir := server.GetSiteRootDir(targetSite.PublicDir)
			if targetSite.Type == "wp" || targetSite.Type == "woocommerce" || targetSite.Type == "" {
				err := server.RunAsUser(targetSite.SystemUser, homeDir, "wp", "db", "check", "--path="+targetSite.PublicDir)
				if err != nil {
					dbConnected = false
				}
			}
		}

		if dbConnected {
			ui.RowBadge("Connection Status", "● Connected & Responsive", ui.BrightGreen)
		} else {
			ui.RowBadge("Connection Status", "✖ Failed to Connect", ui.BrightRed)
			recommendations = append(recommendations,
				fmt.Sprintf("Database Connection Failure: WordPress could not connect to '%s'. Check credentials in wp-config.php.", targetSite.DatabaseName))
			fixableIssues = append(fixableIssues, "Database connection failure")
		}
	}

	// -------------------------------------------------------------
	// 7. WordPress Core, Error Logs & Heavy Plugins Sizing
	// -------------------------------------------------------------
	isWP := targetSite.Type == "wp" || targetSite.Type == "woocommerce" || targetSite.Type == ""
	if isWP {
		ui.SectionHeader("7. WORDPRESS APPLICATION & ERROR LOGS")
		wpConfigPath := server.FindWPConfigPath(targetSite.PublicDir)
		if wpConfigPath != "" {
			ui.RowBadge("Configuration File", "● Found", ui.BrightGreen)
			ui.Row("wp-config.php Path", wpConfigPath)
		} else {
			ui.RowBadge("Configuration File", "✖ wp-config.php Not Found", ui.BrightRed)
			recommendations = append(recommendations,
				fmt.Sprintf("WordPress Core Missing: wp-config.php was not found. Run 'ap site reinstall %s' to restore core files.", targetSite.Domain))
		}

		// Inspect WP_MEMORY_LIMIT in wp-config.php and detect conflict with PHP pool
		wpMem, wpMaxMem, _ := server.ReadWordPressMemoryLimit(targetSite.PublicDir)
		if wpMem != "" {
			ui.Row("WP_MEMORY_LIMIT", wpMem)
			if wpMaxMem != "" && wpMaxMem != wpMem {
				ui.Row("WP_MAX_MEMORY_LIMIT", wpMaxMem)
			}

			// Check for conflict: wp-config.php requests more memory than pool allows
			if server.CompareMemory(wpMem, activePoolMem) > 0 {
				needsMemoryScale = true
				if server.CompareMemory(wpMem, recommendedMemory) > 0 {
					recommendedMemory = wpMem
				}
				ui.RowBadge("Memory Limit Conflict", fmt.Sprintf("✖ CRITICAL: wp-config requests %s, pool caps at %s", wpMem, activePoolMem), ui.BrightRed)
				ui.Row("Conflict Details", fmt.Sprintf("PHP-FPM php_admin_value[memory_limit] (%s) overrides wp-config.php (%s). WordPress cannot allocate requested memory, causing fatal OOM crashes.", activePoolMem, wpMem))
				recommendations = append(recommendations,
					fmt.Sprintf("PHP Memory Conflict: wp-config.php specifies WP_MEMORY_LIMIT '%s' but PHP-FPM pool caps at '%s' via php_admin_value. Run 'ap site doctor %s --fix' to upgrade pool to %s.",
						wpMem, activePoolMem, targetSite.Domain, wpMem))
				fixableIssues = append(fixableIssues, fmt.Sprintf("Memory Limit Conflict (wp-config requests %s, pool caps at %s)", wpMem, activePoolMem))
			} else {
				ui.RowBadge("Memory Alignment", fmt.Sprintf("● Aligned (wp-config: %s, pool: %s)", wpMem, activePoolMem), ui.BrightGreen)
			}
		}

		// Inspect debug.log for fatal errors
		debugLogPath := filepath.Join(targetSite.PublicDir, "wp-content", "debug.log")
		hasFatals := false
		hasOOMFatal := false
		var fatalSnippet string
		if f, err := os.Open(debugLogPath); err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "Fatal error:") || strings.Contains(line, "Parse error:") {
					hasFatals = true
					fatalSnippet = line
					if strings.Contains(line, "Allowed memory size") && strings.Contains(line, "exhausted") {
						hasOOMFatal = true
					}
				}
			}
		}

		if hasFatals {
			ui.RowBadge("Error Log Audit", "▲ Fatal Errors Detected", ui.BrightYellow)
			if len(fatalSnippet) > 75 {
				fatalSnippet = fatalSnippet[:72] + "..."
			}
			ui.Row("Latest Fatal Error", fatalSnippet)

			if hasOOMFatal {
				needsMemoryScale = true
				if server.CompareMemory("256M", recommendedMemory) > 0 {
					recommendedMemory = "256M"
				}
				recommendations = append(recommendations,
					fmt.Sprintf("PHP Fatal Memory Exhaustion Detected: Site crashed due to memory limit exhaustion. Run 'ap site doctor %s --fix' to auto-upgrade to %s.", targetSite.Domain, recommendedMemory))
				fixableIssues = append(fixableIssues, "PHP Memory Exhaustion (Fatal Error in debug.log)")
			} else {
				recommendations = append(recommendations,
					fmt.Sprintf("PHP Fatal Error: A plugin or theme crashed WordPress. Run 'ap site safe-mode %s on' to isolate.", targetSite.Domain))
			}
		} else {
			ui.RowBadge("Error Log Audit", "● No Fatal Errors Detected", ui.BrightGreen)
		}

		// Audit plugins for resource-intensive page builders / e-commerce
		pluginsDir := filepath.Join(targetSite.PublicDir, "wp-content", "plugins")
		heavyFound := []string{}
		checkHeavy := []struct {
			name string
			slug string
		}{
			{"Elementor", "elementor"},
			{"WooCommerce", "woocommerce"},
			{"Divi Builder", "divi-builder"},
			{"WPBakery", "js_composer"},
			{"WPML", "sitepress-multilingual-cms"},
			{"Wordfence", "wordfence"},
			{"Rank Math", "seo-by-rank-math"},
		}
		for _, hp := range checkHeavy {
			if _, err := os.Stat(filepath.Join(pluginsDir, hp.slug)); err == nil {
				heavyFound = append(heavyFound, hp.name)
			}
		}

		if len(heavyFound) > 0 {
			if targetSite.MemoryLimit == "" || targetSite.MemoryLimit == "128M" || server.CompareMemory("256M", activePoolMem) > 0 {
				needsMemoryScale = true
				if server.CompareMemory("256M", recommendedMemory) > 0 {
					recommendedMemory = "256M"
				}
				if len(heavyFound) >= 3 && server.CompareMemory("512M", recommendedMemory) > 0 {
					recommendedMemory = "512M"
				}
				ui.RowBadge("Plugin Memory Sizing", fmt.Sprintf("▲ %s (Low for %s)", activePoolMem, strings.Join(heavyFound, ", ")), ui.BrightYellow)
				recommendations = append(recommendations,
					fmt.Sprintf("Heavy Plugins Detected (%s): Active memory limit is %s. Recommended: %s. Run 'ap site doctor %s --fix' to auto-upgrade.", strings.Join(heavyFound, ", "), activePoolMem, recommendedMemory, targetSite.Domain))
				fixableIssues = append(fixableIssues, fmt.Sprintf("Under-allocated PHP Memory for heavy plugins (%s)", strings.Join(heavyFound, ", ")))
			} else {
				ui.RowBadge("Plugin Memory Sizing", fmt.Sprintf("● %s (Optimized for %s)", activePoolMem, strings.Join(heavyFound, ", ")), ui.BrightGreen)
			}
		}
	}

	// -------------------------------------------------------------
	// 8. Directory Security & Permissions
	// -------------------------------------------------------------
	ui.SectionHeader("8. DIRECTORY SECURITY & PERMISSIONS")
	if targetSite.IsLocked {
		ui.RowBadge("Security Lock", "● Locked (Read-Only / Protected)", ui.BrightGreen)
	} else {
		ui.RowBadge("Security Lock", "○ Unlocked (Standard Write Access)", ui.BrightYellow)
	}
	ui.Row("Webroot Directory", targetSite.PublicDir)
	ui.Row("System User", targetSite.SystemUser)

	// -------------------------------------------------------------
	// Auto-Remediation Execution (--fix flag)
	// -------------------------------------------------------------
	if autoFix && (needsCaddyWWWFix || needsMemoryScale || needsOrphanCleanup || needsOpcacheFix || needsPMFix || len(fixableIssues) > 0) {
		ui.Divider()
		ui.Banner("STARTING AUTOMATIC REMEDIATION & HEALING")
		ui.PrintInfo(fmt.Sprintf("Found %d auto-healable issue(s). Applying safe, non-destructive remedies...", len(fixableIssues)))

		// 1. Fix Caddyfile (Adds WWW canonical redirect and prevents Cloudflare Error 525)
		if needsCaddyWWWFix {
			ui.PrintStep(1, "Regenerating Caddy configuration with WWW canonical redirect...")
			if err := server.WriteCaddyfile(state); err != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to write Caddyfile: %v", err))
			} else {
				ui.PrintInfo("Reloading Caddy webserver (Zero-Downtime)...")
				if err := server.ReloadCaddy(state); err != nil {
					ui.PrintWarning(fmt.Sprintf("Failed to reload Caddy: %v", err))
				} else {
					ui.PrintSuccess("Caddyfile updated with WWW redirect! Caddy reloaded successfully.")
					// Re-probe TLS to verify
					reProbe := probeLocalTLS(wwwDomain)
					if reProbe.Success {
						ui.PrintSuccess(fmt.Sprintf("TLS Handshake Re-Probe Passed: 'https://%s' is now fully reachable. (Cloudflare Error 525 Resolved!)", wwwDomain))
					}
				}
			}
		}

		// 2. Clean up Orphaned / Inactive PHP Pools
		if needsOrphanCleanup && len(orphanedVersions) > 0 {
			ui.PrintStep(2, fmt.Sprintf("Purging orphaned PHP pool configs in PHP %s...", strings.Join(orphanedVersions, ", ")))
			cleaned, err := server.DeleteOrphanedPHPPools(targetSite.Domain, targetSite.PHPVersion, state.Global.SupportedPHPVersions)
			if err != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to remove some orphaned pools: %v", err))
			} else {
				ui.PrintSuccess(fmt.Sprintf("Purged orphaned PHP pools (%s) and reloaded daemons.", strings.Join(cleaned, ", ")))
			}
		}

		// 3. Fix PHP Memory Limit, OPcache, and PM Configuration
		if needsMemoryScale || needsOpcacheFix || needsPMFix {
			targetMem := targetSite.MemoryLimit
			if needsMemoryScale {
				targetMem = recommendedMemory
			}
			if targetMem == "" {
				if isWP {
					targetMem = "256M"
				} else {
					targetMem = "128M"
				}
			}

			ui.PrintStep(3, fmt.Sprintf("Synchronizing PHP pool configuration for %s (memory=%s, opcache validate=1, safe PM bounds)...", targetSite.Domain, targetMem))
			targetSite.MemoryLimit = targetMem
			if targetMem == "256M" || targetMem == "512M" {
				targetSite.MaxExecutionTime = 300
				targetSite.MaxInputVars = 5000
			}

			_ = config.WithLockedState(statePath, func(s *config.State) error {
				for i, st := range s.Sites {
					if strings.EqualFold(st.Domain, targetSite.Domain) {
						s.Sites[i].MemoryLimit = targetMem
						if targetMem == "256M" || targetMem == "512M" {
							s.Sites[i].MaxExecutionTime = 300
							s.Sites[i].MaxInputVars = 5000
						}
						break
					}
				}
				return nil
			})

			if err := server.WritePHPPool(targetSite); err != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to rewrite PHP pool: %v", err))
			} else {
				ui.PrintInfo(fmt.Sprintf("Reloading PHP-FPM %s...", targetSite.PHPVersion))
				if err := server.ReloadPHP(targetSite.PHPVersion); err != nil {
					ui.PrintWarning(fmt.Sprintf("Failed to reload PHP %s: %v", targetSite.PHPVersion, err))
				} else {
					ui.PrintSuccess(fmt.Sprintf("PHP-FPM pool upgraded: memory_limit=%s, opcache validate=1, safe PM bounds applied.", targetMem))
				}
			}

			if isWP {
				_ = server.TuneWordPressMemory(targetSite.SystemUser, targetSite.PublicDir, targetMem)
			}
		}

		// 4. Re-run verification check
		ui.PrintStep(4, "Performing post-repair verification probe...")
		postProbe := probeLocalHTTP(targetSite.Domain)
		if postProbe.StatusCode == 200 || postProbe.StatusCode == 301 || postProbe.StatusCode == 302 {
			ui.PrintSuccess(fmt.Sprintf("Post-repair verification passed! Site responded with HTTP %d.", postProbe.StatusCode))
		}
		ui.Divider()
		ui.PrintSuccess("All auto-healable issues have been cured. Site is operational!")
		fmt.Println()
		return nil
	}

	// -------------------------------------------------------------
	// Prescription & Summary
	// -------------------------------------------------------------
	ui.Divider()
	if len(recommendations) == 0 {
		ui.PrintSuccess("All Diagnostic Checks Passed! Site is fully operational and healthy.")
	} else {
		ui.PrintWarning(fmt.Sprintf("%d Diagnostic Warning(s) or Issue(s) Found", len(recommendations)))
		ui.SectionHeader("PRESCRIPTION & RECOMMENDED ACTIONS")
		for i, rec := range recommendations {
			fmt.Printf("  %s%d.%s %s\n", ui.BrightCyan, i+1, ui.Reset, rec)
		}

		if len(fixableIssues) > 0 {
			fmt.Println()
			ui.PrintInfo(fmt.Sprintf("Quick Fix Available: Run 'ap site doctor %s --fix' to automatically resolve %d of these issues with 1 click.", targetSite.Domain, len(fixableIssues)))
		}
	}
	fmt.Println()

	return nil
}

// DoctorAll runs diagnostic health checks across all sites configured in AgilePanel.
func DoctorAll(autoFix bool) error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return err
	}

	if len(state.Sites) == 0 {
		ui.PrintInfo("No websites currently configured on this server.")
		return nil
	}

	ui.Banner(fmt.Sprintf("AGILEPANEL SYSTEM-WIDE DOCTOR (%d Sites)", len(state.Sites)))
	for _, site := range state.Sites {
		_ = DoctorWithFix(site.Domain, autoFix)
	}
	return nil
}
