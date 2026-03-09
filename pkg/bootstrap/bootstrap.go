// Package bootstrap provides a reusable initialization function for ClickNest.
// This allows the cloud binary to import and start ClickNest without duplicating
// the entire main.go initialization logic.
package bootstrap

import (
	"context"
	"io/fs"
	"log"
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

	// OnReady is called after the server is configured but before it starts listening.
	// Use this to inspect or modify startup behavior.
	OnReady func()
}

// Run initializes all ClickNest subsystems and starts the HTTP server.
// It blocks until the process receives SIGINT or SIGTERM, then shuts down gracefully.
func Run(cfg Config) {
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
	defer events.Close()

	enc, err := storage.NewEncryptor(cfg.DataDir)
	if err != nil {
		log.Fatalf("initializing encryption: %v", err)
	}
	log.Println("Encryption at rest enabled for API keys and tokens")

	meta, err := storage.NewSQLite(sqlitePath, enc)
	if err != nil {
		log.Fatalf("opening sqlite: %v", err)
	}
	defer meta.Close()

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
	defer namer.Close()

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

	// Start HTTP server.
	srv := server.New(server.Config{
		Addr:               cfg.Addr,
		DataDir:            cfg.DataDir,
		DevMode:            cfg.DevMode,
		WebFS:              cfg.WebFS,
		SDKJS:              cfg.SDKJS,
		GitHubClientID:     ghClientID,
		GitHubClientSecret: ghClientSecret,
	}, events, meta, namer, syncer, matcher, cfg.Registry)

	if cfg.OnReady != nil {
		cfg.OnReady()
	}

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.Start(); err != nil {
			log.Printf("server stopped: %v", err)
		}
	}()

	log.Printf("ClickNest started on %s (dev=%v, data=%s)", cfg.Addr, cfg.DevMode, cfg.DataDir)

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
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
