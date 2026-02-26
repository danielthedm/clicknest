package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// ComputeFingerprint generates a stable hash from DOM context for event dedup and naming.
// The fingerprint captures the structural identity of a UI element.
func ComputeFingerprint(elementTag, elementID, elementClasses, parentPath, urlPath string) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(elementTag)),
		strings.ToLower(strings.TrimSpace(elementID)),
		strings.ToLower(strings.TrimSpace(elementClasses)),
		strings.TrimSpace(parentPath),
		strings.TrimSpace(urlPath),
	}

	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte("|"))
		}
		h.Write([]byte(p))
	}

	return hex.EncodeToString(h.Sum(nil))[:16]
}
