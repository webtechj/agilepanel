package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PingPayload is the JSON body received from AgilePanel instances.
type PingPayload struct {
	UUID      string `json:"uuid"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Version   string `json:"version"`
	SiteCount int    `json:"site_count"`
}

// ServerRecord is the schema stored on disk.
type ServerRecord struct {
	UUID      string    `json:"uuid"`
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
	Version   string    `json:"version"`
	SiteCount int       `json:"site_count"`
	LastSeen  time.Time `json:"last_seen"`
	IPHash    string    `json:"ip_hash"`
}

var (
	dataDir string
	mu      sync.Mutex // Protects file operations from race conditions
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dataDir = os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data/servers"
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/api/ping", handlePing)
	http.HandleFunc("/api/badge", handleJSONBadge)
	http.HandleFunc("/badge.svg", handleSVGBadge)

	log.Printf("Starting telemetry server on port %s...", port)
	log.Printf("Storing records in: %s", dataDir)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "AgilePanel Telemetry Service is running.")
}

func getClientIP(r *http.Request) string {
	// Check Cloudflare header first, then X-Forwarded-For, then RemoteAddr
	if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
		return cf
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func hashIP(ip string) string {
	h := sha256.New()
	h.Write([]byte(ip))
	return hex.EncodeToString(h.Sum(nil))
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var payload PingPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	payload.UUID = strings.TrimSpace(payload.UUID)
	if payload.UUID == "" || len(payload.UUID) < 10 {
		http.Error(w, "Invalid UUID", http.StatusBadRequest)
		return
	}

	ipHash := hashIP(getClientIP(r))

	record := ServerRecord{
		UUID:      payload.UUID,
		OS:        payload.OS,
		Arch:      payload.Arch,
		Version:   payload.Version,
		SiteCount: payload.SiteCount,
		LastSeen:  time.Now(),
		IPHash:    ipHash,
	}

	recordData, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	// Write atomically (temp file + rename)
	filePath := filepath.Join(dataDir, record.UUID+".json")
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, recordData, 0644); err != nil {
		http.Error(w, "Failed to write record", http.StatusInternalServerError)
		return
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		http.Error(w, "Failed to persist record", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

type BadgeMetrics struct {
	ActiveServers int
	TotalServers  int
	TotalSites    int
}

func getMetrics() (BadgeMetrics, error) {
	mu.Lock()
	defer mu.Unlock()

	files, err := os.ReadDir(dataDir)
	if err != nil {
		return BadgeMetrics{}, err
	}

	var metrics BadgeMetrics
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		metrics.TotalServers++

		filePath := filepath.Join(dataDir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var rec ServerRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}

		if rec.LastSeen.After(sevenDaysAgo) {
			metrics.ActiveServers++
			metrics.TotalSites += rec.SiteCount
		}
	}

	return metrics, nil
}

func handleJSONBadge(w http.ResponseWriter, r *http.Request) {
	metrics, err := getMetrics()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	metricType := r.URL.Query().Get("metric")
	if metricType == "" {
		metricType = "active"
	}

	label := "active servers"
	value := strconv.Itoa(metrics.ActiveServers)
	color := "blue"

	switch metricType {
	case "total":
		label = "total servers"
		value = strconv.Itoa(metrics.TotalServers)
		color = "blue"
	case "sites":
		label = "wordpress sites"
		value = strconv.Itoa(metrics.TotalSites)
		color = "orange"
	}

	// Response structured for shields.io endpoint badge
	resp := map[string]interface{}{
		"schemaVersion": 1,
		"label":         label,
		"message":       value,
		"color":         color,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	json.NewEncoder(w).Encode(resp)
}

func handleSVGBadge(w http.ResponseWriter, r *http.Request) {
	metrics, err := getMetrics()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	metricType := r.URL.Query().Get("metric")
	if metricType == "" {
		metricType = "active"
	}

	label := "active servers"
	val := strconv.Itoa(metrics.ActiveServers)
	color := "#007ec6" // Shields.io blue

	switch metricType {
	case "total":
		label = "total servers"
		val = strconv.Itoa(metrics.TotalServers)
		color = "#007ec6"
	case "sites":
		label = "wordpress sites"
		val = strconv.Itoa(metrics.TotalSites)
		color = "#fe7d37" // Shields.io orange
	}

	// Calculate widths dynamically using flat badge heuristic
	labelWidth := 90
	switch label {
	case "total servers":
		labelWidth = 85
	case "wordpress sites":
		labelWidth = 105
	}

	valWidth := 30
	valLen := len(val)
	if valLen >= 5 {
		valWidth = 55
	} else if valLen >= 3 {
		valWidth = 40
	}

	totalWidth := labelWidth + valWidth
	labelX := labelWidth / 2
	valX := labelWidth + (valWidth / 2)

	svgTemplate := `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20">
  <linearGradient id="b" x2="0" y2="100%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <mask id="a">
    <rect width="%d" height="20" rx="3" fill="#fff"/>
  </mask>
  <g mask="url(#a)">
    <path fill="#555" d="M0 0h%dv20H0z"/>
    <path fill="%s" d="M%d 0h%dv20H%dz"/>
    <path fill="url(#b)" d="M0 0h%dv20H0z"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
  </g>
</svg>`

	svgContent := fmt.Sprintf(svgTemplate,
		totalWidth,
		totalWidth,
		labelWidth,
		color,
		labelWidth, valWidth, labelWidth,
		totalWidth,
		labelX, label,
		labelX, label,
		valX, val,
		valX, val,
	)

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(svgContent))
}
