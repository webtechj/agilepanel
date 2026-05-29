package server

import (
	"testing"
)

func TestInstallWordPressMock(t *testing.T) {
	adminPassword, err := InstallWordPress(
		"wp_testsite_com",
		"testsite.com",
		"/var/www/testsite.com/public",
		"db_testsite_com",
		"wp_testsite_com",
		"random_db_password",
		"/var/run/redis/redis-server.sock",
		"siteadmin",
		"siteadmin@testsite.com",
	)
	if err != nil {
		t.Fatalf("expected no error from mocked InstallWordPress, got %v", err)
	}

	if len(adminPassword) != 16 {
		t.Errorf("expected admin password length to be 16 characters, got %d", len(adminPassword))
	}
}
