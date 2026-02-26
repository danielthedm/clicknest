package ai

import (
	"context"
	"log"
	"sync"

	"github.com/danielleslie/clicknest/internal/storage"
)

// SourceMatcher finds source code for DOM elements (optional, used when GitHub is connected).
type SourceMatcher interface {
	MatchAndFetch(ctx context.Context, projectID, elementID, elementClasses, parentPath string) (sourceCode, sourceFile string, ok bool)
}

// NamingJob represents a pending event naming task.
type NamingJob struct {
	ProjectID   string
	Fingerprint string
	Request     NamingRequest
}

// Namer orchestrates the AI event naming pipeline.
// It maintains a worker pool that processes unnamed fingerprints asynchronously.
type Namer struct {
	mu       sync.RWMutex
	provider Provider
	matcher  SourceMatcher
	cache    *Cache
	events   *storage.DuckDB
	jobs     chan NamingJob
	wg       sync.WaitGroup
}

// NewNamer creates a naming orchestrator with the given number of workers.
func NewNamer(provider Provider, cache *Cache, events *storage.DuckDB, workers int) *Namer {
	if workers <= 0 {
		workers = 2
	}
	n := &Namer{
		provider: provider,
		cache:    cache,
		events:   events,
		jobs:     make(chan NamingJob, 1000),
	}

	for i := 0; i < workers; i++ {
		n.wg.Add(1)
		go n.worker()
	}

	return n
}

// SetProvider swaps the LLM provider at runtime (e.g. when settings change).
func (n *Namer) SetProvider(p Provider) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.provider = p
}

// SetMatcher sets the source code matcher (called when GitHub is connected).
func (n *Namer) SetMatcher(m SourceMatcher) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.matcher = m
}

// Submit queues a naming job if the fingerprint isn't already cached.
func (n *Namer) Submit(ctx context.Context, job NamingJob) {
	n.mu.RLock()
	noProvider := n.provider == nil
	n.mu.RUnlock()
	if noProvider {
		return
	}
	if _, ok := n.cache.Get(ctx, job.ProjectID, job.Fingerprint); ok {
		return
	}
	select {
	case n.jobs <- job:
	default:
		// Queue full, drop the job — it will be retried on next event.
	}
}

// Backfill queues naming jobs for all existing unnamed fingerprints in a project.
func (n *Namer) Backfill(ctx context.Context, projectID string) {
	n.mu.RLock()
	noProvider := n.provider == nil
	n.mu.RUnlock()
	if noProvider {
		return
	}

	events, err := n.events.UnnamedFingerprints(ctx, projectID)
	if err != nil {
		log.Printf("WARN backfill query: %v", err)
		return
	}

	queued := 0
	for _, e := range events {
		if _, ok := n.cache.Get(ctx, e.ProjectID, e.Fingerprint); ok {
			continue
		}
		select {
		case n.jobs <- NamingJob{
			ProjectID:   e.ProjectID,
			Fingerprint: e.Fingerprint,
			Request: NamingRequest{
				ElementTag:     e.ElementTag,
				ElementID:      e.ElementID,
				ElementClasses: e.ElementClasses,
				ElementText:    e.ElementText,
				AriaLabel:      e.AriaLabel,
				ParentPath:     e.ParentPath,
				URL:            e.URL,
				URLPath:        e.URLPath,
				PageTitle:      e.PageTitle,
			},
		}:
			queued++
		default:
			log.Printf("WARN backfill queue full, queued %d/%d", queued, len(events))
			return
		}
	}
	if queued > 0 {
		log.Printf("Backfill: queued %d unnamed fingerprints for naming", queued)
	}
}

// Close shuts down the naming workers.
func (n *Namer) Close() {
	close(n.jobs)
	n.wg.Wait()
}

func (n *Namer) worker() {
	defer n.wg.Done()
	for job := range n.jobs {
		ctx := context.Background()

		// Double-check cache.
		if _, ok := n.cache.Get(ctx, job.ProjectID, job.Fingerprint); ok {
			continue
		}

		n.mu.RLock()
		provider := n.provider
		matcher := n.matcher
		n.mu.RUnlock()
		if provider == nil {
			continue
		}

		// Enrich with source code if GitHub is connected.
		req := job.Request
		if matcher != nil {
			if code, file, ok := matcher.MatchAndFetch(ctx, job.ProjectID, req.ElementID, req.ElementClasses, req.ParentPath); ok {
				req.SourceCode = code
				req.SourceFile = file
			}
		}

		result, err := provider.GenerateEventName(ctx, req)
		if err != nil {
			log.Printf("WARN naming event %s: %v", job.Fingerprint, err)
			continue
		}

		if err := n.cache.Set(ctx, job.ProjectID, job.Fingerprint, result); err != nil {
			log.Printf("WARN caching name for %s: %v", job.Fingerprint, err)
			continue
		}

		// Backfill existing events with the new name.
		name := result.Name
		if err := n.events.BackfillEventName(ctx, job.ProjectID, job.Fingerprint, name); err != nil {
			log.Printf("WARN backfilling name for %s: %v", job.Fingerprint, err)
		}
	}
}
