package config

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSendTelemetryPing(t *testing.T) {
	// Setup mock state
	state := DefaultState()
	state.Global.UUID = "test-uuid-12345"
	state.Sites = []SiteConfig{
		{Domain: "site1.com"},
		{Domain: "site2.com"},
	}

	// Create test HTTP server
	var receivedPayload TelemetryPayload
	var receivedContentType string
	var receivedUserAgent string
	var serverCalled bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		receivedContentType = r.Header.Get("Content-Type")
		receivedUserAgent = r.Header.Get("User-Agent")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(body, &receivedPayload); err != nil {
			t.Errorf("failed to unmarshal request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Override endpoint
	os.Setenv("AGILEPANEL_TELEMETRY_URL", ts.URL)
	defer os.Unsetenv("AGILEPANEL_TELEMETRY_URL")

	err := SendTelemetryPing(state)
	if err != nil {
		t.Fatalf("SendTelemetryPing failed: %v", err)
	}

	if !serverCalled {
		t.Error("expected telemetry server to be called, but it wasn't")
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", receivedContentType)
	}
	if receivedUserAgent != "AgilePanel-CLI/0.8.0" {
		t.Errorf("expected User-Agent AgilePanel-CLI/0.8.0, got %s", receivedUserAgent)
	}
	if receivedPayload.UUID != "test-uuid-12345" {
		t.Errorf("expected UUID test-uuid-12345, got %s", receivedPayload.UUID)
	}
	if receivedPayload.SiteCount != 2 {
		t.Errorf("expected SiteCount 2, got %d", receivedPayload.SiteCount)
	}
	if receivedPayload.Version != "0.8.0" {
		t.Errorf("expected Version 0.8.0, got %s", receivedPayload.Version)
	}
}

func TestSendTelemetryPingOptOut(t *testing.T) {
	// Set telemetry to none
	os.Setenv("AGILEPANEL_TELEMETRY_URL", "none")
	defer os.Unsetenv("AGILEPANEL_TELEMETRY_URL")

	// Even with nil state, it should return nil/no-op immediately when opt-out is configured
	err := SendTelemetryPing(nil)
	if err != nil {
		t.Fatalf("expected no-op (nil error) when opted out, got: %v", err)
	}
}
