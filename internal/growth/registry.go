package growth

import (
	"fmt"
	"sync"
)

// Registry holds registered Connector implementations.
type Registry struct {
	mu         sync.RWMutex
	connectors map[string]Connector
}

func NewRegistry() *Registry {
	return &Registry{connectors: make(map[string]Connector)}
}

// Register adds a connector. Panics on duplicate (Go stdlib pattern).
func (r *Registry) Register(c Connector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.connectors[c.Name()]; ok {
		panic(fmt.Sprintf("connector %q already registered", c.Name()))
	}
	r.connectors[c.Name()] = c
}

// Get returns a connector by name, or nil if not found.
func (r *Registry) Get(name string) Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connectors[name]
}

// List returns all registered connectors.
func (r *Registry) List() []Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Connector, 0, len(r.connectors))
	for _, c := range r.connectors {
		result = append(result, c)
	}
	return result
}
