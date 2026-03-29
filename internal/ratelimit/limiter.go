package ratelimit

import (
	"sync"
	"time"
)

// Limiter implements a per-key token bucket rate limiter.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second (sustained)
	burst   int     // max tokens (burst)
}

type bucket struct {
	tokens    float64
	lastCheck time.Time
	rate      float64 // effective rate for this bucket (0 = use limiter default)
	burst     int     // effective burst for this bucket (0 = use limiter default)
}

// New creates a Limiter that replenishes at rate tokens/sec up to burst max.
func New(rate float64, burst int) *Limiter {
	return &Limiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
	}
}

// Allow checks if a request for the given key should be allowed.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(l.burst), lastCheck: now}
		l.buckets[key] = b
	}

	elapsed := now.Sub(b.lastCheck).Seconds()
	b.lastCheck = now
	b.tokens += elapsed * l.rate
	if b.tokens > float64(l.burst) {
		b.tokens = float64(l.burst)
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// AllowRate is like Allow but uses the given rate and burst instead of the
// limiter's defaults. If the stored bucket was created with different limits,
// it is reset so the new config takes effect immediately.
func (l *Limiter) AllowRate(key string, rate float64, burst int) bool {
	// Unlimited: rate <= 0 means no throttling.
	if rate <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok || b.rate != rate || b.burst != burst {
		// New bucket or config changed — start full.
		b = &bucket{tokens: float64(burst), lastCheck: now, rate: rate, burst: burst}
		l.buckets[key] = b
	}

	elapsed := now.Sub(b.lastCheck).Seconds()
	b.lastCheck = now
	b.tokens += elapsed * rate
	if b.tokens > float64(burst) {
		b.tokens = float64(burst)
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Cleanup removes stale buckets that haven't been used in the given duration.
func (l *Limiter) Cleanup(maxAge time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for k, b := range l.buckets {
		if b.lastCheck.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}
