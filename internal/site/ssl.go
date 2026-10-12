package site

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agilepanel/internal/config"
	"agilepanel/internal/server"
	"agilepanel/internal/ui"
)

type SSLInfo struct {
	CertPath   string
	KeyPath    string
	MetaPath   string
	Issuer     string
	Expiration time.Time
	Active     bool
}

// GetSSLInfo searches Caddy storage paths for certificate data.
func GetSSLInfo(domain string) (*SSLInfo, error) {
	if os.Getenv("AGILEPANEL_TEST_MODE") == "true" {
		return &SSLInfo{
			CertPath:   "/var/lib/caddy/.local/share/caddy/certificates/acme-v02.api.letsencrypt.org-directory/" + domain + "/" + domain + ".crt",
			KeyPath:    "/var/lib/caddy/.local/share/caddy/certificates/acme-v02.api.letsencrypt.org-directory/" + domain + "/" + domain + ".key",
			MetaPath:   "/var/lib/caddy/.local/share/caddy/certificates/acme-v02.api.letsencrypt.org-directory/" + domain + "/" + domain + ".json",
			Issuer:     "Let's Encrypt",
			Expiration: time.Now().AddDate(0, 2, 15),
			Active:     true,
		}, nil
	}

	caddyStoragePaths := []string{
		"/var/lib/caddy/.local/share/caddy/certificates",
		"/root/.local/share/caddy/certificates",
	}

	var foundCertPath string
	var foundKeyPath string
	var foundMetaPath string
	var issuer string

	for _, storagePath := range caddyStoragePaths {
		if _, err := os.Stat(storagePath); err != nil {
			continue
		}

		entries, err := os.ReadDir(storagePath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			domainPath := filepath.Join(storagePath, entry.Name(), domain)
			certFile := filepath.Join(domainPath, domain+".crt")
			keyFile := filepath.Join(domainPath, domain+".key")
			metaFile := filepath.Join(domainPath, domain+".json")

			if _, err := os.Stat(certFile); err == nil {
				foundCertPath = certFile
				foundKeyPath = keyFile
				foundMetaPath = metaFile
				issuer = entry.Name()
				if strings.Contains(issuer, "letsencrypt") {
					issuer = "Let's Encrypt"
				} else if strings.Contains(issuer, "zerossl") {
					issuer = "ZeroSSL"
				}
				break
			}
		}

		if foundCertPath != "" {
			break
		}
	}

	if foundCertPath == "" {
		return &SSLInfo{Active: false}, nil
	}

	certBytes, err := os.ReadFile(foundCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate: %w", err)
	}

	block, _ := pem.Decode(certBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse x509 certificate: %w", err)
	}

	return &SSLInfo{
		CertPath:   foundCertPath,
		KeyPath:    foundKeyPath,
		MetaPath:   foundMetaPath,
		Issuer:     issuer,
		Expiration: cert.NotAfter,
		Active:     true,
	}, nil
}

// SSLRenew deletes cached Caddy certificates for the domain to force immediate renewal on reload.
func SSLRenew(domain string) error {
	statePath := config.GetStatePath()
	state, err := config.ReadState(statePath)
	if err != nil {
		return err
	}

	found := false
	for _, site := range state.Sites {
		if strings.EqualFold(site.Domain, domain) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("site %s not found in state", domain)
	}

	certPaths := []string{
		fmt.Sprintf("/var/lib/caddy/.local/share/caddy/certificates/acme-v02.api.letsencrypt.org-directory/%s", domain),
		fmt.Sprintf("/var/lib/caddy/.local/share/caddy/certificates/acme.zerossl.com-v2-directory/%s", domain),
		fmt.Sprintf("/root/.local/share/caddy/certificates/acme-v02.api.letsencrypt.org-directory/%s", domain),
		fmt.Sprintf("/root/.local/share/caddy/certificates/acme.zerossl.com-v2-directory/%s", domain),
	}

	fmt.Printf("Caddy: Removing cached SSL credentials for %s...\n", domain)
	for _, path := range certPaths {
		if _, err := os.Stat(path); err == nil {
			if err := os.RemoveAll(path); err != nil {
				fmt.Printf("Warning: Failed to delete cert cache folder at %s: %v\n", path, err)
			} else {
				fmt.Printf("SSL: Removed cached certificates at %s\n", path)
			}
		}
	}

	fmt.Println("Caddy: Triggering Caddy configuration reload to force SSL renewal...")
	if err := server.ReloadCaddy(state); err != nil {
		return err
	}

	ui.PrintSuccess("SSL Renewal Triggered")
	ui.PrintInfo("AgilePanel has deleted the local SSL certificate cache for " + domain + " and triggered a Caddy configuration reload. Caddy will dynamically negotiate a new, valid SSL certificate with Let's Encrypt / ZeroSSL on the next connection.")
	ui.Divider()
	fmt.Println()
	return nil
}
