package config

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"time"
)

const Version = "0.8.0"

// TelemetryPayload represents the anonymous metadata sent to the telemetry server.
type TelemetryPayload struct {
	UUID      string `json:"uuid"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Version   string `json:"version"`
	SiteCount int    `json:"site_count"`
}

// SendTelemetryPing transmits anonymous usage metrics to the telemetry endpoint.
func SendTelemetryPing(s *State) error {
	// Allow user to opt-out of telemetry completely
	if urlEnv := os.Getenv("AGILEPANEL_TELEMETRY_URL"); urlEnv == "none" {
		return nil
	}

	endpoint := "https://telemetry.agilepanel.io/api/ping"
	if urlEnv := os.Getenv("AGILEPANEL_TELEMETRY_URL"); urlEnv != "" {
		endpoint = urlEnv
	}

	payload := TelemetryPayload{
		UUID:      s.Global.UUID,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Version:   Version,
		SiteCount: len(s.Sites),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: 1500 * time.Millisecond,
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AgilePanel-CLI/"+Version)


	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// PingAsync triggers SendTelemetryPing in a safe, non-blocking background goroutine.
func PingAsync(s *State) {
	if s == nil {
		return
	}
	go func() {
		defer func() {
			_ = recover() // Catch any panic to prevent CLI crash
		}()
		_ = SendTelemetryPing(s)
	}()
}
