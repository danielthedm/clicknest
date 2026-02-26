package ingest

import (
	"strings"
	"testing"
)

func validEvent() IngestEvent {
	return IngestEvent{
		EventType: "click",
		URL:       "https://example.com/page",
	}
}

func validPayload() IngestPayload {
	return IngestPayload{
		SessionID: "sess-abc123",
		Events:    []IngestEvent{validEvent()},
	}
}

func TestValidatePayload_Valid(t *testing.T) {
	p := validPayload()
	if err := ValidatePayload(&p); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidatePayload_EmptyBatch(t *testing.T) {
	p := validPayload()
	p.Events = nil
	if err := ValidatePayload(&p); err != ErrEmptyBatch {
		t.Fatalf("expected ErrEmptyBatch, got: %v", err)
	}
}

func TestValidatePayload_BatchTooLarge(t *testing.T) {
	p := validPayload()
	p.Events = make([]IngestEvent, 101)
	for i := range p.Events {
		p.Events[i] = validEvent()
	}
	if err := ValidatePayload(&p); err != ErrBatchTooLarge {
		t.Fatalf("expected ErrBatchTooLarge, got: %v", err)
	}
}

func TestValidatePayload_MissingSession(t *testing.T) {
	p := validPayload()
	p.SessionID = ""
	if err := ValidatePayload(&p); err != ErrMissingSession {
		t.Fatalf("expected ErrMissingSession, got: %v", err)
	}
}

func TestValidatePayload_MissingEventType(t *testing.T) {
	p := validPayload()
	p.Events[0].EventType = ""
	if err := ValidatePayload(&p); err != ErrMissingType {
		t.Fatalf("expected ErrMissingType, got: %v", err)
	}
}

func TestValidatePayload_InvalidEventType(t *testing.T) {
	p := validPayload()
	p.Events[0].EventType = "hover"
	if err := ValidatePayload(&p); err != ErrInvalidType {
		t.Fatalf("expected ErrInvalidType, got: %v", err)
	}
}

func TestValidatePayload_AllValidEventTypes(t *testing.T) {
	for _, et := range []string{"click", "pageview", "input", "submit", "custom", "error"} {
		p := validPayload()
		p.Events[0].EventType = et
		if err := ValidatePayload(&p); err != nil {
			t.Errorf("event_type %q should be valid, got: %v", et, err)
		}
	}
}

func TestValidatePayload_MissingURL(t *testing.T) {
	p := validPayload()
	p.Events[0].URL = ""
	if err := ValidatePayload(&p); err != ErrMissingURL {
		t.Fatalf("expected ErrMissingURL, got: %v", err)
	}
}

func TestValidatePayload_InvalidURL(t *testing.T) {
	p := validPayload()
	p.Events[0].URL = "not a url"
	if err := ValidatePayload(&p); err != ErrInvalidURL {
		t.Fatalf("expected ErrInvalidURL, got: %v", err)
	}
}

func TestValidatePayload_DerivesURLPath(t *testing.T) {
	p := validPayload()
	p.Events[0].URL = "https://example.com/some/path?q=1"
	p.Events[0].URLPath = ""
	if err := ValidatePayload(&p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Events[0].URLPath != "/some/path" {
		t.Fatalf("expected url_path=/some/path, got %q", p.Events[0].URLPath)
	}
}

func TestValidatePayload_TruncatesLongText(t *testing.T) {
	p := validPayload()
	p.Events[0].ElementText = strings.Repeat("x", 1000)
	if err := ValidatePayload(&p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len([]rune(p.Events[0].ElementText)) != maxTextLength {
		t.Fatalf("expected ElementText truncated to %d runes", maxTextLength)
	}
}

func TestTruncate_ShortString(t *testing.T) {
	s := truncate("hello", 10)
	if s != "hello" {
		t.Fatalf("short string should be unchanged, got %q", s)
	}
}

func TestTruncate_ExactLength(t *testing.T) {
	s := truncate("hello", 5)
	if s != "hello" {
		t.Fatalf("exact-length string should be unchanged, got %q", s)
	}
}

func TestTruncate_LongString(t *testing.T) {
	s := truncate(strings.Repeat("a", 600), 500)
	if len([]rune(s)) != 500 {
		t.Fatalf("expected 500 runes, got %d", len([]rune(s)))
	}
}

func TestTruncate_MultibyteChars(t *testing.T) {
	// Each emoji is 1 rune but multiple bytes — truncation must be rune-safe.
	input := strings.Repeat("😀", 600)
	s := truncate(input, 500)
	runes := []rune(s)
	if len(runes) != 500 {
		t.Fatalf("expected 500 runes, got %d", len(runes))
	}
	// Verify no broken multibyte sequences.
	for _, r := range s {
		if r == '😀' {
			continue
		}
		t.Fatalf("unexpected rune in truncated emoji string: %q", r)
	}
}

func TestTruncate_TrimsWhitespace(t *testing.T) {
	s := truncate("  hello  ", 100)
	if s != "hello" {
		t.Fatalf("expected whitespace trimmed, got %q", s)
	}
}

func TestTruncate_EmptyString(t *testing.T) {
	s := truncate("", 100)
	if s != "" {
		t.Fatalf("expected empty string, got %q", s)
	}
}
