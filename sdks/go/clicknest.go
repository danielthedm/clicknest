// Package analytics provides a Go SDK for server-side ClickNest analytics.
package analytics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type event struct {
	EventType  string         `json:"event_type"`
	URL        string         `json:"url"`
	URLPath    string         `json:"url_path"`
	Timestamp  int64          `json:"timestamp"`
	Properties map[string]any `json:"properties,omitempty"`
}

type queueItem struct {
	Event      event  `json:"event"`
	DistinctID string `json:"distinct_id"`
}

type payload struct {
	Events     []event `json:"events"`
	SessionID  string  `json:"session_id"`
	DistinctID string  `json:"distinct_id"`
}

// Client is a ClickNest analytics client.
type Client struct {
	apiKey       string
	host         string
	maxBatch     int
	mu           sync.Mutex
	queue        []queueItem
	client       *http.Client
	done         chan struct{}
}

// New creates a new ClickNest client that flushes every flushInterval.
func New(apiKey, host string, flushInterval time.Duration) *Client {
	c := &Client{
		apiKey:   apiKey,
		host:     host,
		maxBatch: 50,
		client:   &http.Client{Timeout: 5 * time.Second},
		done:     make(chan struct{}),
	}
	go c.flushLoop(flushInterval)
	return c
}

// Capture tracks an event.
func (c *Client) Capture(eventName string, distinctID string, properties map[string]any) {
	if properties == nil {
		properties = map[string]any{}
	}
	properties["$event_name"] = eventName

	c.mu.Lock()
	c.queue = append(c.queue, queueItem{
		Event: event{
			EventType:  "custom",
			URL:        eventName,
			URLPath:    eventName,
			Timestamp:  time.Now().UnixMilli(),
			Properties: properties,
		},
		DistinctID: distinctID,
	})
	full := len(c.queue) >= c.maxBatch
	c.mu.Unlock()

	if full {
		c.Flush()
	}
}

// Identify links an anonymous ID to an identified user.
func (c *Client) Identify(distinctID, previousID string) {
	if previousID != "" {
		c.Capture("$identify", distinctID, map[string]any{
			"previous_id": previousID,
		})
	}
}

// Flush sends all queued events. Failed batches are re-queued for retry.
func (c *Client) Flush() {
	c.mu.Lock()
	items := c.queue
	c.queue = nil
	c.mu.Unlock()

	if len(items) == 0 {
		return
	}

	// Group by distinct_id.
	batches := map[string][]event{}
	for _, item := range items {
		batches[item.DistinctID] = append(batches[item.DistinctID], item.Event)
	}

	var failed []queueItem
	for did, events := range batches {
		p := payload{
			Events:     events,
			SessionID:  fmt.Sprintf("server-%d", time.Now().Unix()),
			DistinctID: did,
		}
		body, err := json.Marshal(p)
		if err != nil {
			continue
		}

		req, err := http.NewRequest("POST", c.host+"/api/v1/events", bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", c.apiKey)

		resp, err := c.client.Do(req)
		if err != nil {
			// Re-queue for retry on next flush.
			for _, ev := range events {
				failed = append(failed, queueItem{Event: ev, DistinctID: did})
			}
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 500 {
			for _, ev := range events {
				failed = append(failed, queueItem{Event: ev, DistinctID: did})
			}
		}
	}

	// Re-queue failed events (cap at 500 to prevent unbounded growth).
	if len(failed) > 0 {
		c.mu.Lock()
		total := len(failed) + len(c.queue)
		if total > 500 {
			// Drop oldest failed events to stay within cap.
			drop := total - 500
			if drop >= len(failed) {
				failed = nil
			} else {
				failed = failed[drop:]
			}
		}
		c.queue = append(failed, c.queue...)
		c.mu.Unlock()
	}
}

// Shutdown flushes remaining events and stops the background goroutine.
func (c *Client) Shutdown() {
	close(c.done)
	c.Flush()
}

func (c *Client) flushLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.Flush()
		}
	}
}
