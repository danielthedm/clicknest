// Package telemetry provides anonymous, opt-out usage reporting for ClickNest.
//
// Data collected:
//   - Random anonymous instance ID (no PII)
//   - App version, OS, architecture
//   - Feature usage counts (projects, users, connectors)
//
// Disable by setting CLICKNEST_TELEMETRY=off.
package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	telemetryURL  = "https://api.clicknest.app/api/v1/telemetry/heartbeat"
	flushInterval = 24 * time.Hour
)

// Stats is a function that returns current usage counts.
// Provided by the caller so telemetry doesn't import storage.
type Stats func(ctx context.Context) map[string]any

// Reporter sends anonymous usage data to the ClickNest control plane.
type Reporter struct {
	distinctID string
	version    string
	stats      Stats
}

// New creates a telemetry reporter. It persists an anonymous instance ID
// in dataDir so it stays consistent across restarts.
func New(dataDir, version string, stats Stats) *Reporter {
	return &Reporter{
		distinctID: getOrCreateID(dataDir),
		version:    version,
		stats:      stats,
	}
}

// Start launches the background heartbeat loop.
func (r *Reporter) Start(ctx context.Context) {
	// Send initial ping on startup.
	go r.send(ctx)

	go func() {
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.send(ctx)
			}
		}
	}()
}

func (r *Reporter) send(ctx context.Context) {
	payload := map[string]any{
		"anonymous_id": r.distinctID,
		"version":      r.version,
		"go_version":   runtime.Version(),
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
	}

	if r.stats != nil {
		for k, v := range r.stats(ctx) {
			payload[k] = v
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, telemetryURL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// getOrCreateID reads or creates a persistent anonymous instance ID.
func getOrCreateID(dataDir string) string {
	idFile := filepath.Join(dataDir, ".clicknest-id")

	if data, err := os.ReadFile(idFile); err == nil {
		id := string(bytes.TrimSpace(data))
		if len(id) > 0 {
			return id
		}
	}

	b := make([]byte, 16)
	rand.Read(b)
	id := hex.EncodeToString(b)

	if err := os.WriteFile(idFile, []byte(id), 0644); err != nil {
		log.Printf("telemetry: failed to write instance ID: %v", err)
	}

	return id
}

// Enabled returns true if telemetry is not disabled via environment variable.
func Enabled() bool {
	v := os.Getenv("CLICKNEST_TELEMETRY")
	return v != "off" && v != "false" && v != "0"
}
