// Package cloudreport provides a usage reporter that periodically flushes
// accumulated event counts to the ClickNest control plane for billing.
package cloudreport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

// Reporter accumulates event counts and periodically reports them to the
// control plane for usage-based billing.
type Reporter struct {
	controlPlaneURL string
	instanceID      string
	instanceSecret  string
	events          atomic.Int64
}

// New creates a new usage reporter.
func New(controlPlaneURL, instanceID, instanceSecret string) *Reporter {
	return &Reporter{
		controlPlaneURL: controlPlaneURL,
		instanceID:      instanceID,
		instanceSecret:  instanceSecret,
	}
}

// RecordEvents adds to the accumulated event counter. This is safe for
// concurrent use and designed to be called from the OnEventIngested hook.
func (r *Reporter) RecordEvents(count int64) {
	r.events.Add(count)
}

// Start launches the background flush loop. It runs until ctx is cancelled.
func (r *Reporter) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				// Final flush on shutdown.
				r.flush(context.Background())
				return
			case <-ticker.C:
				r.flush(ctx)
			}
		}
	}()
}

func (r *Reporter) flush(ctx context.Context) {
	count := r.events.Swap(0)
	if count == 0 {
		return
	}

	if err := r.report(ctx, count); err != nil {
		log.Printf("cloud usage reporter: %v", err)
		// Put the events back so they get retried next cycle.
		r.events.Add(count)
	}
}

func (r *Reporter) report(ctx context.Context, eventCount int64) error {
	url := r.controlPlaneURL + "/api/v1/usage/report"
	body, _ := json.Marshal(map[string]any{
		"instance_id": r.instanceID,
		"events":      eventCount,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.instanceSecret)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
