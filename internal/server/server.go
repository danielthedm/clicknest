package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/danielleslie/clicknest/internal/ai"
	"github.com/danielleslie/clicknest/internal/auth"
	ghub "github.com/danielleslie/clicknest/internal/github"
	"github.com/danielleslie/clicknest/internal/ingest"
	"github.com/danielleslie/clicknest/internal/query"
	"github.com/danielleslie/clicknest/internal/storage"
)

type Config struct {
	Addr               string
	DataDir            string
	DevMode            bool
	WebFS              fs.FS  // Embedded SvelteKit build (nil in dev mode)
	SDKJS              []byte // Embedded SDK JS bundle (nil in dev mode)
	GitHubClientID     string // GitHub OAuth app client ID (enables OAuth when set)
	GitHubClientSecret string // GitHub OAuth app client secret
}

type Server struct {
	config   Config
	events   *storage.DuckDB
	meta     *storage.SQLite
	namer    *ai.Namer
	syncer   *ghub.Syncer
	matcher  *ghub.Matcher
	mux      *http.ServeMux
	server   *http.Server
}

func New(config Config, events *storage.DuckDB, meta *storage.SQLite, namer *ai.Namer, syncer *ghub.Syncer, matcher *ghub.Matcher) *Server {
	s := &Server{
		config:  config,
		events:  events,
		meta:    meta,
		namer:   namer,
		syncer:  syncer,
		matcher: matcher,
		mux:     http.NewServeMux(),
	}
	s.routes()
	s.server = &http.Server{
		Addr:         config.Addr,
		Handler:      CORS(s.mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s
}

func (s *Server) routes() {
	ingestHandler := ingest.NewHandler(s.events, s.namer)
	queryHandler := query.NewHandler(s.events, s.meta)

	apiKeyAuth := auth.APIKeyMiddleware(s.meta)
	sessionAuth := auth.SessionMiddleware(s.meta)

	// SDK ingestion endpoint (API key auth).
	s.mux.Handle("POST /api/v1/events", apiKeyAuth(ingestHandler))

	// Dashboard query endpoints (session auth).
	s.mux.Handle("GET /api/v1/events", sessionAuth(http.HandlerFunc(queryHandler.EventsHandler)))
	s.mux.Handle("GET /api/v1/events/stats", sessionAuth(http.HandlerFunc(queryHandler.EventStatsHandler)))
	s.mux.Handle("GET /api/v1/events/live", sessionAuth(http.HandlerFunc(s.liveEventsHandler)))
	s.mux.Handle("GET /api/v1/trends", sessionAuth(http.HandlerFunc(queryHandler.TrendsHandler)))
	s.mux.Handle("GET /api/v1/trends/breakdown", sessionAuth(http.HandlerFunc(queryHandler.TrendsBreakdownHandler)))
	s.mux.Handle("GET /api/v1/pages", sessionAuth(http.HandlerFunc(queryHandler.PagesHandler)))
	s.mux.Handle("GET /api/v1/sessions", sessionAuth(http.HandlerFunc(queryHandler.SessionsHandler)))
	s.mux.Handle("GET /api/v1/sessions/{id}", sessionAuth(http.HandlerFunc(queryHandler.SessionDetailHandler)))

	// Properties.
	s.mux.Handle("GET /api/v1/properties/keys", sessionAuth(http.HandlerFunc(queryHandler.PropertyKeysHandler)))
	s.mux.Handle("GET /api/v1/properties/values", sessionAuth(http.HandlerFunc(queryHandler.PropertyValuesHandler)))

	// Users.
	s.mux.Handle("GET /api/v1/users", sessionAuth(http.HandlerFunc(queryHandler.UsersHandler)))
	s.mux.Handle("GET /api/v1/users/{id}/events", sessionAuth(http.HandlerFunc(queryHandler.UserEventsHandler)))

	// Funnels.
	s.mux.Handle("GET /api/v1/funnels", sessionAuth(http.HandlerFunc(queryHandler.ListFunnelsHandler)))
	s.mux.Handle("POST /api/v1/funnels", sessionAuth(http.HandlerFunc(queryHandler.CreateFunnelHandler)))
	s.mux.Handle("GET /api/v1/funnels/{id}", sessionAuth(http.HandlerFunc(queryHandler.GetFunnelHandler)))
	s.mux.Handle("DELETE /api/v1/funnels/{id}", sessionAuth(http.HandlerFunc(queryHandler.DeleteFunnelHandler)))
	s.mux.Handle("GET /api/v1/funnels/{id}/results", sessionAuth(http.HandlerFunc(queryHandler.FunnelResultsHandler)))
	s.mux.Handle("GET /api/v1/funnels/{id}/cohorts", sessionAuth(http.HandlerFunc(queryHandler.FunnelCohortsHandler)))
	s.mux.Handle("POST /api/v1/funnels/suggest", sessionAuth(http.HandlerFunc(s.suggestFunnelsHandler)))

	// AI chat.
	s.mux.Handle("POST /api/v1/ai/chat", sessionAuth(http.HandlerFunc(s.aiChatHandler)))

	// Retention.
	s.mux.Handle("GET /api/v1/retention", sessionAuth(http.HandlerFunc(queryHandler.RetentionHandler)))

	// Dashboards.
	s.mux.Handle("GET /api/v1/dashboards", sessionAuth(http.HandlerFunc(queryHandler.ListDashboardsHandler)))
	s.mux.Handle("POST /api/v1/dashboards", sessionAuth(http.HandlerFunc(queryHandler.CreateDashboardHandler)))
	s.mux.Handle("GET /api/v1/dashboards/{id}", sessionAuth(http.HandlerFunc(queryHandler.GetDashboardHandler)))
	s.mux.Handle("PUT /api/v1/dashboards/{id}", sessionAuth(http.HandlerFunc(queryHandler.UpdateDashboardHandler)))
	s.mux.Handle("DELETE /api/v1/dashboards/{id}", sessionAuth(http.HandlerFunc(queryHandler.DeleteDashboardHandler)))

	// Event names.
	s.mux.Handle("GET /api/v1/names", sessionAuth(http.HandlerFunc(s.listNamesHandler)))
	s.mux.Handle("PUT /api/v1/names/{fp}", sessionAuth(http.HandlerFunc(s.overrideNameHandler)))

	// Project/settings endpoints.
	s.mux.Handle("GET /api/v1/project", sessionAuth(http.HandlerFunc(s.projectHandler)))
	s.mux.Handle("GET /api/v1/llm/config", sessionAuth(http.HandlerFunc(s.getLLMConfigHandler)))
	s.mux.Handle("PUT /api/v1/llm/config", sessionAuth(http.HandlerFunc(s.llmConfigHandler)))

	// GitHub integration.
	s.mux.Handle("GET /api/v1/github", sessionAuth(http.HandlerFunc(s.githubGetHandler)))
	s.mux.Handle("PUT /api/v1/github", sessionAuth(http.HandlerFunc(s.githubConnectHandler)))

	// Errors.
	s.mux.Handle("GET /api/v1/errors", sessionAuth(http.HandlerFunc(queryHandler.ErrorsHandler)))

	// Feature flags.
	s.mux.Handle("GET /api/v1/flags", sessionAuth(http.HandlerFunc(s.listFlagsHandler)))
	s.mux.Handle("POST /api/v1/flags", sessionAuth(http.HandlerFunc(s.createFlagHandler)))
	s.mux.Handle("PUT /api/v1/flags/{id}", sessionAuth(http.HandlerFunc(s.updateFlagHandler)))
	s.mux.Handle("DELETE /api/v1/flags/{id}", sessionAuth(http.HandlerFunc(s.deleteFlagHandler)))
	s.mux.Handle("GET /api/v1/flags/evaluate", apiKeyAuth(http.HandlerFunc(s.evaluateFlagsHandler)))

	// Alerts.
	s.mux.Handle("GET /api/v1/alerts", sessionAuth(http.HandlerFunc(s.listAlertsHandler)))
	s.mux.Handle("POST /api/v1/alerts", sessionAuth(http.HandlerFunc(s.createAlertHandler)))
	s.mux.Handle("PUT /api/v1/alerts/{id}", sessionAuth(http.HandlerFunc(s.updateAlertHandler)))
	s.mux.Handle("DELETE /api/v1/alerts/{id}", sessionAuth(http.HandlerFunc(s.deleteAlertHandler)))

	// Path analysis.
	s.mux.Handle("GET /api/v1/paths", sessionAuth(http.HandlerFunc(queryHandler.PathsHandler)))

	// Heatmap.
	s.mux.Handle("GET /api/v1/heatmap", sessionAuth(http.HandlerFunc(queryHandler.HeatmapHandler)))

	// Backup / restore.
	s.mux.Handle("GET /api/v1/export", sessionAuth(http.HandlerFunc(s.exportHandler)))
	s.mux.Handle("POST /api/v1/import", sessionAuth(http.HandlerFunc(s.importHandler)))

	// Storage stats.
	s.mux.Handle("GET /api/v1/storage", sessionAuth(http.HandlerFunc(s.storageHandler)))

	// GitHub OAuth (only functional when GITHUB_CLIENT_ID is set).
	s.mux.Handle("GET /api/v1/github/oauth/enabled", sessionAuth(http.HandlerFunc(s.githubOAuthEnabledHandler)))
	s.mux.Handle("GET /api/v1/github/oauth/authorize", sessionAuth(http.HandlerFunc(s.githubOAuthAuthorizeHandler)))
	s.mux.HandleFunc("GET /api/v1/github/oauth/callback", s.githubOAuthCallbackHandler) // No session auth — browser redirect from GitHub

	// Auth (no session required).
	s.mux.HandleFunc("GET /api/v1/auth/setup-required", s.setupRequiredHandler)
	s.mux.HandleFunc("POST /api/v1/auth/setup", s.setupHandler)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.loginHandler)
	s.mux.HandleFunc("POST /api/v1/auth/logout", s.logoutHandler)
	s.mux.Handle("GET /api/v1/auth/me", sessionAuth(http.HandlerFunc(s.meHandler)))

	// Health check.
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Test page for verifying the SDK works (dev mode only).
	if s.config.DevMode {
		s.mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, testPageHTML)
		})
	}

	// SDK JS file served at /sdk.js.
	s.mux.HandleFunc("GET /sdk.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		if s.config.SDKJS != nil {
			w.Write(s.config.SDKJS)
		} else {
			fmt.Fprint(w, "/* ClickNest SDK - build with 'make sdk' */")
		}
	})

	// SPA catch-all — serve embedded frontend or dev placeholder.
	if s.config.WebFS != nil {
		s.mux.Handle("/", SPAHandler(s.config.WebFS))
	} else {
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, devHTML)
		})
	}
}

func (s *Server) Start() error {
	log.Printf("ClickNest listening on %s", s.config.Addr)
	s.startAlertChecker()
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// --- Handler implementations ---

func (s *Server) liveEventsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Disable the server's WriteTimeout for this long-lived SSE connection.
	// Without this, the 30s WriteTimeout kills the connection, EventSource
	// reconnects, and stale connections exhaust the browser's 6-connection limit.
	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	lastCheck := time.Now().UTC()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			// SSE comment to keep connection alive and detect dead clients.
			if _, err := fmt.Fprint(w, ":heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			events, err := s.events.QueryEvents(r.Context(), storage.EventFilter{
				ProjectID: project.ID,
				StartTime: lastCheck,
				Limit:     50,
			})
			if err != nil {
				continue
			}
			lastCheck = time.Now().UTC()

			if len(events) > 0 {
				data, err := json.Marshal(events)
				if err != nil {
					return
				}
				if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

func (s *Server) listNamesHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	names, err := s.meta.ListEventNames(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"names": names})
}

func (s *Server) overrideNameHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	fp := r.PathValue("fp")
	if fp == "" {
		http.Error(w, `{"error":"fingerprint required"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	if err := s.meta.OverrideEventName(r.Context(), project.ID, fp, body.Name); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) projectHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

func (s *Server) getLLMConfigHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	cfg, err := s.meta.GetLLMConfig(r.Context(), project.ID)
	if err != nil {
		// No config saved yet — return empty.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"provider": "",
			"model":    "",
			"base_url": "",
			"api_key_set": false,
		})
		return
	}

	apiKeySet := cfg.APIKey != nil && *cfg.APIKey != ""
	baseURL := ""
	if cfg.BaseURL != nil {
		baseURL = *cfg.BaseURL
	}

	// Build a masked key hint like "sk-ant-...a1b2" for display.
	apiKeyHint := ""
	if apiKeySet && cfg.APIKey != nil {
		k := *cfg.APIKey
		if len(k) > 8 {
			// Show first segment up to the 2nd dash, then "...", then last 4 chars.
			prefix := k
			if idx := strings.Index(k[3:], "-"); idx >= 0 {
				prefix = k[:idx+4]
			} else if len(k) > 6 {
				prefix = k[:6]
			}
			apiKeyHint = prefix + "..." + k[len(k)-4:]
		} else {
			apiKeyHint = "••••"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"provider":     cfg.Provider,
		"model":        cfg.Model,
		"base_url":     baseURL,
		"api_key_set":  apiKeySet,
		"api_key_hint": apiKeyHint,
	})
}

func (s *Server) llmConfigHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var config storage.LLMConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	config.ProjectID = project.ID

	// If no new API key was provided, preserve the existing one.
	if config.APIKey == nil || *config.APIKey == "" {
		existing, err := s.meta.GetLLMConfig(r.Context(), project.ID)
		if err == nil && existing != nil {
			config.APIKey = existing.APIKey
		}
	}

	if err := s.meta.SetLLMConfig(r.Context(), config); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}

	// Hot-reload the AI provider so naming starts/updates without a restart.
	if s.namer != nil {
		provider := ai.NewProviderFromConfig(&config)
		s.namer.SetProvider(provider)
		if provider != nil {
			log.Printf("AI naming provider updated: %s/%s", config.Provider, config.Model)
			// Backfill any existing unnamed events with the new provider.
			go s.namer.Backfill(context.Background(), project.ID)
		} else {
			log.Printf("AI naming provider cleared")
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) suggestFunnelsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	cfg, err := s.meta.GetLLMConfig(r.Context(), project.ID)
	if err != nil || cfg.Provider == "" {
		http.Error(w, `{"error":"LLM not configured. Go to Settings to configure an AI provider."}`, http.StatusBadRequest)
		return
	}

	end := time.Now().UTC()
	start := end.Add(-30 * 24 * time.Hour)
	sequences, err := s.events.QueryTopSequences(r.Context(), project.ID, start, end, 20)
	if err != nil {
		log.Printf("ERROR querying top sequences: %v", err)
		http.Error(w, `{"error":"failed to query event sequences"}`, http.StatusInternalServerError)
		return
	}
	if len(sequences) == 0 {
		http.Error(w, `{"error":"Not enough event data to suggest funnels. Record more events first."}`, http.StatusBadRequest)
		return
	}

	suggestions, err := ai.SuggestFunnels(r.Context(), cfg, sequences)
	if err != nil {
		log.Printf("ERROR suggesting funnels: %v", err)
		http.Error(w, `{"error":"AI suggestion failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"suggestions": suggestions})
}

func (s *Server) aiChatHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	cfg, err := s.meta.GetLLMConfig(r.Context(), project.ID)
	if err != nil || cfg.Provider == "" {
		http.Error(w, `{"error":"LLM not configured. Go to Settings to add an AI provider."}`, http.StatusBadRequest)
		return
	}

	var body struct {
		Message string            `json:"message"`
		History []ai.ChatMessage  `json:"history"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	// Gather analytics context for the system prompt.
	now := time.Now().UTC()
	weekAgo := now.Add(-7 * 24 * time.Hour)
	monthAgo := now.Add(-30 * 24 * time.Hour)

	trendData, _ := s.events.QueryTrends(r.Context(), project.ID, "day", weekAgo, now)
	topPages, _ := s.events.QueryTopPages(r.Context(), project.ID, weekAgo, now, 10)
	topEvents, _ := s.events.QueryTopEventNames(r.Context(), project.ID, monthAgo, now, 10)

	systemMsg := buildAnalyticsSystemPrompt(trendData, topPages, topEvents)

	history := append(body.History, ai.ChatMessage{Role: "user", Content: body.Message})

	reply, err := ai.ChatWithHistory(r.Context(), cfg, systemMsg, history)
	if err != nil {
		log.Printf("ERROR ai chat: %v", err)
		http.Error(w, `{"error":"AI request failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"reply": reply})
}

func buildAnalyticsSystemPrompt(trends []storage.TrendPoint, pages []storage.PageStat, events []storage.EventNameStat) string {
	var b strings.Builder
	b.WriteString("You are an analytics assistant embedded in ClickNest, a product analytics dashboard. ")
	b.WriteString("You have access to real analytics data from the user's product. ")
	b.WriteString("Be concise, direct, and actionable. Use plain paragraphs — no markdown headers or bullet lists unless explicitly asked. ")
	b.WriteString("Focus on insights that help the user understand their product's performance and what to improve.\n\n")

	if len(trends) > 0 {
		total := int64(0)
		for _, p := range trends {
			total += p.Count
		}
		fmt.Fprintf(&b, "EVENT VOLUME (last 7 days): %d total events across %d days\n", total, len(trends))
		if len(trends) >= 2 {
			last := trends[len(trends)-1].Count
			prev := trends[len(trends)-2].Count
			if prev > 0 {
				pct := int64(100) * (last - prev) / prev
				fmt.Fprintf(&b, "Recent trend: %+d%% day-over-day\n", pct)
			}
		}
		b.WriteString("\n")
	}

	if len(pages) > 0 {
		b.WriteString("TOP PAGES (last 7 days):\n")
		for i, p := range pages {
			fmt.Fprintf(&b, "%d. %s — %d views, %d sessions\n", i+1, p.Path, p.Views, p.Sessions)
		}
		b.WriteString("\n")
	}

	if len(events) > 0 {
		b.WriteString("TOP NAMED EVENTS (last 30 days):\n")
		for i, e := range events {
			fmt.Fprintf(&b, "%d. %s — %d occurrences\n", i+1, e.Name, e.Count)
		}
		b.WriteString("\n")
	}

	b.WriteString("Answer questions about this data. Provide insights and concrete recommendations.")
	return b.String()
}

func (s *Server) githubGetHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	oauthEnabled := s.config.GitHubClientID != ""

	conn, err := s.meta.GetGitHubConnection(r.Context(), project.ID)
	if err != nil {
		// No connection configured yet.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"connected":     false,
			"oauth_enabled": oauthEnabled,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"connected":      true,
		"repo_owner":     conn.RepoOwner,
		"repo_name":      conn.RepoName,
		"default_branch": conn.DefaultBranch,
		"last_synced_at": conn.LastSyncedAt,
		"oauth_enabled":  oauthEnabled,
	})
}

func (s *Server) githubConnectHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var body struct {
		RepoOwner     string `json:"repo_owner"`
		RepoName      string `json:"repo_name"`
		AccessToken   string `json:"access_token"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	// If no token provided, reuse existing OAuth token from a prior connection.
	if body.AccessToken == "" {
		existing, err := s.meta.GetGitHubConnection(r.Context(), project.ID)
		if err == nil && existing.AccessToken != "" {
			body.AccessToken = existing.AccessToken
		}
	}
	if body.RepoOwner == "" || body.RepoName == "" || body.AccessToken == "" {
		http.Error(w, `{"error":"repo_owner, repo_name, and access_token are required"}`, http.StatusBadRequest)
		return
	}
	if body.DefaultBranch == "" {
		body.DefaultBranch = "main"
	}

	// Verify the token works by listing the repo root.
	client := ghub.NewClient(body.AccessToken)
	if _, err := client.ListDirectory(r.Context(), body.RepoOwner, body.RepoName, "", body.DefaultBranch); err != nil {
		http.Error(w, `{"error":"failed to access repo: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	conn := storage.GitHubConnection{
		ProjectID:     project.ID,
		RepoOwner:     body.RepoOwner,
		RepoName:      body.RepoName,
		AccessToken:   body.AccessToken,
		DefaultBranch: body.DefaultBranch,
	}
	if err := s.meta.SetGitHubConnection(r.Context(), conn); err != nil {
		http.Error(w, `{"error":"save failed"}`, http.StatusInternalServerError)
		return
	}

	// Wire the matcher into the naming pipeline.
	if s.namer != nil && s.matcher != nil {
		s.namer.SetMatcher(s.matcher)
	}

	// Trigger background sync.
	if s.syncer != nil {
		go func() {
			if err := s.syncer.SyncRepo(context.Background(), project.ID); err != nil {
				log.Printf("WARN github sync failed: %v", err)
			} else {
				log.Printf("GitHub repo %s/%s synced for project %s", body.RepoOwner, body.RepoName, project.ID)
			}
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// --- GitHub OAuth handlers ---

func (s *Server) githubOAuthEnabledHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"enabled": s.config.GitHubClientID != "",
	})
}

func (s *Server) githubOAuthAuthorizeHandler(w http.ResponseWriter, r *http.Request) {
	if s.config.GitHubClientID == "" {
		http.Error(w, `{"error":"oauth not configured"}`, http.StatusBadRequest)
		return
	}

	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Generate random state token.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		http.Error(w, `{"error":"failed to generate state"}`, http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(b)

	if err := s.meta.SetOAuthState(r.Context(), state, project.ID); err != nil {
		http.Error(w, `{"error":"failed to store state"}`, http.StatusInternalServerError)
		return
	}

	authorizeURL := fmt.Sprintf(
		"https://github.com/login/oauth/authorize?client_id=%s&state=%s&scope=repo",
		url.QueryEscape(s.config.GitHubClientID),
		url.QueryEscape(state),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": authorizeURL})
}

func (s *Server) githubOAuthCallbackHandler(w http.ResponseWriter, r *http.Request) {
	if s.config.GitHubClientID == "" {
		http.Error(w, "oauth not configured", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	// Validate state and get project ID.
	projectID, err := s.meta.ValidateOAuthState(r.Context(), state)
	if err != nil {
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	// Exchange code for access token.
	tokenURL := "https://github.com/login/oauth/access_token"
	form := url.Values{
		"client_id":     {s.config.GitHubClientID},
		"client_secret": {s.config.GitHubClientSecret},
		"code":          {code},
	}

	req, err := http.NewRequestWithContext(r.Context(), "POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "failed to exchange code", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read token response", http.StatusBadGateway)
		return
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		http.Error(w, "invalid token response", http.StatusBadGateway)
		return
	}
	if tokenResp.Error != "" || tokenResp.AccessToken == "" {
		log.Printf("GitHub OAuth error: %s — %s", tokenResp.Error, tokenResp.ErrorDesc)
		http.Error(w, `{"error":"GitHub denied the authorization request"}`, http.StatusBadRequest)
		return
	}

	// Validate the token by calling GitHub API.
	client := ghub.NewClient(tokenResp.AccessToken)
	if _, err := client.GetUser(r.Context()); err != nil {
		http.Error(w, "token validation failed", http.StatusBadRequest)
		return
	}

	// Store the token. We use a placeholder repo — user will configure repo details next.
	existing, _ := s.meta.GetGitHubConnection(r.Context(), projectID)
	conn := storage.GitHubConnection{
		ProjectID:     projectID,
		AccessToken:   tokenResp.AccessToken,
		DefaultBranch: "main",
	}
	if existing != nil {
		// Preserve existing repo details if already configured.
		conn.RepoOwner = existing.RepoOwner
		conn.RepoName = existing.RepoName
		conn.DefaultBranch = existing.DefaultBranch
	}
	if err := s.meta.SetGitHubConnection(r.Context(), conn); err != nil {
		http.Error(w, "failed to store connection", http.StatusInternalServerError)
		return
	}

	// Redirect to settings page.
	http.Redirect(w, r, "/settings?github=connected", http.StatusTemporaryRedirect)
}

// --- Feature Flag handlers ---

func (s *Server) listFlagsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	flags, err := s.meta.ListFeatureFlags(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"flags": flags})
}

func (s *Server) createFlagHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Key               string `json:"key"`
		Name              string `json:"name"`
		RolloutPercentage int    `json:"rollout_percentage"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == "" || body.Name == "" {
		http.Error(w, `{"error":"key and name are required"}`, http.StatusBadRequest)
		return
	}
	id, err := generateID()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	rollout := body.RolloutPercentage
	if rollout <= 0 {
		rollout = 100
	}
	flag := storage.FeatureFlag{
		ID:                id,
		ProjectID:         project.ID,
		Key:               body.Key,
		Name:              body.Name,
		Enabled:           true,
		RolloutPercentage: rollout,
	}
	if err := s.meta.CreateFeatureFlag(r.Context(), flag); err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(flag)
}

func (s *Server) updateFlagHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	var body struct {
		Enabled           bool `json:"enabled"`
		RolloutPercentage int  `json:"rollout_percentage"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := s.meta.UpdateFeatureFlag(r.Context(), project.ID, id, body.Enabled, body.RolloutPercentage); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) deleteFlagHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if err := s.meta.DeleteFeatureFlag(r.Context(), project.ID, id); err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) evaluateFlagsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	distinctID := r.URL.Query().Get("distinct_id")
	flags, err := s.meta.ListFeatureFlags(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	result := make(map[string]bool, len(flags))
	for _, f := range flags {
		if !f.Enabled {
			result[f.Key] = false
			continue
		}
		if f.RolloutPercentage >= 100 {
			result[f.Key] = true
			continue
		}
		h := fnv.New32a()
		h.Write([]byte(distinctID + ":" + f.ID))
		result[f.Key] = int(h.Sum32()%100) < f.RolloutPercentage
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"flags": result})
}

// --- Alert handlers ---

func (s *Server) listAlertsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	alerts, err := s.meta.ListAlerts(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"alerts": alerts})
}

func (s *Server) createAlertHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Name          string `json:"name"`
		Metric        string `json:"metric"`
		EventName     string `json:"event_name"`
		Threshold     int    `json:"threshold"`
		WindowMinutes int    `json:"window_minutes"`
		WebhookURL    string `json:"webhook_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.Metric == "" || body.WebhookURL == "" {
		http.Error(w, `{"error":"name, metric, and webhook_url are required"}`, http.StatusBadRequest)
		return
	}
	if body.WindowMinutes <= 0 {
		body.WindowMinutes = 60
	}
	id, err := generateID()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	alert := storage.Alert{
		ID:            id,
		ProjectID:     project.ID,
		Name:          body.Name,
		Metric:        body.Metric,
		EventName:     body.EventName,
		Threshold:     body.Threshold,
		WindowMinutes: body.WindowMinutes,
		WebhookURL:    body.WebhookURL,
		Enabled:       true,
	}
	if err := s.meta.CreateAlert(r.Context(), alert); err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(alert)
}

func (s *Server) updateAlertHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	var body struct {
		Enabled    bool   `json:"enabled"`
		Threshold  int    `json:"threshold"`
		WebhookURL string `json:"webhook_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := s.meta.UpdateAlert(r.Context(), project.ID, id, body.Enabled, body.Threshold, body.WebhookURL); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) deleteAlertHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if err := s.meta.DeleteAlert(r.Context(), project.ID, id); err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Alert checker ---

func (s *Server) startAlertChecker() {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for range ticker.C {
			s.checkAlerts(context.Background())
		}
	}()
}

func (s *Server) checkAlerts(ctx context.Context) {
	alerts, err := s.meta.ListAllEnabledAlerts(ctx)
	if err != nil {
		log.Printf("WARN alert checker: failed to list alerts: %v", err)
		return
	}
	for _, a := range alerts {
		since := time.Now().UTC().Add(-time.Duration(a.WindowMinutes) * time.Minute)
		var eventType, eventName string
		switch a.Metric {
		case "error_count":
			eventType = "error"
		case "pageview_count":
			eventType = "pageview"
		case "event_count":
			eventName = a.EventName
		}
		count, err := s.events.CountEvents(ctx, a.ProjectID, eventType, eventName, since)
		if err != nil {
			log.Printf("WARN alert checker: count failed for alert %s: %v", a.ID, err)
			continue
		}
		if count <= int64(a.Threshold) {
			continue
		}
		// Cooldown: don't re-fire within the same window.
		if a.LastTriggeredAt != nil {
			if time.Since(*a.LastTriggeredAt) < time.Duration(a.WindowMinutes)*time.Minute {
				continue
			}
		}
		// Fire webhook.
		payload, _ := json.Marshal(map[string]any{
			"alert":      a.Name,
			"metric":     a.Metric,
			"count":      count,
			"threshold":  a.Threshold,
			"project_id": a.ProjectID,
		})
		req, err := http.NewRequestWithContext(ctx, "POST", a.WebhookURL, bytes.NewReader(payload))
		if err != nil {
			log.Printf("WARN alert %s: failed to build webhook request: %v", a.Name, err)
		} else {
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("WARN alert %s: webhook delivery failed: %v", a.Name, err)
			} else {
				resp.Body.Close()
				log.Printf("INFO alert %s fired: count=%d threshold=%d", a.Name, count, a.Threshold)
			}
		}
		now := time.Now().UTC()
		if err := s.meta.UpdateAlertTriggered(ctx, a.ID, now); err != nil {
			log.Printf("WARN alert checker: failed to update last_triggered_at: %v", err)
		}
	}
}

func (s *Server) storageHandler(w http.ResponseWriter, r *http.Request) {
	type storageInfo struct {
		EventsBytes int64 `json:"events_bytes"`
		MetaBytes   int64 `json:"meta_bytes"`
		TotalBytes  int64 `json:"total_bytes"`
		VolumeBytes int64 `json:"volume_bytes"`
		FreeBytes   int64 `json:"free_bytes"`
	}

	info := storageInfo{}

	fileSize := func(path string) int64 {
		fi, err := os.Stat(path)
		if err != nil {
			return 0
		}
		return fi.Size()
	}

	eventsBase := filepath.Join(s.config.DataDir, "events.duckdb")
	info.EventsBytes = fileSize(eventsBase) + fileSize(eventsBase+".wal")

	metaBase := filepath.Join(s.config.DataDir, "clicknest.db")
	info.MetaBytes = fileSize(metaBase) + fileSize(metaBase+"-wal") + fileSize(metaBase+"-shm")

	info.TotalBytes = info.EventsBytes + info.MetaBytes

	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.config.DataDir, &stat); err == nil {
		blockSize := int64(stat.Bsize)
		info.VolumeBytes = int64(stat.Blocks) * blockSize
		info.FreeBytes = int64(stat.Bavail) * blockSize
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

const devHTML = `<!DOCTYPE html>
<html>
<head><title>ClickNest</title></head>
<body>
<h1>ClickNest - Development Mode</h1>
<p>The dashboard is not yet built. Run <code>make web</code> to build the SvelteKit frontend.</p>
<p>API endpoints are available at <code>/api/v1/</code>.</p>
<p><a href="/test">SDK Test Page</a> | <a href="/api/health">Health Check</a></p>
</body>
</html>`

const testPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>ClickNest SDK Test Page</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; max-width: 600px; margin: 40px auto; padding: 0 20px; color: #1a1a1a; }
    h1 { margin-bottom: 8px; }
    p.subtitle { color: #666; margin-bottom: 32px; }
    .card { border: 1px solid #e0e0e0; border-radius: 8px; padding: 20px; margin-bottom: 16px; }
    .card h2 { font-size: 16px; margin-bottom: 12px; }
    button { padding: 8px 16px; border: none; border-radius: 6px; cursor: pointer; font-size: 14px; margin: 4px; }
    .btn-primary { background: #2563eb; color: white; }
    .btn-secondary { background: #e5e7eb; color: #374151; }
    .btn-danger { background: #dc2626; color: white; }
    input, select { padding: 8px 12px; border: 1px solid #d1d5db; border-radius: 6px; font-size: 14px; margin: 4px; }
    form { display: flex; flex-direction: column; gap: 8px; }
    #log { background: #f8f9fa; border: 1px solid #e0e0e0; border-radius: 8px; padding: 16px; margin-top: 24px; font-family: monospace; font-size: 12px; max-height: 300px; overflow-y: auto; white-space: pre-wrap; }
    #status { padding: 8px 12px; border-radius: 6px; margin-bottom: 24px; font-size: 13px; }
    #status.ok { background: #dcfce7; color: #166534; }
    #status.err { background: #fee2e2; color: #991b1b; }
    #status.loading { background: #f3f4f6; color: #6b7280; }
    nav { display: flex; gap: 8px; margin-bottom: 24px; }
    nav a { color: #2563eb; text-decoration: none; padding: 4px 8px; border-radius: 4px; }
    nav a:hover { background: #eff6ff; }
  </style>
</head>
<body>
  <h1>ClickNest Test Page</h1>
  <p class="subtitle">Interact with elements below to generate analytics events</p>
  <div id="status" class="loading">Connecting to ClickNest...</div>
  <nav>
    <a href="#home">Home</a>
    <a href="#products">Products</a>
    <a href="#checkout">Checkout</a>
    <a href="#account">Account</a>
  </nav>
  <div class="card">
    <h2>Actions</h2>
    <button id="add-to-cart" class="btn-primary" aria-label="Add to cart">Add to Cart</button>
    <button id="checkout-submit" class="btn-primary" aria-label="Submit checkout form">Place Order</button>
    <button id="save-settings" class="btn-secondary">Save Settings</button>
    <button id="delete-account" class="btn-danger" data-action="destructive">Delete Account</button>
  </div>
  <div class="card">
    <h2>Form Test</h2>
    <form id="signup-form" aria-label="Sign up form">
      <input type="text" id="username" placeholder="Username" aria-label="Username" />
      <input type="email" id="email" placeholder="Email" aria-label="Email address" />
      <select id="plan" aria-label="Select plan">
        <option value="free">Free</option>
        <option value="pro">Pro</option>
        <option value="enterprise">Enterprise</option>
      </select>
      <button type="submit" class="btn-primary" id="signup-btn">Sign Up</button>
    </form>
  </div>
  <div id="log">Waiting for events...</div>
  <script>
    const HOST = window.location.origin;
    const statusEl = document.getElementById('status');
    const logEl = document.getElementById('log');
    function logEvent(msg) {
      const time = new Date().toLocaleTimeString();
      logEl.textContent = '[' + time + '] ' + msg + '\n' + logEl.textContent;
    }
    async function init() {
      try {
        const resp = await fetch(HOST + '/api/v1/project');
        if (!resp.ok) throw new Error('Server returned ' + resp.status);
        const project = await resp.json();
        statusEl.className = 'ok';
        statusEl.textContent = 'Connected to "' + project.name + '" \u2014 API key: ' + project.api_key;
        const script = document.createElement('script');
        script.src = HOST + '/sdk.js';
        script.dataset.apiKey = project.api_key;
        script.dataset.host = HOST;
        document.head.appendChild(script);
        logEvent('SDK loaded, autocapture active. Click around!');
      } catch (e) {
        statusEl.className = 'err';
        statusEl.textContent = 'Failed to connect: ' + e.message + '. Is the server running?';
      }
    }
    init();
    document.addEventListener('click', function(e) {
      if (e.target.tagName === 'BUTTON' || e.target.tagName === 'A') {
        logEvent('click: <' + e.target.tagName.toLowerCase() + '> "' + e.target.textContent.trim() + '" #' + e.target.id);
      }
    });
    document.getElementById('signup-form').addEventListener('submit', function(e) {
      e.preventDefault();
      logEvent('submit: signup-form');
    });
  </script>
</body>
</html>`
