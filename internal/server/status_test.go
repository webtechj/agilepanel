package server

import (
	"testing"
)

func TestGetStatus(t *testing.T) {
	status, err := GetStatus()
	if err != nil {
		t.Fatalf("expected no error from GetStatus, got %v", err)
	}

	if status.Services == nil {
		t.Error("expected Services status map to be populated, got nil")
	}

	// Verify it contains standard services
	expectedServices := []string{"caddy", "mariadb", "redis-server"}
	for _, svc := range expectedServices {
		if _, ok := status.Services[svc]; !ok {
			t.Errorf("expected status to track service %s, but it was missing", svc)
		}
	}
}

func TestRestartServiceMock(t *testing.T) {
	err := RestartService("caddy")
	if err != nil {
		t.Fatalf("expected no error from mocked RestartService, got %v", err)
	}

	err = RestartService("all")
	if err != nil {
		t.Fatalf("expected no error from mocked RestartService with 'all', got %v", err)
	}
}
