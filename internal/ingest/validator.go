package ingest

import (
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"
)

var (
	ErrEmptyBatch     = errors.New("empty event batch")
	ErrBatchTooLarge  = errors.New("batch exceeds maximum size of 100 events")
	ErrMissingType    = errors.New("event_type is required")
	ErrInvalidType    = errors.New("invalid event_type")
	ErrMissingURL     = errors.New("url is required")
	ErrInvalidURL     = errors.New("invalid url")
	ErrMissingSession = errors.New("session_id is required")
)

const maxBatchSize = 100
const maxTextLength = 500

var validEventTypes = map[string]bool{
	"click":    true,
	"pageview": true,
	"input":    true,
	"submit":   true,
	"custom":   true,
	"error":    true,
}

// ValidatePayload checks the incoming ingestion request for required fields.
func ValidatePayload(p *IngestPayload) error {
	if len(p.Events) == 0 {
		return ErrEmptyBatch
	}
	if len(p.Events) > maxBatchSize {
		return ErrBatchTooLarge
	}
	if p.SessionID == "" {
		return ErrMissingSession
	}
	for i := range p.Events {
		if err := validateEvent(&p.Events[i]); err != nil {
			return err
		}
	}
	return nil
}

func validateEvent(e *IngestEvent) error {
	if e.EventType == "" {
		return ErrMissingType
	}
	if !validEventTypes[e.EventType] {
		return ErrInvalidType
	}
	if e.URL == "" {
		return ErrMissingURL
	}
	if _, err := url.ParseRequestURI(e.URL); err != nil {
		return ErrInvalidURL
	}

	// Sanitize text fields to prevent excessive storage.
	e.ElementText = truncate(e.ElementText, maxTextLength)
	e.AriaLabel = truncate(e.AriaLabel, maxTextLength)
	e.PageTitle = truncate(e.PageTitle, maxTextLength)
	e.ParentPath = truncate(e.ParentPath, 1000)

	// Derive url_path if not set.
	if e.URLPath == "" {
		if u, err := url.Parse(e.URL); err == nil {
			e.URLPath = u.Path
		}
	}

	return nil
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen])
}
