// Package bootstrap provides a reusable initialization function for ClickNest.
// This allows the cloud binary to import and start ClickNest without duplicating
// the entire main.go initialization logic.
package bootstrap

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "github.com/marcboeker/go-duckdb"
	_ "modernc.org/sqlite"

	"github.com/danielthedm/clicknest/internal/ai"
	ghub "github.com/danielthedm/clicknest/internal/github"
	"github.com/danielthedm/clicknest/internal/growth"
	"github.com/danielthedm/clicknest/internal/server"
	"github.com/danielthedm/clicknest/internal/storage"
)

// Config holds configuration for bootstrapping a ClickNest instance.
type Config struct {
	Addr    string
	DataDir string
	DevMode bool
	WebFS   fs.FS  // Embedded SvelteKit build (nil in dev mode)
	SDKJS   []byte // Embedded SDK JS bundle (nil in dev mode)

	// Registry allows callers to pre-register connectors before startup.
	Registry *growth.Registry

	// CloudMode tells the frontend this is a cloud-managed instance.
	CloudMode bool

	// RouteHook is called after OSS routes are registered. It receives
	// the shared HTTP mux and the metadata store so that EE code can
	// inject additional routes (billing, signup, instances) and access
	// the user/project database directly.
	RouteHook func(mux *http.ServeMux, meta *storage.SQLite)

	// ResourceLimitFn, if set, is consulted before creating metered resources
	// (campaigns, leads, etc.). Returns HTTP status + message when exceeded,
	// or 0/"" to allow. Nil means unlimited (self-hosted default).
	ResourceLimitFn func(ctx context.Context, projectID, metric string) (int, string)

	// RetentionDaysFn, if set, returns the data retention window in days for a project.
	// Return -1 for unlimited, or a positive number to delete older events.
	// When nil, the server uses a 365-day default.
	RetentionDaysFn func(ctx context.Context, projectID string) int

	// RateLimitFn, if set, returns per-project event ingestion rate limits (tokens/sec, burst).
	// Return rate <= 0 to disable rate limiting for the project (e.g. enterprise tier).
	// When nil, the default 10/s, 50 burst limits apply.
	RateLimitFn func(ctx context.Context, projectID string) (rate float64, burst int)

	// OnEventIngested, if set, is called after a successful event batch is written.
	// It receives the project ID and the number of events accepted.
	// Used by EE to increment the monthly usage counter in PostgreSQL.
	OnEventIngested func(ctx context.Context, projectID string, count int64)

	// OnReady is called after the server is configured but before it starts listening.
	// Use this to inspect or modify startup behavior.
	OnReady func()
}

// App holds initialized ClickNest subsystems.
type App struct {
	Meta   *storage.SQLite
	Events *storage.DuckDB
	Server *server.Server
	namer  *ai.Namer
}

// Setup initializes all ClickNest subsystems and returns an App.
// The caller must call App.Run() to start the server and App.Close()
// to release resources.
func Setup(cfg Config) *App {
	if cfg.Registry == nil {
		cfg.Registry = growth.NewRegistry()
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("creating data dir: %v", err)
	}

	// Open databases.
	duckdbPath := filepath.Join(cfg.DataDir, "events.duckdb")
	sqlitePath := filepath.Join(cfg.DataDir, "clicknest.db")

	events, err := storage.NewDuckDB(duckdbPath)
	if err != nil {
		log.Fatalf("opening duckdb: %v", err)
	}

	enc, err := storage.NewEncryptor(cfg.DataDir)
	if err != nil {
		log.Fatalf("initializing encryption: %v", err)
	}
	log.Println("Encryption at rest enabled for API keys and tokens")

	meta, err := storage.NewSQLite(sqlitePath, enc)
	if err != nil {
		log.Fatalf("opening sqlite: %v", err)
	}

	// Ensure a default project exists.
	ensureDefaultProject(meta)

	// Initialize AI naming pipeline.
	cache := ai.NewCache(meta)
	var provider ai.Provider
	project := getDefaultProject(meta)
	if project != nil {
		llmCfg, err := meta.GetLLMConfig(context.Background(), project.ID)
		if err == nil && llmCfg != nil {
			provider = ai.NewProviderFromConfig(llmCfg)
			if provider != nil {
				log.Printf("AI naming enabled: %s/%s", llmCfg.Provider, llmCfg.Model)
			}
		}
	}
	namer := ai.NewNamer(provider, cache, events, 2)

	if provider != nil && project != nil {
		go namer.Backfill(context.Background(), project.ID)
	}

	// Initialize GitHub integration.
	syncer := ghub.NewSyncer(meta)
	matcher := ghub.NewMatcher(meta)
	if project != nil {
		if _, err := meta.GetGitHubConnection(context.Background(), project.ID); err == nil {
			namer.SetMatcher(matcher)
			log.Printf("GitHub source matching enabled")
		}
	}

	// Read GitHub OAuth config from environment.
	ghClientID := os.Getenv("GITHUB_CLIENT_ID")
	ghClientSecret := os.Getenv("GITHUB_CLIENT_SECRET")

	// Create HTTP server.
	srv := server.New(server.Config{
		Addr:               cfg.Addr,
		DataDir:            cfg.DataDir,
		DevMode:            cfg.DevMode,
		WebFS:              cfg.WebFS,
		SDKJS:              cfg.SDKJS,
		GitHubClientID:     ghClientID,
		GitHubClientSecret: ghClientSecret,
		CloudMode:          cfg.CloudMode,
		RouteHook:          cfg.RouteHook,
		ResourceLimitFn:    cfg.ResourceLimitFn,
		RetentionDaysFn:    cfg.RetentionDaysFn,
		RateLimitFn:        cfg.RateLimitFn,
		OnEventIngested:    cfg.OnEventIngested,
	}, events, meta, namer, syncer, matcher, cfg.Registry)

	if cfg.OnReady != nil {
		cfg.OnReady()
	}

	return &App{
		Meta:   meta,
		Events: events,
		Server: srv,
		namer:  namer,
	}
}

// Run starts the HTTP server and blocks until SIGINT or SIGTERM,
// then shuts down gracefully.
func (a *App) Run() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := a.Server.Start(); err != nil {
			log.Printf("server stopped: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.Server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

// Close releases all resources held by the App.
func (a *App) Close() {
	a.namer.Close()
	a.Meta.Close()
	a.Events.Close()
}

func getDefaultProject(meta *storage.SQLite) *storage.Project {
	projects, err := meta.ListProjects(context.Background())
	if err != nil || len(projects) == 0 {
		return nil
	}
	return &projects[0]
}

func ensureDefaultProject(meta *storage.SQLite) {
	ctx := context.Background()
	projects, err := meta.ListProjects(ctx)
	if err != nil {
		log.Printf("WARN listing projects: %v", err)
		return
	}
	if len(projects) > 0 {
		log.Printf("Using project: %s (API key: %s)", projects[0].Name, projects[0].APIKey)
		return
	}

	project, err := meta.CreateProject(ctx, "default", "My App")
	if err != nil {
		log.Printf("WARN creating default project: %v", err)
		return
	}
	log.Printf("Created default project: %s (API key: %s)", project.Name, project.APIKey)
}
