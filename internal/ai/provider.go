package ai

import (
	"context"

	"github.com/danielleslie/clicknest/internal/storage"
)

// NamingRequest contains the context needed to generate an event name.
type NamingRequest struct {
	ElementTag     string
	ElementID      string
	ElementClasses string
	ElementText    string
	AriaLabel      string
	ParentPath     string
	URL            string
	URLPath        string
	PageTitle      string
	SourceCode     string // matched source code snippet (from GitHub)
	SourceFile     string // matched source file path
}

// NamingResult contains the AI-generated name and metadata.
type NamingResult struct {
	Name       string
	Confidence float64
	SourceFile string
}

// Provider defines the interface for LLM backends.
type Provider interface {
	// GenerateEventName takes DOM/source context and returns a human-readable name.
	GenerateEventName(ctx context.Context, req NamingRequest) (*NamingResult, error)
}

// NewProviderFromConfig creates the appropriate Provider from a stored LLM configuration.
// Returns nil if the config is nil or the provider is empty/unknown.
func NewProviderFromConfig(cfg *storage.LLMConfig) Provider {
	if cfg == nil || cfg.Provider == "" {
		return nil
	}

	apiKey := ""
	if cfg.APIKey != nil {
		apiKey = *cfg.APIKey
	}
	baseURL := ""
	if cfg.BaseURL != nil {
		baseURL = *cfg.BaseURL
	}

	switch cfg.Provider {
	case "openai":
		return NewOpenAI(apiKey, cfg.Model, baseURL)
	case "anthropic":
		return NewAnthropic(apiKey, cfg.Model, baseURL)
	case "ollama":
		return NewOllama(cfg.Model, baseURL)
	default:
		return nil
	}
}
