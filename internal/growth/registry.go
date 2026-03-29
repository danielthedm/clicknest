package growth

import (
	"fmt"
	"sync"
)

// Registry holds registered Publisher and Source implementations.
type Registry struct {
	mu         sync.RWMutex
	publishers map[string]Publisher
	sources    map[string]Source
}

func NewRegistry() *Registry {
	return &Registry{
		publishers: make(map[string]Publisher),
		sources:    make(map[string]Source),
	}
}

// RegisterPublisher adds a publisher. Panics on duplicate.
func (r *Registry) RegisterPublisher(p Publisher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.publishers[p.Name()]; ok {
		panic(fmt.Sprintf("publisher %q already registered", p.Name()))
	}
	r.publishers[p.Name()] = p
}

// RegisterSource adds a source. Panics on duplicate.
func (r *Registry) RegisterSource(s Source) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sources[s.Name()]; ok {
		panic(fmt.Sprintf("source %q already registered", s.Name()))
	}
	r.sources[s.Name()] = s
}

// GetPublisher returns a publisher by name, or nil if not found.
func (r *Registry) GetPublisher(name string) Publisher {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.publishers[name]
}

// GetSource returns a source by name, or nil if not found.
func (r *Registry) GetSource(name string) Source {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sources[name]
}

// ListPublishers returns all registered publishers.
func (r *Registry) ListPublishers() []Publisher {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Publisher, 0, len(r.publishers))
	for _, p := range r.publishers {
		result = append(result, p)
	}
	return result
}

// ListSources returns all registered sources.
func (r *Registry) ListSources() []Source {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Source, 0, len(r.sources))
	for _, s := range r.sources {
		result = append(result, s)
	}
	return result
}
