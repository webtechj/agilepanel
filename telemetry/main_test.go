package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleHome(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handleHome(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("expected text/plain content type, got %s", contentType)
	}
}

func TestHandlePing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "telemetry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	dataDir = tempDir

	payload := PingPayload{
		UUID:      "test-server-uuid-99999",
		OS:        "linux",
		Arch:      "amd64",
		Version:   "0.8.0",
		SiteCount: 3,
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/ping", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlePing(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", resp.StatusCode)
	}

	// Verify file is created
	recordPath := filepath.Join(dataDir, payload.UUID+".json")
	if _, err := os.Stat(recordPath); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist, but it doesn't", recordPath)
	}

	// Verify file contents
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}

	var record ServerRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("failed to unmarshal created file data: %v", err)
	}

	if record.UUID != payload.UUID {
		t.Errorf("expected UUID %s, got %s", payload.UUID, record.UUID)
	}
	if record.SiteCount != 3 {
		t.Errorf("expected site count 3, got %d", record.SiteCount)
	}
	if record.LastSeen.IsZero() {
		t.Error("expected LastSeen timestamp to be set")
	}
}

func TestHandlePingInvalid(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "telemetry-test-*")
	defer os.RemoveAll(tempDir)
	dataDir = tempDir

	// Test invalid JSON
	req := httptest.NewRequest("POST", "/api/ping", bytes.NewBufferString("{invalid-json}"))
	w := httptest.NewRecorder()
	handlePing(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for invalid JSON, got %d", w.Result().StatusCode)
	}

	// Test missing UUID
	payload := PingPayload{
		OS:      "linux",
		Version: "0.8.0",
	}
	body, _ := json.Marshal(payload)
	req2 := httptest.NewRequest("POST", "/api/ping", bytes.NewBuffer(body))
	w2 := httptest.NewRecorder()
	handlePing(w2, req2)
	if w2.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for missing UUID, got %d", w2.Result().StatusCode)
	}
}

func TestBadges(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "telemetry-test-*")
	defer os.RemoveAll(tempDir)
	dataDir = tempDir

	// Setup mock data
	records := []ServerRecord{
		{
			UUID:      "server-1",
			OS:        "linux",
			Version:   "0.8.0",
			SiteCount: 2,
			LastSeen:  time.Now(), // Active
		},
		{
			UUID:      "server-2",
			OS:        "linux",
			Version:   "0.8.0",
			SiteCount: 5,
			LastSeen:  time.Now().AddDate(0, 0, -10), // Inactive (> 7 days)
		},
	}

	for _, rec := range records {
		data, _ := json.Marshal(rec)
		os.WriteFile(filepath.Join(dataDir, rec.UUID+".json"), data, 0644)
	}

	// 1. Test JSON Badge
	req := httptest.NewRequest("GET", "/api/badge?metric=active", nil)
	w := httptest.NewRecorder()
	handleJSONBadge(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var jsonBadge map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&jsonBadge)

	if jsonBadge["label"] != "active servers" {
		t.Errorf("expected label 'active servers', got %v", jsonBadge["label"])
	}
	// "server-1" is active. "server-2" is inactive. So count should be 1.
	if jsonBadge["message"] != "1" {
		t.Errorf("expected message '1', got %v", jsonBadge["message"])
	}

	// Test JSON Badge for Sites
	reqSites := httptest.NewRequest("GET", "/api/badge?metric=sites", nil)
	wSites := httptest.NewRecorder()
	handleJSONBadge(wSites, reqSites)
	var jsonBadgeSites map[string]interface{}
	json.NewDecoder(wSites.Result().Body).Decode(&jsonBadgeSites)
	if jsonBadgeSites["label"] != "wordpress sites" {
		t.Errorf("expected label 'wordpress sites', got %v", jsonBadgeSites["label"])
	}
	// Only active server sites are counted: 2 sites.
	if jsonBadgeSites["message"] != "2" {
		t.Errorf("expected message '2', got %v", jsonBadgeSites["message"])
	}

	// 2. Test SVG Badge
	reqSVG := httptest.NewRequest("GET", "/badge.svg?metric=active", nil)
	wSVG := httptest.NewRecorder()
	handleSVGBadge(wSVG, reqSVG)

	respSVG := wSVG.Result()
	if respSVG.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", respSVG.StatusCode)
	}

	contentType := respSVG.Header.Get("Content-Type")
	if contentType != "image/svg+xml" {
		t.Errorf("expected image/svg+xml, got %s", contentType)
	}

	svgBytes, _ := io.ReadAll(respSVG.Body)
	svgStr := string(svgBytes)
	if !strings.Contains(svgStr, "<svg") || !strings.Contains(svgStr, "active servers") {
		t.Error("rendered SVG is malformed or does not contain active servers text")
	}
}
