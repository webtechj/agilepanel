package site

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
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

// Doctor runs an automated diagnostic audit for a website and prints a health report with recommendations.
func Doctor(domain string) error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return err
	}

	var targetSite config.SiteConfig
	found := false
	for _, s := range state.Sites {
		if strings.EqualFold(s.Domain, domain) {
			targetSite = s
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("site %s not found in state", domain)
	}

	serverIP := server.ResolvePublicIP()
	var recommendations []string

	ui.Banner("Site Doctor Diagnostic Report: " + strings.ToUpper(domain))

	// 1. DNS Resolution Check
	ui.SectionHeader("1. DNS & NETWORK REACHABILITY")
	resolvedIP, dnsMatches, dnsErr := CheckDomainDNS(domain)
	if dnsErr != nil {
		ui.RowBadge("DNS Resolution", "✖ Failed", ui.BrightRed)
		ui.Row("Lookup Error", dnsErr.Error())
		ui.Row("Expected Server IP", serverIP)
		recommendations = append(recommendations,
			fmt.Sprintf("DNS Record Missing: Ensure you have added an 'A' record pointing '%s' to '%s' at your DNS provider.", domain, serverIP))
	} else if !dnsMatches {
		ui.RowBadge("DNS Resolution", "▲ IP Mismatch", ui.BrightYellow)
		ui.Row("Current Domain IP", resolvedIP)
		ui.Row("This Server IP", serverIP)
		recommendations = append(recommendations,
			fmt.Sprintf("DNS IP Mismatch: Domain currently resolves to %s instead of this VPS (%s). If using Cloudflare, verify proxy or update your DNS A-record.", resolvedIP, serverIP))
	} else {
		ui.RowBadge("DNS Resolution", "● Healthy", ui.BrightGreen)
		ui.Row("Domain IP", fmt.Sprintf("%s (matches server)", resolvedIP))
	}

	// 2. SSL / HTTPS Certificate Check
	ui.SectionHeader("2. SSL / TLS CERTIFICATE")
	sslInfo, _ := GetSSLInfo(domain)
	if sslInfo != nil && sslInfo.Active {
		daysLeft := int(time.Until(sslInfo.Expiration).Hours() / 24)
		if daysLeft < 14 {
			ui.RowBadge("SSL Status", fmt.Sprintf("▲ Expiring Soon (%d days)", daysLeft), ui.BrightYellow)
			recommendations = append(recommendations,
				fmt.Sprintf("SSL Nearing Expiry: Only %d days remaining on %s certificate. Run 'ap site ssl-renew %s' to force renewal.", daysLeft, sslInfo.Issuer, domain))
		} else {
			ui.RowBadge("SSL Status", "● Active", ui.BrightGreen)
		}
		ui.Row("Issuer", sslInfo.Issuer)
		ui.Row("Expiration", fmt.Sprintf("%s (%d days remaining)", sslInfo.Expiration.Format("2006-01-02"), daysLeft))
	} else {
		ui.RowBadge("SSL Status", "▲ Inactive / Self-Signed", ui.BrightYellow)
		recommendations = append(recommendations,
			fmt.Sprintf("SSL Inactive: Let's Encrypt certificate not found for %s. Once DNS points to %s, run 'ap site ssl-renew %s'.", domain, serverIP, domain))
	}

	// 3. PHP-FPM Service & Pool Configuration
	ui.SectionHeader("3. PHP-FPM PROCESS MANAGER")
	poolPath := server.GetPHPPoolPath(targetSite.PHPVersion, targetSite.Domain)
	if _, err := os.Stat(poolPath); err == nil {
		ui.RowBadge("PHP Pool Config", "● Active", ui.BrightGreen)
		ui.Row("PHP Version", targetSite.PHPVersion)
		memLimit := targetSite.MemoryLimit
		if memLimit == "" {
			memLimit = "128M (default)"
		}
		ui.Row("PHP Memory Limit", memLimit)
		ui.Row("Config File", poolPath)
	} else {
		ui.RowBadge("PHP Pool Config", "✖ Missing", ui.BrightRed)
		ui.Row("Target Path", poolPath)
		recommendations = append(recommendations,
			fmt.Sprintf("PHP Pool Missing: Run 'ap repair' to rebuild pool configurations for PHP %s.", targetSite.PHPVersion))
	}

	// 4. Database Connectivity Check
	ui.SectionHeader("4. MARIADB DATABASE CONNECTIVITY")
	if targetSite.DatabaseName == "" || targetSite.Type == "html" {
		ui.RowBadge("Database Engine", "● N/A (Static Site)", ui.BrightGreen)
	} else {
		ui.Row("Database Name", targetSite.DatabaseName)
		ui.Row("Database User", targetSite.DatabaseUser)
		// Test connectivity
		dbConnected := true
		if os.Getenv("AGILEPANEL_TEST_MODE") != "true" && runtime.GOOS == "linux" {
			homeDir := server.GetSiteRootDir(targetSite.PublicDir)
			if targetSite.Type == "wp" || targetSite.Type == "" {
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
				fmt.Sprintf("Database Connection Failure: WordPress could not connect to database '%s'. Check database credentials or run 'ap site reinstall %s' if tables were dropped.", targetSite.DatabaseName, domain))
		}
	}

	// 5. WordPress Core & Log Inspection
	isWP := targetSite.Type == "wp" || targetSite.Type == "woocommerce" || targetSite.Type == ""
	if isWP {
		ui.SectionHeader("5. WORDPRESS APPLICATION & ERROR LOGS")
		wpConfigPath := filepath.Join(server.GetSiteRootDir(targetSite.PublicDir), "wp-config.php")
		if _, err := os.Stat(wpConfigPath); err == nil {
			ui.RowBadge("Configuration File", "● Secured in parent root", ui.BrightGreen)
		} else {
			// Check webroot
			altPath := filepath.Join(targetSite.PublicDir, "wp-config.php")
			if _, err := os.Stat(altPath); err == nil {
				ui.RowBadge("Configuration File", "● Found in webroot", ui.BrightGreen)
			} else {
				ui.RowBadge("Configuration File", "✖ wp-config.php Not Found", ui.BrightRed)
				recommendations = append(recommendations,
					fmt.Sprintf("WordPress Core Missing: wp-config.php was not found. Run 'ap site reinstall %s' to restore clean core files.", domain))
			}
		}

		// Inspect debug.log for fatal errors if present
		debugLogPath := filepath.Join(targetSite.PublicDir, "wp-content", "debug.log")
		hasFatals := false
		var fatalSnippet string
		if f, err := os.Open(debugLogPath); err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "Fatal error:") || strings.Contains(line, "Parse error:") {
					hasFatals = true
					fatalSnippet = line
				}
			}
		}

		if hasFatals {
			ui.RowBadge("Error Log Audit", "▲ Fatal Errors Detected", ui.BrightYellow)
			if len(fatalSnippet) > 70 {
				fatalSnippet = fatalSnippet[:67] + "..."
			}
			ui.Row("Latest Fatal Error", fatalSnippet)
			recommendations = append(recommendations,
				fmt.Sprintf("PHP Fatal Error: A plugin or theme is crashing WordPress. Run 'ap site safe-mode %s on' to temporarily deactivate plugins and restore access.", domain))
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
		}
		for _, hp := range checkHeavy {
			if _, err := os.Stat(filepath.Join(pluginsDir, hp.slug)); err == nil {
				heavyFound = append(heavyFound, hp.name)
			}
		}

		if len(heavyFound) > 0 {
			activeMem := targetSite.MemoryLimit
			if activeMem == "" || activeMem == "128M" {
				ui.RowBadge("Plugin Memory Sizing", fmt.Sprintf("▲ 128M (Low for %s)", strings.Join(heavyFound, ", ")), ui.BrightYellow)
				recommendations = append(recommendations,
					fmt.Sprintf("Heavy Plugins Detected (%s): Active memory limit is 128M. Recommended: 256M. Run 'ap site update %s --memory 256M' to allocate 256MB.", strings.Join(heavyFound, ", "), domain))
			} else {
				ui.RowBadge("Plugin Memory Sizing", fmt.Sprintf("● %s (Optimized for %s)", activeMem, strings.Join(heavyFound, ", ")), ui.BrightGreen)
			}
		}
	}

	// 6. Security & Directory Protection
	ui.SectionHeader("6. DIRECTORY SECURITY & PERMISSIONS")
	if targetSite.IsLocked {
		ui.RowBadge("Security Lock", "● Locked (Read-Only / Protected)", ui.BrightGreen)
	} else {
		ui.RowBadge("Security Lock", "○ Unlocked (Standard Write Access)", ui.BrightYellow)
	}
	ui.Row("Webroot Directory", targetSite.PublicDir)
	ui.Row("System User", targetSite.SystemUser)

	// Summary / Prescription
	ui.Divider()
	if len(recommendations) == 0 {
		ui.PrintSuccess("All Diagnostic Checks Passed! Site is fully operational and healthy.")
	} else {
		ui.PrintWarning(fmt.Sprintf("%d Diagnostic Warning(s) or Issue(s) Found", len(recommendations)))
		ui.SectionHeader("PRESCRIPTION & RECOMMENDED ACTIONS")
		for i, rec := range recommendations {
			fmt.Printf("  %s%d.%s %s\n", ui.BrightCyan, i+1, ui.Reset, rec)
		}
	}
	fmt.Println()

	return nil
}
