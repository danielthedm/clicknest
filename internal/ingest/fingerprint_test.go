package ingest

import (
	"strings"
	"testing"
)

func TestComputeFingerprint_Deterministic(t *testing.T) {
	a := ComputeFingerprint("button", "submit-btn", "btn btn-primary", "form>div", "/checkout")
	b := ComputeFingerprint("button", "submit-btn", "btn btn-primary", "form>div", "/checkout")
	if a != b {
		t.Fatalf("same inputs produced different fingerprints: %q vs %q", a, b)
	}
}

func TestComputeFingerprint_Length(t *testing.T) {
	fp := ComputeFingerprint("button", "id", "class", "parent", "/path")
	if len(fp) != 16 {
		t.Fatalf("expected 16 chars, got %d: %q", len(fp), fp)
	}
}

func TestComputeFingerprint_CaseInsensitive(t *testing.T) {
	lower := ComputeFingerprint("button", "submit-btn", "btn primary", "form", "/")
	upper := ComputeFingerprint("BUTTON", "SUBMIT-BTN", "BTN PRIMARY", "form", "/")
	if lower != upper {
		t.Fatalf("case should be normalized: %q vs %q", lower, upper)
	}
}

func TestComputeFingerprint_WhitespaceTrimmed(t *testing.T) {
	clean := ComputeFingerprint("button", "id", "class", "parent", "/path")
	padded := ComputeFingerprint("  button  ", "  id  ", "  class  ", "  parent  ", "  /path  ")
	if clean != padded {
		t.Fatalf("whitespace should be trimmed: %q vs %q", clean, padded)
	}
}

func TestComputeFingerprint_Distinct(t *testing.T) {
	a := ComputeFingerprint("button", "id1", "class", "parent", "/page")
	b := ComputeFingerprint("button", "id2", "class", "parent", "/page")
	if a == b {
		t.Fatal("different inputs should produce different fingerprints")
	}
}

func TestComputeFingerprint_EmptyInputs(t *testing.T) {
	fp := ComputeFingerprint("", "", "", "", "")
	if len(fp) != 16 {
		t.Fatalf("empty inputs should still produce 16-char fingerprint, got %d", len(fp))
	}
}

func TestComputeFingerprint_IsHex(t *testing.T) {
	fp := ComputeFingerprint("a", "b", "c", "d", "e")
	for _, c := range fp {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Fatalf("fingerprint contains non-hex char %q: %q", c, fp)
		}
	}
}
