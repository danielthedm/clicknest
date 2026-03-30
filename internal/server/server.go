package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/danielthedm/clicknest/internal/ai"
	"github.com/danielthedm/clicknest/internal/auth"
	ghub "github.com/danielthedm/clicknest/internal/github"
	"github.com/danielthedm/clicknest/internal/growth"
	"github.com/danielthedm/clicknest/internal/ingest"
	"github.com/danielthedm/clicknest/internal/query"
	"github.com/danielthedm/clicknest/internal/ratelimit"
	"github.com/danielthedm/clicknest/internal/storage"
)

type Config struct {
	Addr               string
	DataDir            string
	DevMode            bool
	WebFS              fs.FS  // Embedded SvelteKit build (nil in dev mode)
	SDKJS              []byte // Embedded SDK JS bundle (nil in dev mode)
	GitHubClientID     string // GitHub OAuth app client ID (enables OAuth when set)
	GitHubClientSecret string // GitHub OAuth app client secret
	CloudMode          bool   // True when running as a managed cloud instance

	// Single-tenant cloud instance fields. When ControlPlaneURL is set,
	// this instance is a dedicated customer instance managed by the control plane.
	ControlPlaneURL string // e.g. "https://api.clicknest.app"
	InstanceID      string // UUID from the control plane
	InstanceSecret  string // shared secret for control plane ↔ instance auth

	// RouteHook is called at the end of route setup. EE code uses this
	// to inject billing, signup, and instance routes into the shared mux.
	RouteHook func(mux *http.ServeMux, meta *storage.SQLite)

	// ResourceLimitFn, if set, is consulted before creating metered resources.
	// It returns an HTTP status code and error message if the limit is exceeded,
	// or 0 and "" to allow the request. Nil means unlimited (self-hosted mode).
	ResourceLimitFn func(ctx context.Context, projectID, metric string) (int, string)

	// RetentionDaysFn, if set, returns the data retention window in days for a project.
	// Return -1 for unlimited retention, or a positive number of days to delete older events.
	// When nil, the server uses a 365-day default for all projects.
	RetentionDaysFn func(ctx context.Context, projectID string) int

	// RateLimitFn, if set, returns per-project event ingestion rate limits (tokens/sec, burst).
	// Return rate <= 0 to disable rate limiting for the project (e.g. enterprise tier).
	// When nil, the default 10/s, 50 burst limits apply.
	RateLimitFn func(ctx context.Context, projectID string) (rate float64, burst int)

	// OnEventIngested, if set, is called after a successful event batch is written to DuckDB.
	// It receives the project ID and the number of events accepted.
	// Used by EE to increment the monthly usage counter in PostgreSQL.
	OnEventIngested func(ctx context.Context, projectID string, count int64)

	// MaxConcurrentQueries is the maximum number of concurrent DuckDB analytics queries
	// allowed per project. 0 means unlimited. Default applied in New() if unset.
	MaxConcurrentQueries int
}

type Server struct {
	config       Config
	events       *storage.DuckDB
	meta         *storage.SQLite
	namer        *ai.Namer
	syncer       *ghub.Syncer
	matcher      *ghub.Matcher
	registry     *growth.Registry
	eventLimiter *ratelimit.Limiter
	querySlots   sync.Map // projectID → chan struct{} (semaphore)
	mux          *http.ServeMux
	server       *http.Server
}

func New(config Config, events *storage.DuckDB, meta *storage.SQLite, namer *ai.Namer, syncer *ghub.Syncer, matcher *ghub.Matcher, registry *growth.Registry) *Server {
	if config.MaxConcurrentQueries == 0 {
		config.MaxConcurrentQueries = 5
	}
	s := &Server{
		config:       config,
		events:       events,
		meta:         meta,
		namer:        namer,
		syncer:       syncer,
		matcher:      matcher,
		registry:     registry,
		eventLimiter: ratelimit.New(10, 50),
		mux:          http.NewServeMux(),
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
	if s.config.OnEventIngested != nil {
		fn := s.config.OnEventIngested
		ingestHandler.OnIngested = func(projectID string, count int64) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			fn(ctx, projectID, count)
		}
	}
	queryHandler := query.NewHandler(s.events, s.meta)
	queryHandler.SetMatcher(s.matcher)

	apiKeyAuth := auth.APIKeyMiddleware(s.meta)
	sessionAuth := auth.SessionMiddleware(s.meta)

	// SDK ingestion endpoint (API key auth + rate limiting).
	rateLimitedIngest := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project := auth.ProjectFromContext(r.Context())
		if project != nil {
			allowed := false
			if s.config.RateLimitFn != nil {
				rate, burst := s.config.RateLimitFn(r.Context(), project.ID)
				allowed = s.eventLimiter.AllowRate(project.ID, rate, burst)
			} else {
				allowed = s.eventLimiter.Allow(project.ID)
			}
			if !allowed {
				w.Header().Set("Retry-After", "1")
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
		}
		ingestHandler.ServeHTTP(w, r)
	})
	s.mux.Handle("POST /api/v1/events", apiKeyAuth(rateLimitedIngest))

	// Inbound lead ingestion (API key auth). External services like Gojiberry,
	// Typeform, etc. can POST leads here. Creates synthetic events so the
	// existing lead scoring system picks them up automatically.
	s.mux.Handle("POST /api/v1/leads/ingest", apiKeyAuth(http.HandlerFunc(s.ingestLeadsHandler)))

	// Dashboard query endpoints (session auth + per-project concurrent query limit).
	ql := s.withQueryLimit
	s.mux.Handle("GET /api/v1/events", sessionAuth(ql(http.HandlerFunc(queryHandler.EventsHandler))))
	s.mux.Handle("GET /api/v1/events/stats", sessionAuth(ql(http.HandlerFunc(queryHandler.EventStatsHandler))))
	s.mux.Handle("GET /api/v1/events/live", sessionAuth(http.HandlerFunc(s.liveEventsHandler)))
	s.mux.Handle("GET /api/v1/trends", sessionAuth(ql(http.HandlerFunc(queryHandler.TrendsHandler))))
	s.mux.Handle("GET /api/v1/trends/breakdown", sessionAuth(ql(http.HandlerFunc(queryHandler.TrendsBreakdownHandler))))
	s.mux.Handle("GET /api/v1/pages", sessionAuth(ql(http.HandlerFunc(queryHandler.PagesHandler))))
	s.mux.Handle("GET /api/v1/sessions", sessionAuth(ql(http.HandlerFunc(queryHandler.SessionsHandler))))
	s.mux.Handle("GET /api/v1/sessions/{id}", sessionAuth(ql(http.HandlerFunc(queryHandler.SessionDetailHandler))))

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
	s.mux.Handle("GET /api/v1/funnels/{id}/results", sessionAuth(ql(http.HandlerFunc(queryHandler.FunnelResultsHandler))))
	s.mux.Handle("GET /api/v1/funnels/{id}/cohorts", sessionAuth(ql(http.HandlerFunc(queryHandler.FunnelCohortsHandler))))
	s.mux.Handle("POST /api/v1/funnels/suggest", sessionAuth(http.HandlerFunc(s.suggestFunnelsHandler)))

	// AI chat.
	s.mux.Handle("POST /api/v1/ai/chat", sessionAuth(http.HandlerFunc(s.aiChatHandler)))

	// Retention.
	s.mux.Handle("GET /api/v1/retention", sessionAuth(ql(http.HandlerFunc(queryHandler.RetentionHandler))))

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
	s.mux.Handle("PUT /api/v1/project/description", sessionAuth(http.HandlerFunc(s.updateProjectDescriptionHandler)))
	s.mux.Handle("GET /api/v1/llm/config", sessionAuth(http.HandlerFunc(s.getLLMConfigHandler)))
	s.mux.Handle("PUT /api/v1/llm/config", sessionAuth(http.HandlerFunc(s.llmConfigHandler)))

	// GitHub integration.
	s.mux.Handle("GET /api/v1/github", sessionAuth(http.HandlerFunc(s.githubGetHandler)))
	s.mux.Handle("PUT /api/v1/github", sessionAuth(http.HandlerFunc(s.githubConnectHandler)))

	// Errors.
	s.mux.Handle("GET /api/v1/errors", sessionAuth(http.HandlerFunc(queryHandler.ErrorGroupsHandler)))
	s.mux.Handle("GET /api/v1/errors/detail", sessionAuth(http.HandlerFunc(queryHandler.ErrorDetailHandler)))

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
	s.mux.Handle("GET /api/v1/paths", sessionAuth(ql(http.HandlerFunc(queryHandler.PathsHandler))))

	// Heatmap.
	s.mux.Handle("GET /api/v1/heatmap", sessionAuth(ql(http.HandlerFunc(queryHandler.HeatmapHandler))))

	// Attribution.
	s.mux.Handle("GET /api/v1/attribution", sessionAuth(ql(http.HandlerFunc(queryHandler.AttributionHandler))))
	s.mux.Handle("GET /api/v1/attribution/sources", sessionAuth(ql(http.HandlerFunc(queryHandler.AttributionSourcesHandler))))

	// Ref codes.
	s.mux.Handle("GET /api/v1/refcodes", sessionAuth(http.HandlerFunc(s.listRefCodesHandler)))
	s.mux.Handle("POST /api/v1/refcodes", sessionAuth(http.HandlerFunc(s.createRefCodeHandler)))
	s.mux.Handle("PUT /api/v1/refcodes/{id}", sessionAuth(http.HandlerFunc(s.updateRefCodeHandler)))
	s.mux.Handle("DELETE /api/v1/refcodes/{id}", sessionAuth(http.HandlerFunc(s.deleteRefCodeHandler)))

	// Lead scoring (with optional per-project lead-count limit check).
	s.mux.Handle("GET /api/v1/leads", sessionAuth(s.leadLimitCheck(http.HandlerFunc(queryHandler.LeadScoresHandler))))

	// Scoring rules.
	s.mux.Handle("GET /api/v1/scoring-rules", sessionAuth(http.HandlerFunc(s.listScoringRulesHandler)))
	s.mux.Handle("POST /api/v1/scoring-rules", sessionAuth(s.leadLimitCheck(http.HandlerFunc(s.createScoringRuleHandler))))
	s.mux.Handle("PUT /api/v1/scoring-rules/{id}", sessionAuth(http.HandlerFunc(s.updateScoringRuleHandler)))
	s.mux.Handle("DELETE /api/v1/scoring-rules/{id}", sessionAuth(http.HandlerFunc(s.deleteScoringRuleHandler)))

	// CRM webhooks.
	s.mux.Handle("GET /api/v1/crm-webhooks", sessionAuth(http.HandlerFunc(s.listCRMWebhooksHandler)))
	s.mux.Handle("POST /api/v1/crm-webhooks", sessionAuth(http.HandlerFunc(s.createCRMWebhookHandler)))
	s.mux.Handle("PUT /api/v1/crm-webhooks/{id}", sessionAuth(http.HandlerFunc(s.updateCRMWebhookHandler)))
	s.mux.Handle("DELETE /api/v1/crm-webhooks/{id}", sessionAuth(http.HandlerFunc(s.deleteCRMWebhookHandler)))
	s.mux.Handle("POST /api/v1/crm-webhooks/{id}/test", sessionAuth(http.HandlerFunc(s.testCRMWebhookHandler)))
	s.mux.Handle("GET /api/v1/crm-webhooks/{id}/deliveries", sessionAuth(http.HandlerFunc(s.webhookDeliveriesHandler)))
	s.mux.Handle("POST /api/v1/crm-webhooks/{id}/deliveries/{deliveryId}/retry", sessionAuth(http.HandlerFunc(s.retryWebhookDeliveryHandler)))
	s.mux.Handle("GET /api/v1/crm-webhooks/dead-letters", sessionAuth(http.HandlerFunc(s.deadLettersHandler)))

	// Publishers (outbound connectors).
	s.mux.Handle("GET /api/v1/publishers", sessionAuth(http.HandlerFunc(s.listPublishersHandler)))
	s.mux.Handle("POST /api/v1/publishers/{name}/post", sessionAuth(http.HandlerFunc(s.publisherPostHandler)))
	s.mux.Handle("GET /api/v1/publishers/{name}/engagement/{externalID}", sessionAuth(http.HandlerFunc(s.publisherEngagementHandler)))
	s.mux.Handle("GET /api/v1/publishers/{name}/validate", sessionAuth(http.HandlerFunc(s.publisherValidateHandler)))

	// Keep legacy connector routes for backward compat with existing SDK/frontend.
	s.mux.Handle("GET /api/v1/connectors", sessionAuth(http.HandlerFunc(s.listPublishersHandler)))
	s.mux.Handle("POST /api/v1/connectors/{name}/post", sessionAuth(http.HandlerFunc(s.publisherPostHandler)))
	s.mux.Handle("GET /api/v1/connectors/{name}/engagement/{externalID}", sessionAuth(http.HandlerFunc(s.publisherEngagementHandler)))
	s.mux.Handle("GET /api/v1/connectors/{name}/validate", sessionAuth(http.HandlerFunc(s.publisherValidateHandler)))

	// Sources (inbound connectors).
	s.mux.Handle("GET /api/v1/sources", sessionAuth(http.HandlerFunc(s.listSourcesHandler)))
	s.mux.Handle("POST /api/v1/sources/{name}/search", sessionAuth(http.HandlerFunc(s.triggerSourceSearchHandler)))
	// Source credentials (OAuth setup flow).
	s.mux.Handle("GET /api/v1/sources/{name}/credentials", sessionAuth(http.HandlerFunc(s.getSourceCredentialsHandler)))
	s.mux.Handle("POST /api/v1/sources/{name}/credentials", sessionAuth(http.HandlerFunc(s.saveSourceCredentialsHandler)))
	s.mux.Handle("DELETE /api/v1/sources/{name}/credentials", sessionAuth(http.HandlerFunc(s.deleteSourceCredentialsHandler)))
	s.mux.Handle("GET /api/v1/sources/{name}/oauth/authorize", sessionAuth(http.HandlerFunc(s.sourceOAuthAuthorizeHandler)))
	s.mux.HandleFunc("GET /api/v1/sources/{name}/oauth/callback", s.sourceOAuthCallbackHandler) // no session auth — browser redirect

	// Source configs.
	s.mux.Handle("GET /api/v1/source-configs", sessionAuth(http.HandlerFunc(s.listSourceConfigsHandler)))
	s.mux.Handle("POST /api/v1/source-configs", sessionAuth(http.HandlerFunc(s.upsertSourceConfigHandler)))

	// Mentions inbox.
	s.mux.Handle("GET /api/v1/mentions", sessionAuth(http.HandlerFunc(s.listMentionsHandler)))
	s.mux.Handle("GET /api/v1/mentions/{id}", sessionAuth(http.HandlerFunc(s.getMentionHandler)))
	s.mux.Handle("PUT /api/v1/mentions/{id}", sessionAuth(http.HandlerFunc(s.updateMentionHandler)))
	s.mux.Handle("POST /api/v1/mentions/{id}/draft", sessionAuth(http.HandlerFunc(s.draftMentionReplyHandler)))
	s.mux.Handle("POST /api/v1/mentions/{id}/reply", sessionAuth(http.HandlerFunc(s.publishMentionReplyHandler)))

	// Campaigns.
	s.mux.Handle("GET /api/v1/campaigns", sessionAuth(http.HandlerFunc(s.listCampaignsHandler)))
	s.mux.Handle("POST /api/v1/campaigns", sessionAuth(http.HandlerFunc(s.createCampaignHandler)))
	s.mux.Handle("GET /api/v1/campaigns/{id}", sessionAuth(http.HandlerFunc(s.getCampaignHandler)))
	s.mux.Handle("PUT /api/v1/campaigns/{id}", sessionAuth(http.HandlerFunc(s.updateCampaignHandler)))
	s.mux.Handle("DELETE /api/v1/campaigns/{id}", sessionAuth(http.HandlerFunc(s.deleteCampaignHandler)))
	s.mux.Handle("POST /api/v1/campaigns/generate", sessionAuth(http.HandlerFunc(s.generateCampaignHandler)))
	s.mux.Handle("POST /api/v1/campaigns/{id}/ab-test", sessionAuth(http.HandlerFunc(s.abTestHandler)))
	s.mux.Handle("GET /api/v1/campaigns/{id}/ab-results", sessionAuth(http.HandlerFunc(queryHandler.ABResultsHandler)))
	s.mux.Handle("GET /api/v1/campaigns/{id}/performance", sessionAuth(http.HandlerFunc(s.campaignPerformanceHandler)))
	s.mux.Handle("POST /api/v1/campaigns/{id}/publish", sessionAuth(http.HandlerFunc(s.publishCampaignHandler)))
	s.mux.Handle("POST /api/v1/campaigns/{id}/refresh-engagement", sessionAuth(http.HandlerFunc(s.refreshCampaignEngagementHandler)))

	// ICP.
	s.mux.Handle("POST /api/v1/icp/analyze", sessionAuth(http.HandlerFunc(s.icpAnalyzeHandler)))
	s.mux.Handle("GET /api/v1/icp/analyses", sessionAuth(http.HandlerFunc(s.listICPAnalysesHandler)))
	s.mux.Handle("GET /api/v1/icp/analyses/{id}", sessionAuth(http.HandlerFunc(s.getICPAnalysisHandler)))
	s.mux.Handle("DELETE /api/v1/icp/analyses/{id}", sessionAuth(http.HandlerFunc(s.deleteICPAnalysisHandler)))
	s.mux.Handle("POST /api/v1/icp/analyses/{id}/generate-campaign", sessionAuth(http.HandlerFunc(s.icpGenerateCampaignHandler)))
	s.mux.Handle("POST /api/v1/icp/analyses/{id}/create-scoring-rules", sessionAuth(http.HandlerFunc(s.icpCreateScoringRulesHandler)))
	s.mux.Handle("GET /api/v1/icp/settings", sessionAuth(http.HandlerFunc(s.getICPSettingsHandler)))
	s.mux.Handle("PUT /api/v1/icp/settings", sessionAuth(http.HandlerFunc(s.putICPSettingsHandler)))

	// Lead score history + attribution.
	s.mux.Handle("GET /api/v1/leads/{id}/score-history", sessionAuth(http.HandlerFunc(s.leadScoreHistoryHandler)))
	s.mux.Handle("GET /api/v1/leads/{id}/attribution", sessionAuth(http.HandlerFunc(s.leadAttributionHandler)))

	// Segments.
	s.mux.Handle("GET /api/v1/segments", sessionAuth(http.HandlerFunc(s.listSegmentsHandler)))
	s.mux.Handle("POST /api/v1/segments", sessionAuth(http.HandlerFunc(s.createSegmentHandler)))
	s.mux.Handle("DELETE /api/v1/segments/{id}", sessionAuth(http.HandlerFunc(s.deleteSegmentHandler)))
	s.mux.Handle("GET /api/v1/segments/{id}/members", sessionAuth(ql(http.HandlerFunc(s.segmentMembersHandler))))

	// Conversion Goals.
	s.mux.Handle("GET /api/v1/conversion-goals", sessionAuth(http.HandlerFunc(s.listConversionGoalsHandler)))
	s.mux.Handle("POST /api/v1/conversion-goals", sessionAuth(http.HandlerFunc(s.createConversionGoalHandler)))
	s.mux.Handle("GET /api/v1/conversion-goals/{id}", sessionAuth(http.HandlerFunc(s.getConversionGoalHandler)))
	s.mux.Handle("PUT /api/v1/conversion-goals/{id}", sessionAuth(http.HandlerFunc(s.updateConversionGoalHandler)))
	s.mux.Handle("DELETE /api/v1/conversion-goals/{id}", sessionAuth(http.HandlerFunc(s.deleteConversionGoalHandler)))
	s.mux.Handle("GET /api/v1/conversion-goals/{id}/results", sessionAuth(ql(http.HandlerFunc(queryHandler.ConversionGoalResultsHandler))))

	// Revenue attribution.
	s.mux.Handle("GET /api/v1/attribution/revenue", sessionAuth(ql(http.HandlerFunc(queryHandler.RevenueAttributionHandler))))

	// Experiments.
	s.mux.Handle("GET /api/v1/experiments", sessionAuth(http.HandlerFunc(s.listExperimentsHandler)))
	s.mux.Handle("POST /api/v1/experiments", sessionAuth(http.HandlerFunc(s.createExperimentHandler)))
	s.mux.Handle("GET /api/v1/experiments/{id}", sessionAuth(http.HandlerFunc(s.getExperimentHandler)))
	s.mux.Handle("PUT /api/v1/experiments/{id}", sessionAuth(http.HandlerFunc(s.updateExperimentHandler)))
	s.mux.Handle("DELETE /api/v1/experiments/{id}", sessionAuth(http.HandlerFunc(s.deleteExperimentHandler)))
	s.mux.Handle("GET /api/v1/experiments/{id}/results", sessionAuth(ql(http.HandlerFunc(queryHandler.ExperimentResultsHandler))))
	s.mux.Handle("GET /api/v1/experiments/{id}/sample-size", sessionAuth(http.HandlerFunc(queryHandler.ExperimentSampleSizeHandler)))
	s.mux.Handle("POST /api/v1/experiments/{id}/stop", sessionAuth(http.HandlerFunc(s.stopExperimentHandler)))
	s.mux.Handle("POST /api/v1/experiments/{id}/declare-winner", sessionAuth(http.HandlerFunc(s.declareWinnerHandler)))

	// Backup / restore.
	// Safe in single-tenant cloud instances (only one customer's data).
	// Only disabled when CloudMode is set WITHOUT a ControlPlaneURL (legacy multi-tenant).
	if !s.config.CloudMode || s.config.ControlPlaneURL != "" {
		s.mux.Handle("GET /api/v1/export", sessionAuth(http.HandlerFunc(s.exportHandler)))
		s.mux.Handle("POST /api/v1/import", sessionAuth(http.HandlerFunc(s.importHandler)))
	}

	// Storage stats.
	s.mux.Handle("GET /api/v1/storage", sessionAuth(http.HandlerFunc(s.storageHandler)))

	// GitHub OAuth (only functional when GITHUB_CLIENT_ID is set).
	s.mux.Handle("GET /api/v1/github/oauth/enabled", sessionAuth(http.HandlerFunc(s.githubOAuthEnabledHandler)))
	s.mux.Handle("GET /api/v1/github/oauth/authorize", sessionAuth(http.HandlerFunc(s.githubOAuthAuthorizeHandler)))
	s.mux.HandleFunc("GET /api/v1/github/oauth/callback", s.githubOAuthCallbackHandler) // No session auth — browser redirect from GitHub

	// Auth (no session required).
	// In single-tenant cloud mode (ControlPlaneURL set), users log in at their instance,
	// so setup/login are re-enabled. Only disabled in legacy multi-tenant CloudMode.
	s.mux.HandleFunc("GET /api/v1/auth/setup-required", s.setupRequiredHandler)
	if !s.config.CloudMode || s.config.ControlPlaneURL != "" {
		s.mux.HandleFunc("POST /api/v1/auth/setup", s.setupHandler)
		s.mux.HandleFunc("POST /api/v1/auth/login", s.loginHandler)
	}
	s.mux.HandleFunc("POST /api/v1/auth/logout", s.logoutHandler)
	s.mux.Handle("GET /api/v1/auth/me", sessionAuth(http.HandlerFunc(s.meHandler)))
	s.mux.Handle("PUT /api/v1/auth/project", sessionAuth(http.HandlerFunc(s.switchProjectHandler)))

	// Multi-project management.
	s.mux.Handle("GET /api/v1/projects", sessionAuth(http.HandlerFunc(s.listProjectsHandler)))
	s.mux.Handle("POST /api/v1/projects", sessionAuth(http.HandlerFunc(s.createProjectHandler)))
	s.mux.Handle("GET /api/v1/projects/{id}/members", sessionAuth(http.HandlerFunc(s.listMembersHandler)))
	s.mux.Handle("POST /api/v1/projects/{id}/members", sessionAuth(http.HandlerFunc(s.addMemberHandler)))
	s.mux.Handle("DELETE /api/v1/projects/{id}/members/{userID}", sessionAuth(http.HandlerFunc(s.removeMemberHandler)))

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

	// Public config (tells the frontend about cloud mode).
	s.mux.HandleFunc("GET /api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"cloud_mode": s.config.CloudMode,
		})
	})

	// Cloud instance routes — seed, token exchange, billing proxy.
	// These only activate when this instance is managed by a control plane.
	if s.config.ControlPlaneURL != "" {
		s.mux.HandleFunc("POST /api/v1/internal/seed", s.handleSeed)
		s.mux.HandleFunc("POST /api/v1/auth/token-exchange", s.handleTokenExchange)
		s.mux.Handle("GET /api/v1/billing/usage", sessionAuth(http.HandlerFunc(s.handleBillingProxy)))
		s.mux.Handle("POST /api/v1/billing/checkout", sessionAuth(http.HandlerFunc(s.handleBillingProxy)))
		s.mux.Handle("POST /api/v1/billing/portal", sessionAuth(http.HandlerFunc(s.handleBillingProxy)))
	}

	// EE route injection — billing, signup, instance routes.
	if s.config.RouteHook != nil {
		s.config.RouteHook(s.mux, s.meta)
	}

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
	s.startLeadPusher()
	s.startEngagementPoller()
	s.startSourceMonitor()
	s.startRetentionCleanup()
	s.startLeadScoreSnapshotter()
	s.startICPAutoRefresh()
	s.startExperimentAutoStop()
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			s.eventLimiter.Cleanup(1 * time.Hour)
		}
	}()
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

func (s *Server) updateProjectDescriptionHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := s.meta.UpdateProjectDescription(r.Context(), project.ID, body.Description); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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

	systemMsg := buildAnalyticsSystemPrompt(project.Description, trendData, topPages, topEvents)

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

func buildAnalyticsSystemPrompt(projectDescription string, trends []storage.TrendPoint, pages []storage.PageStat, events []storage.EventNameStat) string {
	var b strings.Builder
	b.WriteString("You are an analytics assistant embedded in ClickNest, a product analytics dashboard. ")
	b.WriteString("You have access to real analytics data from the user's product. ")
	b.WriteString("Be concise, direct, and actionable. Use plain paragraphs — no markdown headers or bullet lists unless explicitly asked. ")
	b.WriteString("Focus on insights that help the user understand their product's performance and what to improve.\n\n")

	if projectDescription != "" {
		b.WriteString("ABOUT THIS PRODUCT:\n")
		b.WriteString(projectDescription)
		b.WriteString("\n\n")
	}

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

	// Enrich with experiment variant assignments.
	experiments := make(map[string]any)
	if expList, err := s.meta.ListExperiments(r.Context(), project.ID); err == nil {
		for _, exp := range expList {
			if exp.Status != "running" {
				continue
			}
			var variants []string
			json.Unmarshal([]byte(exp.Variants), &variants)
			if len(variants) == 0 {
				continue
			}
			h := fnv.New32a()
			h.Write([]byte(distinctID + ":" + exp.FlagKey))
			idx := int(h.Sum32()) % len(variants)
			experiments[exp.FlagKey] = map[string]any{
				"experiment_id": exp.ID,
				"variant":       variants[idx],
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"flags": result, "experiments": experiments})
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

// --- Ref Code handlers ---

func (s *Server) listRefCodesHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	codes, err := s.meta.ListRefCodes(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ref_codes": codes})
}

func (s *Server) createRefCodeHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Code  string `json:"code"`
		Name  string `json:"name"`
		Notes string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" || body.Name == "" {
		http.Error(w, `{"error":"code and name are required"}`, http.StatusBadRequest)
		return
	}
	id, err := generateID()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	rc := storage.RefCode{
		ID:        id,
		ProjectID: project.ID,
		Code:      body.Code,
		Name:      body.Name,
		Notes:     body.Notes,
	}
	if err := s.meta.CreateRefCode(r.Context(), rc); err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rc)
}

func (s *Server) updateRefCodeHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	var body struct {
		Name  string `json:"name"`
		Notes string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := s.meta.UpdateRefCode(r.Context(), project.ID, id, body.Name, body.Notes); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) deleteRefCodeHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if err := s.meta.DeleteRefCode(r.Context(), project.ID, id); err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Publisher handlers ---

func (s *Server) listPublishersHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	publishers := s.registry.ListPublishers()
	result := make([]map[string]string, len(publishers))
	for i, p := range publishers {
		result[i] = map[string]string{"name": p.Name(), "display_name": p.DisplayName()}
	}
	w.Header().Set("Content-Type", "application/json")
	// Return as "connectors" for frontend backward compat.
	json.NewEncoder(w).Encode(map[string]any{"connectors": result})
}

func (s *Server) publisherPostHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	name := r.PathValue("name")
	p := s.registry.GetPublisher(name)
	if p == nil {
		http.Error(w, `{"error":"publisher not found"}`, http.StatusNotFound)
		return
	}
	var body growth.PostContent
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	result, err := p.Post(r.Context(), body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"post failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) publisherEngagementHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	name := r.PathValue("name")
	externalID := r.PathValue("externalID")
	p := s.registry.GetPublisher(name)
	if p == nil {
		http.Error(w, `{"error":"publisher not found"}`, http.StatusNotFound)
		return
	}
	metrics, err := p.FetchEngagement(r.Context(), externalID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"fetch failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (s *Server) publisherValidateHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	name := r.PathValue("name")
	p := s.registry.GetPublisher(name)
	if p == nil {
		http.Error(w, `{"error":"publisher not found"}`, http.StatusNotFound)
		return
	}
	if err := p.Validate(r.Context()); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"valid": false, "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"valid": true})
}

// --- Campaign handlers ---

func (s *Server) listCampaignsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	campaigns, err := s.meta.ListCampaigns(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	// Build a map from ref code string → campaign ID to join stats from DuckDB.
	refCodes, _ := s.meta.ListRefCodes(r.Context(), project.ID)
	refCodeByID := make(map[string]string, len(refCodes)) // ref_code_id → code string
	for _, rc := range refCodes {
		refCodeByID[rc.ID] = rc.Code
	}

	now := time.Now().UTC()
	start := now.Add(-30 * 24 * time.Hour)
	batchStats, _ := s.events.QueryRefCodeStatsBatch(r.Context(), project.ID, start, now)

	// Build campaign_id → CampaignStats by matching ref code strings.
	campaignStats := make(map[string]storage.CampaignStats)
	for _, c := range campaigns {
		if c.RefCodeID == "" {
			continue
		}
		code, ok := refCodeByID[c.RefCodeID]
		if !ok || code == "" {
			continue
		}
		if s, ok := batchStats[code]; ok {
			campaignStats[c.ID] = s
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"campaigns": campaigns,
		"stats":     campaignStats,
	})
}

func (s *Server) createCampaignHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Name    string `json:"name"`
		Channel string `json:"channel"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.Channel == "" {
		http.Error(w, `{"error":"name and channel are required"}`, http.StatusBadRequest)
		return
	}
	id, err := generateID()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if body.Content == "" {
		body.Content = "{}"
	}
	c := storage.Campaign{
		ID:        id,
		ProjectID: project.ID,
		Name:      body.Name,
		Channel:   body.Channel,
		Status:    "draft",
		Content:   body.Content,
	}
	if err := s.meta.CreateCampaign(r.Context(), c); err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(c)
}

func (s *Server) getCampaignHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	c, err := s.meta.GetCampaign(r.Context(), project.ID, id)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c)
}

func (s *Server) updateCampaignHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	var body struct {
		Name    string  `json:"name"`
		Status  string  `json:"status"`
		Content string  `json:"content"`
		Cost    float64 `json:"cost"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := s.meta.UpdateCampaign(r.Context(), project.ID, id, body.Name, body.Status, body.Content, body.Cost); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) deleteCampaignHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if err := s.meta.DeleteCampaign(r.Context(), project.ID, id); err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) generateCampaignHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if s.config.ResourceLimitFn != nil {
		if code, msg := s.config.ResourceLimitFn(r.Context(), project.ID, "campaigns"); code != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			json.NewEncoder(w).Encode(map[string]any{"error": msg, "upgrade_required": true})
			return
		}
	}
	cfg, err := s.meta.GetLLMConfig(r.Context(), project.ID)
	if err != nil || cfg.Provider == "" {
		http.Error(w, `{"error":"LLM not configured. Go to Settings to add an AI provider."}`, http.StatusBadRequest)
		return
	}

	var body struct {
		Channel string `json:"channel"`
		Topic   string `json:"topic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Channel == "" {
		http.Error(w, `{"error":"channel is required"}`, http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	monthAgo := now.Add(-30 * 24 * time.Hour)
	topPages, _ := s.events.QueryTopPages(r.Context(), project.ID, monthAgo, now, 10)
	topEvents, _ := s.events.QueryTopEventNames(r.Context(), project.ID, monthAgo, now, 10)

	// Auto-create a ref code for tracking.
	refCodeID, _ := generateID()
	refCode := fmt.Sprintf("campaign_%s", refCodeID[:8])
	_ = s.meta.CreateRefCode(r.Context(), storage.RefCode{
		ID:        refCodeID,
		ProjectID: project.ID,
		Code:      refCode,
		Name:      fmt.Sprintf("Campaign: %s", body.Topic),
	})

	proj, _ := s.meta.GetProject(r.Context(), project.ID)
	projectDesc := ""
	if proj != nil {
		projectDesc = proj.Description
	}

	cc := ai.CampaignContext{
		ProjectDescription: projectDesc,
		TopPages:           topPages,
		TopEvents:          topEvents,
		Channel:            body.Channel,
		Topic:              body.Topic,
		RefURL:             fmt.Sprintf("?ref=%s", refCode),
	}

	content, err := ai.GenerateCampaign(r.Context(), cfg, cc)
	if err != nil {
		log.Printf("ERROR campaign generation: %v", err)
		http.Error(w, `{"error":"AI generation failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	contentJSON, _ := json.Marshal(content)

	campaignID, _ := generateID()
	campaign := storage.Campaign{
		ID:        campaignID,
		ProjectID: project.ID,
		Name:      content.Title,
		Channel:   body.Channel,
		RefCodeID: refCodeID,
		Status:    "draft",
		Content:   string(contentJSON),
		AIPrompt:  body.Topic,
	}
	if err := s.meta.CreateCampaign(r.Context(), campaign); err != nil {
		http.Error(w, `{"error":"save failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"campaign":  campaign,
		"content":   content,
		"ref_code":  refCode,
	})
}

func (s *Server) abTestHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	cfg, err := s.meta.GetLLMConfig(r.Context(), project.ID)
	if err != nil || cfg.Provider == "" {
		http.Error(w, `{"error":"LLM not configured"}`, http.StatusBadRequest)
		return
	}

	campaignID := r.PathValue("id")
	campaign, err := s.meta.GetCampaign(r.Context(), project.ID, campaignID)
	if err != nil {
		http.Error(w, `{"error":"campaign not found"}`, http.StatusNotFound)
		return
	}

	var original ai.CampaignContent
	json.Unmarshal([]byte(campaign.Content), &original)

	variations, err := ai.GenerateVariations(r.Context(), cfg, original, campaign.Channel, 2)
	if err != nil {
		log.Printf("ERROR A/B variation generation: %v", err)
		http.Error(w, `{"error":"variation generation failed"}`, http.StatusInternalServerError)
		return
	}

	// Create feature flags for each variation.
	type variationWithFlag struct {
		FlagKey string             `json:"flag_key"`
		FlagID  string             `json:"flag_id"`
		Content ai.CampaignContent `json:"content"`
	}
	var results []variationWithFlag

	totalVariations := len(variations) + 1 // +1 for original
	rollout := 100 / totalVariations

	// Original gets a flag too.
	origFlagID, _ := generateID()
	origKey := fmt.Sprintf("ab_campaign_%s_original", campaignID[:8])
	_ = s.meta.CreateFeatureFlag(r.Context(), storage.FeatureFlag{
		ID:                origFlagID,
		ProjectID:         project.ID,
		Key:               origKey,
		Name:              "A/B: Original",
		Enabled:           true,
		RolloutPercentage: rollout,
	})
	results = append(results, variationWithFlag{FlagKey: origKey, FlagID: origFlagID, Content: original})

	for i, v := range variations {
		flagID, _ := generateID()
		key := fmt.Sprintf("ab_campaign_%s_v%d", campaignID[:8], i+1)
		pct := rollout
		if i == len(variations)-1 {
			pct = 100 - rollout*totalVariations + rollout // ensure they sum to ~100
		}
		_ = s.meta.CreateFeatureFlag(r.Context(), storage.FeatureFlag{
			ID:                flagID,
			ProjectID:         project.ID,
			Key:               key,
			Name:              fmt.Sprintf("A/B: Variation %d", i+1),
			Enabled:           true,
			RolloutPercentage: pct,
		})
		results = append(results, variationWithFlag{FlagKey: key, FlagID: flagID, Content: v})
	}

	// Save variations back to campaign content.
	updatedContent, _ := json.Marshal(map[string]any{
		"original":   original,
		"variations": results,
	})
	_ = s.meta.UpdateCampaign(r.Context(), project.ID, campaignID, campaign.Name, campaign.Status, string(updatedContent), campaign.Cost)

	// Create an Experiment record linking to the campaign's base flag key.
	baseFlagKey := fmt.Sprintf("ab_campaign_%s", campaignID[:8])
	variantNames := []string{"original"}
	for i := range variations {
		variantNames = append(variantNames, fmt.Sprintf("v%d", i+1))
	}
	variantsJSON, _ := json.Marshal(variantNames)
	expID, _ := generateID()
	_ = s.meta.CreateExperiment(r.Context(), storage.Experiment{
		ID:        expID,
		ProjectID: project.ID,
		Name:      "A/B: " + campaign.Name,
		FlagKey:   baseFlagKey,
		Variants:  string(variantsJSON),
		Status:    "running",
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"variations": results, "experiment_id": expID})
}

func (s *Server) campaignPerformanceHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	campaignID := r.PathValue("id")
	campaign, err := s.meta.GetCampaign(r.Context(), project.ID, campaignID)
	if err != nil {
		http.Error(w, `{"error":"campaign not found"}`, http.StatusNotFound)
		return
	}

	// Look up ref code to get the code string.
	var refCode string
	if campaign.RefCodeID != "" {
		rc, err := s.meta.GetRefCode(r.Context(), project.ID, campaign.RefCodeID)
		if err == nil {
			refCode = rc.Code
		}
	}

	result := map[string]any{
		"campaign": campaign,
		"ref_code": refCode,
	}

	if refCode != "" {
		now := time.Now().UTC()
		start := now.Add(-30 * 24 * time.Hour)
		if v := r.URL.Query().Get("start"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				start = t
			}
		}
		end := now
		if v := r.URL.Query().Get("end"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				end = t
			}
		}

		stats, err := s.events.QueryCampaignStats(r.Context(), project.ID, refCode, start, end)
		if err == nil {
			result["stats"] = stats
		}

		timeSeries, err := s.events.QueryCampaignTimeSeries(r.Context(), project.ID, refCode, start, end)
		if err == nil {
			result["time_series"] = timeSeries
		}

		// Channel breakdown: how traffic arrived at this campaign.
		if channels, err := s.events.QueryCampaignChannelBreakdown(r.Context(), project.ID, refCode, start, end); err == nil {
			result["channels"] = channels
		}

		// Conversion event counting: if caller specifies a conversion event, count matching sessions.
		if convEvent := r.URL.Query().Get("conversion_event"); convEvent != "" {
			count, err := s.events.QueryCampaignConversions(r.Context(), project.ID, refCode, convEvent, start, end)
			if err == nil && stats != nil && stats.Sessions > 0 {
				result["conversion_count"] = count
				result["conversion_rate"] = float64(count) / float64(stats.Sessions) * 100
				result["conversion_event"] = convEvent
			} else if err == nil {
				result["conversion_count"] = count
				result["conversion_rate"] = 0.0
				result["conversion_event"] = convEvent
			}
		}
	}

	// Revenue query: if a conversion_goal_id is specified, compute revenue and ROI.
	if goalID := r.URL.Query().Get("conversion_goal_id"); goalID != "" && refCode != "" {
		goal, err := s.meta.GetConversionGoal(r.Context(), project.ID, goalID)
		if err == nil {
			now := time.Now().UTC()
			start := now.Add(-30 * 24 * time.Hour)
			end := now
			criteria := storage.GoalCriteria{
				EventType:     goal.EventType,
				EventName:     goal.EventName,
				URLPattern:    goal.URLPattern,
				ValueProperty: goal.ValueProperty,
			}
			overview, err := s.events.QueryRevenueOverview(r.Context(), project.ID, criteria, start, end)
			if err == nil {
				result["revenue"] = overview.TotalRevenue
				if campaign.Cost > 0 {
					result["roi"] = (overview.TotalRevenue - campaign.Cost) / campaign.Cost * 100
				}
			}
		}
	}

	// Include engagement data from campaign posts.
	posts, err := s.meta.ListCampaignPosts(r.Context(), project.ID)
	if err == nil {
		var campaignPosts []storage.CampaignPost
		for _, p := range posts {
			if p.CampaignID == campaignID {
				campaignPosts = append(campaignPosts, p)
			}
		}
		result["posts"] = campaignPosts
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// --- ICP handler ---

func (s *Server) icpAnalyzeHandler(w http.ResponseWriter, r *http.Request) {
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
		ConversionPaths []string `json:"conversion_paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.ConversionPaths) == 0 {
		http.Error(w, `{"error":"conversion_paths is required"}`, http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	monthAgo := now.Add(-30 * 24 * time.Hour)

	profiles, err := s.events.QueryICPProfiles(r.Context(), project.ID, body.ConversionPaths, monthAgo, now, 50)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	// Convert to AI-compatible format.
	aiProfiles := make([]ai.ICPProfile, len(profiles))
	for i, p := range profiles {
		aiProfiles[i] = ai.ICPProfile{
			DistinctID:   p.DistinctID,
			SessionCount: p.SessionCount,
			EventCount:   p.EventCount,
			TopPages:     p.TopPages,
			EntrySource:  p.EntrySource,
		}
	}

	proj, _ := s.meta.GetProject(r.Context(), project.ID)
	projectDesc := ""
	if proj != nil {
		projectDesc = proj.Description
	}

	analysis, err := ai.AnalyzeICP(r.Context(), cfg, aiProfiles, projectDesc)
	if err != nil {
		log.Printf("ERROR ICP analysis: %v", err)
		http.Error(w, `{"error":"AI analysis failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// Auto-save the analysis.
	analysisID, _ := generateID()
	pagesJSON, _ := json.Marshal(body.ConversionPaths)
	traitsJSON, _ := json.Marshal(analysis.CommonTraits)
	channelsJSON, _ := json.Marshal(analysis.BestChannels)
	recsJSON, _ := json.Marshal(analysis.Recommendations)
	_ = s.meta.CreateICPAnalysis(r.Context(), storage.ICPAnalysis{
		ID:              analysisID,
		ProjectID:       project.ID,
		ConversionPages: string(pagesJSON),
		Summary:         analysis.Summary,
		Traits:          string(traitsJSON),
		Channels:        string(channelsJSON),
		Recommendations: string(recsJSON),
		ProfileCount:    len(profiles),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"analysis":    analysis,
		"profiles":    profiles,
		"analysis_id": analysisID,
	})
}

func (s *Server) listICPAnalysesHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	analyses, err := s.meta.ListICPAnalyses(r.Context(), project.ID, 20)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"analyses": analyses})
}

func (s *Server) getICPAnalysisHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	a, err := s.meta.GetICPAnalysis(r.Context(), project.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

func (s *Server) deleteICPAnalysisHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if err := s.meta.DeleteICPAnalysis(r.Context(), project.ID, r.PathValue("id")); err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Engagement poller ---

func (s *Server) startEngagementPoller() {
	ticker := time.NewTicker(30 * time.Minute)
	go func() {
		for range ticker.C {
			s.pollEngagement(context.Background())
		}
	}()
}

func (s *Server) pollEngagement(ctx context.Context) {
	projects, err := s.meta.ListProjects(ctx)
	if err != nil {
		log.Printf("WARN engagement poller: failed to list projects: %v", err)
		return
	}
	for _, proj := range projects {
		posts, err := s.meta.ListCampaignPosts(ctx, proj.ID)
		if err != nil {
			continue
		}
		for _, post := range posts {
			p := s.registry.GetPublisher(post.ConnectorName)
			if p == nil {
				continue
			}
			metrics, err := p.FetchEngagement(ctx, post.ExternalID)
			if err != nil {
				log.Printf("WARN engagement poller: fetch failed for post %s: %v", post.ID, err)
				continue
			}
			engJSON, _ := json.Marshal(metrics)
			_ = s.meta.UpdateCampaignPostEngagement(ctx, post.ID, string(engJSON), time.Now().UTC())
		}
	}
}

// --- Scoring Rule handlers ---

func (s *Server) listScoringRulesHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	rules, err := s.meta.ListScoringRules(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"rules": rules})
}

func (s *Server) createScoringRuleHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Name     string `json:"name"`
		RuleType string `json:"rule_type"`
		Config   string `json:"config"`
		Points   int    `json:"points"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.RuleType == "" {
		http.Error(w, `{"error":"name and rule_type are required"}`, http.StatusBadRequest)
		return
	}
	id, err := generateID()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if body.Config == "" {
		body.Config = "{}"
	}
	rule := storage.ScoringRule{
		ID:        id,
		ProjectID: project.ID,
		Name:      body.Name,
		RuleType:  body.RuleType,
		Config:    body.Config,
		Points:    body.Points,
		Enabled:   true,
	}
	if err := s.meta.CreateScoringRule(r.Context(), rule); err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rule)
}

func (s *Server) updateScoringRuleHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	var body struct {
		Name     string `json:"name"`
		RuleType string `json:"rule_type"`
		Config   string `json:"config"`
		Points   int    `json:"points"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := s.meta.UpdateScoringRule(r.Context(), project.ID, id, body.Name, body.RuleType, body.Config, body.Points, body.Enabled); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) deleteScoringRuleHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if err := s.meta.DeleteScoringRule(r.Context(), project.ID, id); err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- CRM Webhook handlers ---

func (s *Server) listCRMWebhooksHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	webhooks, err := s.meta.ListCRMWebhooks(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"webhooks": webhooks})
}

func (s *Server) createCRMWebhookHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Name            string `json:"name"`
		WebhookURL      string `json:"webhook_url"`
		MinScore        int    `json:"min_score"`
		PayloadTemplate string `json:"payload_template"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.WebhookURL == "" {
		http.Error(w, `{"error":"name and webhook_url are required"}`, http.StatusBadRequest)
		return
	}
	id, err := generateID()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	secret, _ := generateID()
	wh := storage.CRMWebhook{
		ID:              id,
		ProjectID:       project.ID,
		Name:            body.Name,
		WebhookURL:      body.WebhookURL,
		MinScore:        body.MinScore,
		Enabled:         true,
		Secret:          secret,
		PayloadTemplate: body.PayloadTemplate,
	}
	if err := s.meta.CreateCRMWebhook(r.Context(), wh); err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(wh)
}

func (s *Server) updateCRMWebhookHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	var body struct {
		Name            string `json:"name"`
		WebhookURL      string `json:"webhook_url"`
		MinScore        int    `json:"min_score"`
		Enabled         bool   `json:"enabled"`
		Secret          string `json:"secret"`
		PayloadTemplate string `json:"payload_template"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := s.meta.UpdateCRMWebhook(r.Context(), project.ID, id, body.Name, body.WebhookURL, body.MinScore, body.Enabled, body.Secret, body.PayloadTemplate); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) deleteCRMWebhookHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if err := s.meta.DeleteCRMWebhook(r.Context(), project.ID, id); err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testCRMWebhookHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	_ = r.PathValue("id")

	samplePayload, _ := json.Marshal(map[string]any{
		"test":       true,
		"project_id": project.ID,
		"leads": []map[string]any{
			{
				"distinct_id":   "test-user@example.com",
				"score":         85,
				"event_count":   42,
				"session_count": 7,
			},
		},
	})

	// Get the webhook to find the URL.
	webhooks, err := s.meta.ListCRMWebhooks(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	whID := r.PathValue("id")
	var targetURL string
	for _, wh := range webhooks {
		if wh.ID == whID {
			targetURL = wh.WebhookURL
			break
		}
	}
	if targetURL == "" {
		http.Error(w, `{"error":"webhook not found"}`, http.StatusNotFound)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(samplePayload))
	if err != nil {
		http.Error(w, `{"error":"failed to build request"}`, http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"webhook delivery failed: %s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "http_status": resp.StatusCode})
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

// --- Lead pusher ---

func (s *Server) startLeadPusher() {
	ticker := time.NewTicker(15 * time.Minute)
	go func() {
		for range ticker.C {
			s.pushLeads(context.Background())
		}
	}()
}

func (s *Server) pushLeads(ctx context.Context) {
	webhooks, err := s.meta.ListAllEnabledCRMWebhooks(ctx)
	if err != nil {
		log.Printf("WARN lead pusher: failed to list webhooks: %v", err)
		return
	}
	for _, wh := range webhooks {
		rules, err := s.meta.ListScoringRules(ctx, wh.ProjectID)
		if err != nil {
			continue
		}
		now := time.Now().UTC()
		start := now.Add(-30 * 24 * time.Hour)
		leads, _, err := s.events.QueryLeadScores(ctx, wh.ProjectID, rules, start, now, 100, 0)
		if err != nil {
			continue
		}
		var qualified []storage.ScoredLead
		for _, l := range leads {
			if l.Score >= wh.MinScore {
				qualified = append(qualified, l)
			}
		}
		if len(qualified) == 0 {
			continue
		}
		payload := buildWebhookPayload(wh, qualified)
		s.deliverWebhook(ctx, wh, payload, len(qualified))
		_ = s.meta.UpdateCRMWebhookPushed(ctx, wh.ID, now)
	}
}

func (s *Server) deliverWebhook(ctx context.Context, wh storage.CRMWebhook, payload []byte, leadCount int) {
	backoffs := []time.Duration{0, 60 * time.Second, 5 * time.Minute}
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			time.Sleep(backoffs[attempt-1])
		}
		req, err := http.NewRequestWithContext(ctx, "POST", wh.WebhookURL, bytes.NewReader(payload))
		if err != nil {
			s.logDelivery(ctx, wh, leadCount, 0, "", err.Error(), false, attempt)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if sig := signPayload(wh.Secret, payload); sig != "" {
			req.Header.Set("X-ClickNest-Signature", sig)
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			s.logDelivery(ctx, wh, leadCount, 0, "", err.Error(), false, attempt)
			continue
		}
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		success := resp.StatusCode >= 200 && resp.StatusCode < 300
		s.logDelivery(ctx, wh, leadCount, resp.StatusCode, string(bodyBytes), "", success, attempt)
		if success {
			log.Printf("INFO lead pusher: pushed %d leads to webhook %s (attempt %d)", leadCount, wh.Name, attempt)
			return
		}
	}
	log.Printf("WARN lead pusher: webhook %s failed after 3 attempts", wh.Name)
}

func (s *Server) logDelivery(ctx context.Context, wh storage.CRMWebhook, leadCount, statusCode int, respBody, errMsg string, success bool, attempt int) {
	id, _ := generateID()
	_ = s.meta.CreateWebhookDelivery(ctx, storage.WebhookDelivery{
		ID:           id,
		WebhookID:    wh.ID,
		ProjectID:    wh.ProjectID,
		LeadCount:    leadCount,
		StatusCode:   statusCode,
		ResponseBody: respBody,
		Error:        errMsg,
		Success:      success,
		Attempt:      attempt,
	})
}

func signPayload(secret string, payload []byte) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) webhookDeliveriesHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	deliveries, err := s.meta.ListWebhookDeliveries(r.Context(), project.ID, r.PathValue("id"), 50)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"deliveries": deliveries})
}

// scoreRelevance computes a keyword overlap ratio [0,1] for a piece of content.
// A score of 0.5 is returned when no keywords are configured.
func scoreRelevance(content, title string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0.5
	}
	text := strings.ToLower(content + " " + title)
	matched := 0
	for _, kw := range keywords {
		if strings.Contains(text, strings.ToLower(kw)) {
			matched++
		}
	}
	return float64(matched) / float64(len(keywords))
}

// --- Campaign publish / engagement ---

func (s *Server) publishCampaignHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	campaignID := r.PathValue("id")
	campaign, err := s.meta.GetCampaign(r.Context(), project.ID, campaignID)
	if err != nil {
		http.Error(w, `{"error":"campaign not found"}`, http.StatusNotFound)
		return
	}

	var body struct {
		PublisherName   string `json:"publisher_name"`
		ContentOverride string `json:"content_override"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if body.PublisherName == "" {
		http.Error(w, `{"error":"publisher_name required"}`, http.StatusBadRequest)
		return
	}
	pub := s.registry.GetPublisher(body.PublisherName)
	if pub == nil {
		http.Error(w, `{"error":"publisher not found"}`, http.StatusNotFound)
		return
	}

	var cc struct {
		Title string   `json:"title"`
		Body  string   `json:"body"`
		URL   string   `json:"url"`
		Tags  []string `json:"tags"`
	}
	_ = json.Unmarshal([]byte(campaign.Content), &cc)

	postBody := cc.Body
	if body.ContentOverride != "" {
		postBody = body.ContentOverride
	}

	if campaign.RefCodeID != "" {
		if rc, err := s.meta.GetRefCode(r.Context(), project.ID, campaign.RefCodeID); err == nil && rc.Code != "" {
			if cc.URL != "" && !strings.Contains(postBody, "?ref=") {
				postBody += "\n\n" + cc.URL + "?ref=" + rc.Code
			}
		}
	}

	result, err := pub.Post(r.Context(), growth.PostContent{
		Title:       cc.Title,
		Body:        postBody,
		Channel:     campaign.Channel,
		Tags:        cc.Tags,
		ExtraFields: sourceCredentialFields(s.meta, r.Context(), project.ID, body.PublisherName),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"publish failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	postID, _ := generateID()
	_ = s.meta.CreateCampaignPost(r.Context(), storage.CampaignPost{
		ID:             postID,
		CampaignID:     campaignID,
		ProjectID:      project.ID,
		ConnectorName:  body.PublisherName,
		ExternalID:     result.ExternalID,
		ExternalURL:    result.ExternalURL,
		PostedAt:       time.Now().UTC(),
		LastEngagement: "{}",
	})
	_ = s.meta.UpdateCampaign(r.Context(), project.ID, campaignID, campaign.Name, "published", campaign.Content, campaign.Cost)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) refreshCampaignEngagementHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	campaignID := r.PathValue("id")
	posts, err := s.meta.ListCampaignPosts(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	refreshed := 0
	for _, p := range posts {
		if p.CampaignID != campaignID || p.ExternalID == "" {
			continue
		}
		pub := s.registry.GetPublisher(p.ConnectorName)
		if pub == nil {
			continue
		}
		metrics, err := pub.FetchEngagement(r.Context(), p.ExternalID)
		if err != nil {
			log.Printf("WARN refresh engagement post %s: %v", p.ID, err)
			continue
		}
		engJSON, _ := json.Marshal(metrics)
		_ = s.meta.UpdateCampaignPostEngagement(r.Context(), p.ID, string(engJSON), time.Now().UTC())
		refreshed++
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"refreshed": refreshed})
}

// --- Webhook delivery retry ---

func (s *Server) retryWebhookDeliveryHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	webhookID := r.PathValue("id")
	webhooks, err := s.meta.ListCRMWebhooks(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	var wh *storage.CRMWebhook
	for i, w := range webhooks {
		if w.ID == webhookID {
			wh = &webhooks[i]
			break
		}
	}
	if wh == nil {
		http.Error(w, `{"error":"webhook not found"}`, http.StatusNotFound)
		return
	}

	// Fire the webhook immediately with an empty lead list as a retry probe.
	payload, _ := json.Marshal(map[string]any{
		"webhook":   wh.Name,
		"leads":     []any{},
		"retry":     true,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, wh.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		http.Error(w, `{"error":"failed to create request"}`, http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if sig := signPayload(wh.Secret, payload); sig != "" {
		req.Header.Set("X-ClickNest-Signature", sig)
	}

	resp, err := http.DefaultClient.Do(req)
	var statusCode int
	var respBody string
	success := false
	if err == nil {
		statusCode = resp.StatusCode
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		respBody = string(b)
		success = statusCode >= 200 && statusCode < 300
	}

	dlvID, _ := generateID()
	_ = s.meta.CreateWebhookDelivery(r.Context(), storage.WebhookDelivery{
		ID:           dlvID,
		WebhookID:    wh.ID,
		ProjectID:    project.ID,
		LeadCount:    0,
		StatusCode:   statusCode,
		ResponseBody: respBody,
		Success:      success,
		Attempt:      1,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": success, "status_code": statusCode})
}

// --- ICP → action handlers ---

func (s *Server) icpGenerateCampaignHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if s.config.ResourceLimitFn != nil {
		if code, msg := s.config.ResourceLimitFn(r.Context(), project.ID, "campaigns"); code != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			json.NewEncoder(w).Encode(map[string]any{"error": msg, "upgrade_required": true})
			return
		}
	}

	cfg, err := s.meta.GetLLMConfig(r.Context(), project.ID)
	if err != nil || cfg.Provider == "" {
		http.Error(w, `{"error":"LLM not configured"}`, http.StatusBadRequest)
		return
	}

	analysis, err := s.meta.GetICPAnalysis(r.Context(), project.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"error":"analysis not found"}`, http.StatusNotFound)
		return
	}

	var body struct {
		Channel string `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Channel == "" {
		body.Channel = "reddit"
	}

	var traits []string
	_ = json.Unmarshal([]byte(analysis.Traits), &traits)

	now := time.Now().UTC()
	monthAgo := now.Add(-30 * 24 * time.Hour)
	topPages, _ := s.events.QueryTopPages(r.Context(), project.ID, monthAgo, now, 10)
	topEvents, _ := s.events.QueryTopEventNames(r.Context(), project.ID, monthAgo, now, 10)

	proj, _ := s.meta.GetProject(r.Context(), project.ID)
	projectDesc := ""
	if proj != nil {
		projectDesc = proj.Description
	}

	refCodeID, _ := generateID()
	refCode := fmt.Sprintf("icp_%s", refCodeID[:8])
	_ = s.meta.CreateRefCode(r.Context(), storage.RefCode{
		ID:        refCodeID,
		ProjectID: project.ID,
		Code:      refCode,
		Name:      fmt.Sprintf("ICP Campaign (%s)", body.Channel),
	})

	cc := ai.CampaignContext{
		ProjectDescription: projectDesc,
		TopPages:           topPages,
		TopEvents:          topEvents,
		Channel:            body.Channel,
		Topic:              "our ideal customers: " + strings.Join(traits, ", "),
		RefURL:             "?ref=" + refCode,
		ICPTraits:          strings.Join(traits, "; "),
	}

	content, err := ai.GenerateCampaign(r.Context(), cfg, cc)
	if err != nil {
		http.Error(w, `{"error":"AI generation failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	contentJSON, _ := json.Marshal(content)
	campaignID, _ := generateID()
	campaign := storage.Campaign{
		ID:        campaignID,
		ProjectID: project.ID,
		Name:      content.Title,
		Channel:   body.Channel,
		RefCodeID: refCodeID,
		Status:    "draft",
		Content:   string(contentJSON),
		AIPrompt:  "ICP-derived: " + analysis.Summary,
	}
	if err := s.meta.CreateCampaign(r.Context(), campaign); err != nil {
		http.Error(w, `{"error":"save failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"campaign": campaign,
		"content":  content,
		"ref_code": refCode,
	})
}

func (s *Server) icpCreateScoringRulesHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	analysis, err := s.meta.GetICPAnalysis(r.Context(), project.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"error":"analysis not found"}`, http.StatusNotFound)
		return
	}

	var convPages []string
	_ = json.Unmarshal([]byte(analysis.ConversionPages), &convPages)

	created := 0
	for _, page := range convPages {
		ruleID, _ := generateID()
		config, _ := json.Marshal(map[string]string{"url_path": page})
		err := s.meta.CreateScoringRule(r.Context(), storage.ScoringRule{
			ID:        ruleID,
			ProjectID: project.ID,
			Name:      "ICP: Visited " + page,
			RuleType:  "page_visit",
			Config:    string(config),
			Points:    25,
			Enabled:   true,
		})
		if err == nil {
			created++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"created": created})
}

// --- Retention cleanup ---

func (s *Server) startRetentionCleanup() {
	go func() {
		time.Sleep(5 * time.Minute)
		s.runRetentionCleanup(context.Background())
		ticker := time.NewTicker(24 * time.Hour)
		for range ticker.C {
			s.runRetentionCleanup(context.Background())
		}
	}()
}

func (s *Server) runRetentionCleanup(ctx context.Context) {
	projects, err := s.meta.ListProjects(ctx)
	if err != nil {
		log.Printf("WARN retention cleanup: failed to list projects: %v", err)
		return
	}
	for _, proj := range projects {
		var cutoff time.Time
		if s.config.RetentionDaysFn != nil {
			days := s.config.RetentionDaysFn(ctx, proj.ID)
			if days < 0 {
				continue // unlimited retention for this project
			}
			cutoff = time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
		} else {
			cutoff = time.Now().UTC().Add(-365 * 24 * time.Hour)
		}
		deleted, err := s.events.DeleteOldEvents(ctx, proj.ID, cutoff)
		if err != nil {
			log.Printf("WARN retention cleanup: failed for project %s: %v", proj.ID, err)
			continue
		}
		if deleted > 0 {
			log.Printf("INFO retention cleanup: deleted %d events from project %s", deleted, proj.ID)
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

	// In cloud mode, don't expose shared infrastructure storage details.
	if s.config.CloudMode {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(storageInfo{})
		return
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

// --- Conversion Goal handlers ---

func (s *Server) listConversionGoalsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	goals, err := s.meta.ListConversionGoals(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"goals": goals})
}

func (s *Server) createConversionGoalHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Name          string `json:"name"`
		EventType     string `json:"event_type"`
		EventName     string `json:"event_name"`
		URLPattern    string `json:"url_pattern"`
		ValueProperty string `json:"value_property"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	id, err := generateID()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if body.EventType == "" {
		body.EventType = "custom"
	}
	if body.ValueProperty == "" {
		body.ValueProperty = "$value"
	}
	goal := storage.ConversionGoal{
		ID:            id,
		ProjectID:     project.ID,
		Name:          body.Name,
		EventType:     body.EventType,
		EventName:     body.EventName,
		URLPattern:    body.URLPattern,
		ValueProperty: body.ValueProperty,
	}
	if err := s.meta.CreateConversionGoal(r.Context(), goal); err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(goal)
}

func (s *Server) getConversionGoalHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	goal, err := s.meta.GetConversionGoal(r.Context(), project.ID, id)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(goal)
}

func (s *Server) updateConversionGoalHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	var body struct {
		Name          string `json:"name"`
		EventType     string `json:"event_type"`
		EventName     string `json:"event_name"`
		URLPattern    string `json:"url_pattern"`
		ValueProperty string `json:"value_property"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := s.meta.UpdateConversionGoal(r.Context(), project.ID, id, storage.ConversionGoal{
		Name:          body.Name,
		EventType:     body.EventType,
		EventName:     body.EventName,
		URLPattern:    body.URLPattern,
		ValueProperty: body.ValueProperty,
	}); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) deleteConversionGoalHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if err := s.meta.DeleteConversionGoal(r.Context(), project.ID, id); err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Experiment handlers ---

func (s *Server) listExperimentsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	experiments, err := s.meta.ListExperiments(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"experiments": experiments})
}

func (s *Server) createExperimentHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Name             string   `json:"name"`
		FlagKey          string   `json:"flag_key"`
		Variants         []string `json:"variants"`
		ConversionGoalID string   `json:"conversion_goal_id"`
		AutoStop         bool     `json:"auto_stop"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.FlagKey == "" {
		http.Error(w, `{"error":"name and flag_key are required"}`, http.StatusBadRequest)
		return
	}
	if len(body.Variants) < 2 {
		http.Error(w, `{"error":"at least 2 variants required"}`, http.StatusBadRequest)
		return
	}
	id, err := generateID()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	variantsJSON, _ := json.Marshal(body.Variants)
	exp := storage.Experiment{
		ID:               id,
		ProjectID:        project.ID,
		Name:             body.Name,
		FlagKey:          body.FlagKey,
		Variants:         string(variantsJSON),
		ConversionGoalID: body.ConversionGoalID,
		Status:           "running",
		AutoStop:         body.AutoStop,
	}
	if err := s.meta.CreateExperiment(r.Context(), exp); err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(exp)
}

func (s *Server) getExperimentHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	exp, err := s.meta.GetExperiment(r.Context(), project.ID, id)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(exp)
}

func (s *Server) updateExperimentHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	var body struct {
		Name             string `json:"name"`
		Status           string `json:"status"`
		AutoStop         bool   `json:"auto_stop"`
		ConversionGoalID string `json:"conversion_goal_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := s.meta.UpdateExperiment(r.Context(), project.ID, id, body.Name, body.Status, body.AutoStop, body.ConversionGoalID); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) deleteExperimentHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if err := s.meta.DeleteExperiment(r.Context(), project.ID, id); err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) stopExperimentHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if err := s.meta.EndExperiment(r.Context(), project.ID, id, ""); err != nil {
		http.Error(w, `{"error":"stop failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func (s *Server) declareWinnerHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	var body struct {
		Variant string `json:"variant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Variant == "" {
		http.Error(w, `{"error":"variant is required"}`, http.StatusBadRequest)
		return
	}

	exp, err := s.meta.GetExperiment(r.Context(), project.ID, id)
	if err != nil {
		http.Error(w, `{"error":"experiment not found"}`, http.StatusNotFound)
		return
	}

	// End the experiment with the winner.
	if err := s.meta.EndExperiment(r.Context(), project.ID, id, body.Variant); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}

	// Roll out the winning variant by setting the experiment's flag to 100%.
	flags, _ := s.meta.ListFeatureFlags(r.Context(), project.ID)
	for _, f := range flags {
		if f.Key == exp.FlagKey {
			_ = s.meta.UpdateFeatureFlag(r.Context(), project.ID, f.ID, true, 100)
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "winner_declared", "winner": body.Variant})
}

// --- Experiment auto-stop background job ---

func (s *Server) startExperimentAutoStop() {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for range ticker.C {
			s.checkExperimentAutoStop(context.Background())
		}
	}()
}

func (s *Server) checkExperimentAutoStop(ctx context.Context) {
	// List all projects and check their experiments.
	projects, err := s.meta.ListProjects(ctx)
	if err != nil {
		return
	}
	for _, p := range projects {
		experiments, err := s.meta.ListExperiments(ctx, p.ID)
		if err != nil {
			continue
		}
		for _, exp := range experiments {
			if exp.Status != "running" || !exp.AutoStop {
				continue
			}

			var variants []string
			json.Unmarshal([]byte(exp.Variants), &variants)
			if len(variants) < 2 {
				continue
			}

			start := exp.StartedAt
			end := time.Now().UTC()

			results, err := s.events.QueryExperimentResults(ctx, p.ID, exp.FlagKey, variants, nil, start, end)
			if err != nil || len(results) < 2 {
				continue
			}

			// Check minimum sample: at least 100 exposures per variant.
			minExposures := results[0].Exposures
			for _, v := range results[1:] {
				if v.Exposures < minExposures {
					minExposures = v.Exposures
				}
			}
			if minExposures < 100 {
				continue
			}

			// Check statistical significance.
			significant := false
			if len(results) == 2 {
				pVal := zTestPValue(results[0].Conversions, results[0].Exposures, results[1].Conversions, results[1].Exposures)
				significant = pVal < 0.05
			}
			if !significant {
				continue
			}

			// Find the winner (highest conversion rate).
			bestVariant := results[0].Variant
			bestRate := results[0].ConversionRate
			for _, v := range results[1:] {
				if v.ConversionRate > bestRate {
					bestRate = v.ConversionRate
					bestVariant = v.Variant
				}
			}

			_ = s.meta.EndExperiment(ctx, p.ID, exp.ID, bestVariant)
			log.Printf("Experiment auto-stopped: %s (winner: %s, rate: %.2f%%)", exp.Name, bestVariant, bestRate)
		}
	}
}

// zTestPValue returns the two-tailed p-value for a Z-test comparing two proportions.
func zTestPValue(cA, nA, cB, nB int64) float64 {
	if nA <= 0 || nB <= 0 {
		return 1
	}
	p1 := float64(cA) / float64(nA)
	p2 := float64(cB) / float64(nB)
	pPooled := float64(cA+cB) / float64(nA+nB)
	se := math.Sqrt(pPooled * (1 - pPooled) * (1.0/float64(nA) + 1.0/float64(nB)))
	if se == 0 {
		return 1
	}
	z := (p1 - p2) / se
	return math.Erfc(math.Abs(z) / math.Sqrt2)
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- Source handlers ---

func (s *Server) listSourcesHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	sources := s.registry.ListSources()
	result := make([]map[string]string, len(sources))
	for i, src := range sources {
		result[i] = map[string]string{"name": src.Name(), "display_name": src.DisplayName()}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"sources": result})
}

func (s *Server) triggerSourceSearchHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	name := r.PathValue("name")
	src := s.registry.GetSource(name)
	if src == nil {
		http.Error(w, `{"error":"source not found"}`, http.StatusNotFound)
		return
	}

	cfg, err := s.meta.GetSourceConfig(r.Context(), project.ID, name)
	if err != nil {
		http.Error(w, `{"error":"source not configured"}`, http.StatusBadRequest)
		return
	}

	var keywords []string
	_ = json.Unmarshal([]byte(cfg.Keywords), &keywords)

	mentions, err := src.Search(r.Context(), growth.SearchQuery{
		Keywords:    keywords,
		MaxResults:  25,
		ExtraFields: sourceCredentialFields(s.meta, r.Context(), project.ID, name),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"search failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	count := 0
	for _, m := range mentions {
		id, _ := generateID()
		_ = s.meta.UpsertMention(r.Context(), storage.MentionRecord{
			ID:             id,
			ProjectID:      project.ID,
			SourceName:     name,
			ExternalID:     m.ExternalID,
			ExternalURL:    m.ExternalURL,
			Author:         m.Author,
			Title:          m.Title,
			Content:        m.Content,
			RelevanceScore: scoreRelevance(m.Content, m.Title, keywords),
			ParentID:       m.ParentID,
			Metadata:       "{}",
			PostedAt:       &m.PostedAt,
		})
		count++
	}

	_ = s.meta.UpdateSourceConfigLastRun(r.Context(), project.ID, name, time.Now().UTC())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"found": count})
}

// --- Source config handlers ---

func (s *Server) listSourceConfigsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	configs, err := s.meta.ListSourceConfigs(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"list failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"configs": configs})
}

func (s *Server) upsertSourceConfigHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		SourceName      string   `json:"source_name"`
		Keywords        []string `json:"keywords"`
		ScheduleMinutes int      `json:"schedule_minutes"`
		Enabled         bool     `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if body.SourceName == "" {
		http.Error(w, `{"error":"source_name required"}`, http.StatusBadRequest)
		return
	}
	if body.ScheduleMinutes <= 0 {
		body.ScheduleMinutes = 60
	}

	// Enforce connector limits before enabling a source.
	if body.Enabled && s.config.ResourceLimitFn != nil {
		if code, msg := s.config.ResourceLimitFn(r.Context(), project.ID, "connectors"); code != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			json.NewEncoder(w).Encode(map[string]any{"error": msg, "upgrade_required": true})
			return
		}
	}

	kwJSON, _ := json.Marshal(body.Keywords)
	id, _ := generateID()
	err := s.meta.UpsertSourceConfig(r.Context(), storage.SourceConfig{
		ID:              id,
		ProjectID:       project.ID,
		SourceName:      body.SourceName,
		Keywords:        string(kwJSON),
		Filters:         "{}",
		ScheduleMinutes: body.ScheduleMinutes,
		Enabled:         body.Enabled,
	})
	if err != nil {
		http.Error(w, `{"error":"save failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// --- Mention handlers ---

func (s *Server) listMentionsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	status := r.URL.Query().Get("status")
	source := r.URL.Query().Get("source")
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}
	mentions, total, err := s.meta.ListMentions(r.Context(), project.ID, status, source, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"list failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"mentions": mentions, "total": total})
}

func (s *Server) getMentionHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	m, err := s.meta.GetMention(r.Context(), project.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m)
}

func (s *Server) updateMentionHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	switch body.Status {
	case "new", "reviewed", "replied", "dismissed", "lead":
	default:
		http.Error(w, `{"error":"invalid status"}`, http.StatusBadRequest)
		return
	}
	if err := s.meta.UpdateMentionStatus(r.Context(), project.ID, r.PathValue("id"), body.Status); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) draftMentionReplyHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	m, err := s.meta.GetMention(r.Context(), project.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"error":"mention not found"}`, http.StatusNotFound)
		return
	}

	cfg, err := s.meta.GetLLMConfig(r.Context(), project.ID)
	if err != nil || cfg.Provider == "" {
		http.Error(w, `{"error":"LLM not configured"}`, http.StatusBadRequest)
		return
	}

	proj, _ := s.meta.GetProject(r.Context(), project.ID)
	desc := ""
	if proj != nil {
		desc = proj.Description
	}

	reply, err := ai.DraftMentionReply(r.Context(), cfg, ai.MentionDraftContext{
		ProjectDescription: desc,
		MentionTitle:       m.Title,
		MentionContent:     m.Content,
		MentionAuthor:      m.Author,
		Platform:           m.SourceName,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"draft failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	_ = s.meta.UpdateMentionReply(r.Context(), project.ID, m.ID, reply)
	_ = s.meta.UpdateMentionStatus(r.Context(), project.ID, m.ID, "reviewed")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"reply": reply})
}

func (s *Server) publishMentionReplyHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	m, err := s.meta.GetMention(r.Context(), project.ID, r.PathValue("id"))
	if err != nil {
		http.Error(w, `{"error":"mention not found"}`, http.StatusNotFound)
		return
	}

	var body struct {
		PublisherName string `json:"publisher_name"`
		ReplyText     string `json:"reply_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	if body.PublisherName == "" {
		body.PublisherName = m.SourceName
	}
	if body.ReplyText == "" {
		body.ReplyText = m.SuggestedReply
	}
	if body.ReplyText == "" {
		http.Error(w, `{"error":"no reply text"}`, http.StatusBadRequest)
		return
	}

	pub := s.registry.GetPublisher(body.PublisherName)
	if pub == nil {
		http.Error(w, `{"error":"publisher not found"}`, http.StatusNotFound)
		return
	}

	result, err := pub.Post(r.Context(), growth.PostContent{
		Body:    body.ReplyText,
		Channel: m.SourceName,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"publish failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	_ = s.meta.UpdateMentionStatus(r.Context(), project.ID, m.ID, "replied")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// --- Source monitor background job ---

func (s *Server) startSourceMonitor() {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for range ticker.C {
			s.runSourceMonitor(context.Background())
		}
	}()
}

func (s *Server) runSourceMonitor(ctx context.Context) {
	projects, err := s.meta.ListProjects(ctx)
	if err != nil {
		log.Printf("WARN source monitor: failed to list projects: %v", err)
		return
	}
	for _, proj := range projects {
		configs, err := s.meta.ListSourceConfigs(ctx, proj.ID)
		if err != nil {
			continue
		}
		for _, cfg := range configs {
			if !cfg.Enabled {
				continue
			}
			if cfg.LastRunAt != nil {
				next := cfg.LastRunAt.Add(time.Duration(cfg.ScheduleMinutes) * time.Minute)
				if time.Now().Before(next) {
					continue
				}
			}

			src := s.registry.GetSource(cfg.SourceName)
			if src == nil {
				continue
			}

			var keywords []string
			_ = json.Unmarshal([]byte(cfg.Keywords), &keywords)

			since := time.Now().Add(-24 * time.Hour)
			if cfg.LastRunAt != nil {
				since = *cfg.LastRunAt
			}

			mentions, err := src.Search(ctx, growth.SearchQuery{
				Keywords:    keywords,
				Since:       since,
				MaxResults:  25,
				ExtraFields: sourceCredentialFields(s.meta, ctx, proj.ID, cfg.SourceName),
			})
			if err != nil {
				log.Printf("WARN source monitor: search failed for %s/%s: %v", proj.ID, cfg.SourceName, err)
				continue
			}

			for _, m := range mentions {
				id, _ := generateID()
				_ = s.meta.UpsertMention(ctx, storage.MentionRecord{
					ID:             id,
					ProjectID:      proj.ID,
					SourceName:     cfg.SourceName,
					ExternalID:     m.ExternalID,
					ExternalURL:    m.ExternalURL,
					Author:         m.Author,
					Title:          m.Title,
					Content:        m.Content,
					RelevanceScore: scoreRelevance(m.Content, m.Title, keywords),
					ParentID:       m.ParentID,
					Metadata:       "{}",
					PostedAt:       &m.PostedAt,
				})
			}

			_ = s.meta.UpdateSourceConfigLastRun(ctx, proj.ID, cfg.SourceName, time.Now().UTC())
		}
	}
}

// --- Query concurrency limiter ---

// withQueryLimit wraps a handler to enforce the per-project concurrent DuckDB
// query limit. Requests that exceed the limit get a 429 response immediately.
func (s *Server) withQueryLimit(h http.Handler) http.Handler {
	if s.config.MaxConcurrentQueries <= 0 {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project := auth.ProjectFromContext(r.Context())
		if project == nil {
			h.ServeHTTP(w, r)
			return
		}
		val, _ := s.querySlots.LoadOrStore(project.ID,
			make(chan struct{}, s.config.MaxConcurrentQueries))
		sem := val.(chan struct{})
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			http.Error(w, `{"error":"too many concurrent queries, try again shortly"}`, http.StatusTooManyRequests)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// --- Lead limit middleware ---

// leadLimitCheck wraps a handler to enforce the per-project lead count limit.
func (s *Server) leadLimitCheck(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.config.ResourceLimitFn != nil {
			project := auth.ProjectFromContext(r.Context())
			if project != nil {
				if code, msg := s.config.ResourceLimitFn(r.Context(), project.ID, "leads"); code != 0 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(code)
					json.NewEncoder(w).Encode(map[string]any{"error": msg, "upgrade_required": true})
					return
				}
			}
		}
		h.ServeHTTP(w, r)
	})
}

// --- Dead letter queue ---

func (s *Server) deadLettersHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	deliveries, err := s.meta.ListDeadLetterDeliveries(r.Context(), project.ID, 50)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	// Attach webhook names for display.
	webhooks, _ := s.meta.ListCRMWebhooks(r.Context(), project.ID)
	nameMap := make(map[string]string, len(webhooks))
	for _, wh := range webhooks {
		nameMap[wh.ID] = wh.Name
	}
	type deadLetter struct {
		storage.WebhookDelivery
		WebhookName string `json:"webhook_name"`
	}
	result := make([]deadLetter, 0, len(deliveries))
	for _, d := range deliveries {
		result = append(result, deadLetter{
			WebhookDelivery: d,
			WebhookName:     nameMap[d.WebhookID],
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"dead_letters": result})
}

// --- Payload template ---

// buildWebhookPayload builds the JSON body for a webhook push. If the webhook
// has a payload_template, placeholders are substituted; otherwise the default
// envelope is used.
func buildWebhookPayload(wh storage.CRMWebhook, leads []storage.ScoredLead) []byte {
	if wh.PayloadTemplate == "" {
		b, _ := json.Marshal(map[string]any{
			"project_id": wh.ProjectID,
			"webhook":    wh.Name,
			"leads":      leads,
		})
		return b
	}
	leadsJSON, _ := json.Marshal(leads)
	ts := time.Now().UTC().Format(time.RFC3339)

	tmpl, err := template.New("payload").Parse(wh.PayloadTemplate)
	if err != nil {
		// Fallback to default on bad template.
		b, _ := json.Marshal(map[string]any{
			"project_id": wh.ProjectID,
			"webhook":    wh.Name,
			"leads":      leads,
		})
		return b
	}

	data := map[string]any{
		"leads":       string(leadsJSON),
		"lead_count":  len(leads),
		"project_id":  wh.ProjectID,
		"webhook_name": wh.Name,
		"timestamp":   ts,
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		b, _ := json.Marshal(map[string]any{
			"project_id": wh.ProjectID,
			"webhook":    wh.Name,
			"leads":      leads,
		})
		return b
	}
	return []byte(buf.String())
}

// sourceCredentialFields looks up stored per-project OAuth credentials for a source/publisher
// and returns them as an ExtraFields map for SearchQuery or PostContent.
func sourceCredentialFields(meta *storage.SQLite, ctx context.Context, projectID, sourceName string) map[string]string {
	creds, err := meta.GetSourceCredentials(ctx, projectID, sourceName)
	if err != nil || creds == nil {
		return nil
	}
	fields := map[string]string{}
	if creds.AccessToken != "" {
		fields["access_token"] = creds.AccessToken
	}
	if creds.RefreshToken != "" {
		fields["refresh_token"] = creds.RefreshToken
	}
	return fields
}

func generateCodeVerifier() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func (s *Server) getSourceCredentialsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	name := r.PathValue("name")
	creds, err := s.meta.GetSourceCredentials(r.Context(), project.ID, name)
	w.Header().Set("Content-Type", "application/json")
	if err != nil || creds == nil {
		json.NewEncoder(w).Encode(map[string]any{"connected": false})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"connected":    true,
		"username":     creds.Username,
		"connected_at": creds.UpdatedAt,
	})
}

func (s *Server) saveSourceCredentialsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	name := r.PathValue("name")

	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if body.RefreshToken == "" && body.AccessToken == "" {
		http.Error(w, `{"error":"access_token or refresh_token required"}`, http.StatusBadRequest)
		return
	}

	username, err := s.validateSourceToken(r.Context(), name, body.AccessToken, body.RefreshToken)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"credential validation failed: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if err := s.meta.UpsertSourceCredentials(r.Context(), storage.SourceCredentials{
		ProjectID:    project.ID,
		SourceName:   name,
		AccessToken:  body.AccessToken,
		RefreshToken: body.RefreshToken,
		Username:     username,
	}); err != nil {
		http.Error(w, `{"error":"failed to save credentials"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"connected": true, "username": username})
}

func (s *Server) deleteSourceCredentialsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	name := r.PathValue("name")
	_ = s.meta.DeleteSourceCredentials(r.Context(), project.ID, name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) sourceOAuthAuthorizeHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	name := r.PathValue("name")

	stateBytes := make([]byte, 16)
	rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)

	var redirectURI string
	if s.config.CloudMode && s.config.ControlPlaneURL != "" {
		// Route OAuth through the control plane proxy so all instances
		// share a single redirect URI registered with the provider.
		slug := strings.Split(r.Host, ".")[0]
		state = slug + "--" + state
		redirectURI = s.config.ControlPlaneURL + "/api/v1/oauth/" + name + "/callback"
	} else {
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		redirectURI = fmt.Sprintf("%s://%s/api/v1/sources/%s/oauth/callback", scheme, r.Host, name)
	}

	w.Header().Set("Content-Type", "application/json")

	switch name {
	case "reddit":
		clientID := os.Getenv("REDDIT_CLIENT_ID")
		if clientID == "" {
			json.NewEncoder(w).Encode(map[string]any{"oauth_available": false})
			return
		}
		if err := s.meta.SetOAuthStateExtra(r.Context(), state, project.ID, ""); err != nil {
			http.Error(w, `{"error":"failed to create oauth state"}`, http.StatusInternalServerError)
			return
		}
		params := url.Values{
			"client_id":     {clientID},
			"response_type": {"code"},
			"state":         {state},
			"redirect_uri":  {redirectURI},
			"duration":      {"permanent"},
			"scope":         {"identity read submit"},
		}
		json.NewEncoder(w).Encode(map[string]any{
			"oauth_available": true,
			"url":             "https://www.reddit.com/api/v1/authorize?" + params.Encode(),
		})

	case "twitter":
		clientID := os.Getenv("TWITTER_CLIENT_ID")
		if clientID == "" {
			json.NewEncoder(w).Encode(map[string]any{"oauth_available": false})
			return
		}
		verifier := generateCodeVerifier()
		challenge := pkceChallenge(verifier)
		if err := s.meta.SetOAuthStateExtra(r.Context(), state, project.ID, verifier); err != nil {
			http.Error(w, `{"error":"failed to create oauth state"}`, http.StatusInternalServerError)
			return
		}
		params := url.Values{
			"response_type":         {"code"},
			"client_id":             {clientID},
			"redirect_uri":          {redirectURI},
			"scope":                 {"tweet.read tweet.write users.read offline.access"},
			"state":                 {state},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}
		json.NewEncoder(w).Encode(map[string]any{
			"oauth_available": true,
			"url":             "https://twitter.com/i/oauth2/authorize?" + params.Encode(),
		})

	default:
		json.NewEncoder(w).Encode(map[string]any{"oauth_available": false})
	}
}

func (s *Server) sourceOAuthCallbackHandler(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errParam := r.URL.Query().Get("error")

	if errParam != "" {
		http.Redirect(w, r, "/growth/connectors?error=oauth_denied", http.StatusFound)
		return
	}
	if code == "" || state == "" {
		http.Redirect(w, r, "/growth/connectors?error=oauth_invalid", http.StatusFound)
		return
	}

	projectID, extra, err := s.meta.ValidateOAuthStateExtra(r.Context(), state)
	if err != nil {
		http.Redirect(w, r, "/growth/connectors?error=oauth_state_invalid", http.StatusFound)
		return
	}

	// The redirect URI for token exchange must match the one used in the
	// authorization request. In cloud mode that's the control plane proxy.
	var redirectURI string
	if s.config.CloudMode && s.config.ControlPlaneURL != "" {
		redirectURI = s.config.ControlPlaneURL + "/api/v1/oauth/" + name + "/callback"
	} else {
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		redirectURI = fmt.Sprintf("%s://%s/api/v1/sources/%s/oauth/callback", scheme, r.Host, name)
	}

	var accessToken, refreshToken, username string

	switch name {
	case "reddit":
		clientID := os.Getenv("REDDIT_CLIENT_ID")
		clientSecret := os.Getenv("REDDIT_CLIENT_SECRET")
		accessToken, refreshToken, err = exchangeRedditCode(r.Context(), clientID, clientSecret, code, redirectURI)
		if err != nil {
			log.Printf("reddit oauth exchange error: %v", err)
			http.Redirect(w, r, "/growth/connectors?error=oauth_exchange_failed", http.StatusFound)
			return
		}
		username, _ = fetchRedditUsername(r.Context(), accessToken)

	case "twitter":
		clientID := os.Getenv("TWITTER_CLIENT_ID")
		clientSecret := os.Getenv("TWITTER_CLIENT_SECRET")
		accessToken, refreshToken, err = exchangeTwitterCode(r.Context(), clientID, clientSecret, code, redirectURI, extra)
		if err != nil {
			log.Printf("twitter oauth exchange error: %v", err)
			http.Redirect(w, r, "/growth/connectors?error=oauth_exchange_failed", http.StatusFound)
			return
		}
		username, _ = fetchTwitterUsername(r.Context(), accessToken)

	default:
		http.Redirect(w, r, "/growth/connectors?error=unknown_source", http.StatusFound)
		return
	}

	if err := s.meta.UpsertSourceCredentials(r.Context(), storage.SourceCredentials{
		ProjectID:    projectID,
		SourceName:   name,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Username:     username,
	}); err != nil {
		log.Printf("save source credentials error: %v", err)
		http.Redirect(w, r, "/growth/connectors?error=save_failed", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/growth/connectors?connected="+url.QueryEscape(name), http.StatusFound)
}

func exchangeRedditCode(ctx context.Context, clientID, clientSecret, code, redirectURI string) (accessToken, refreshToken string, err error) {
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://www.reddit.com/api/v1/access_token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", "", err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "ClickNest/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decode reddit token response: %w", err)
	}
	if result.Error != "" {
		return "", "", fmt.Errorf("reddit token error: %s", result.Error)
	}
	return result.AccessToken, result.RefreshToken, nil
}

func fetchRedditUsername(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://oauth.reddit.com/api/v1/me", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "ClickNest/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var me struct{ Name string `json:"name"` }
	json.NewDecoder(resp.Body).Decode(&me)
	return me.Name, nil
}

func exchangeTwitterCode(ctx context.Context, clientID, clientSecret, code, redirectURI, codeVerifier string) (accessToken, refreshToken string, err error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.twitter.com/2/oauth2/token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", "", err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decode twitter token response: %w", err)
	}
	if result.Error != "" {
		return "", "", fmt.Errorf("twitter token error: %s: %s", result.Error, result.ErrorDesc)
	}
	return result.AccessToken, result.RefreshToken, nil
}

func fetchTwitterUsername(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitter.com/2/users/me", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var me struct {
		Data struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&me)
	return me.Data.Username, nil
}

// validateSourceToken calls the platform API to verify credentials and return the username.
func (s *Server) validateSourceToken(ctx context.Context, sourceName, accessToken, refreshToken string) (string, error) {
	switch sourceName {
	case "reddit":
		clientID := os.Getenv("REDDIT_CLIENT_ID")
		clientSecret := os.Getenv("REDDIT_CLIENT_SECRET")
		// If no access token, try to exchange the refresh token for one.
		if accessToken == "" && refreshToken != "" && clientID != "" {
			form := url.Values{
				"grant_type":    {"refresh_token"},
				"refresh_token": {refreshToken},
			}
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
				"https://www.reddit.com/api/v1/access_token",
				strings.NewReader(form.Encode()),
			)
			req.SetBasicAuth(clientID, clientSecret)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("User-Agent", "ClickNest/1.0")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()
			var tok struct {
				AccessToken string `json:"access_token"`
				Error       string `json:"error"`
			}
			json.NewDecoder(resp.Body).Decode(&tok)
			if tok.Error != "" {
				return "", fmt.Errorf("reddit token error: %s", tok.Error)
			}
			accessToken = tok.AccessToken
		}
		if accessToken == "" {
			return "", fmt.Errorf("no valid access token available")
		}
		return fetchRedditUsername(ctx, accessToken)

	case "twitter":
		if accessToken == "" {
			return "", fmt.Errorf("access_token required for Twitter")
		}
		return fetchTwitterUsername(ctx, accessToken)

	default:
		return "", fmt.Errorf("unsupported source: %s", sourceName)
	}
}

// ---- Inbound lead ingestion -------------------------------------------------

func (s *Server) ingestLeadsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var payload struct {
		Leads []struct {
			Email      string         `json:"email"`
			Name       string         `json:"name,omitempty"`
			Source     string         `json:"source,omitempty"`
			Attributes map[string]any `json:"attributes,omitempty"`
		} `json:"leads"`
		// Single-lead shorthand (no wrapping array needed).
		Email      string         `json:"email,omitempty"`
		Name       string         `json:"name,omitempty"`
		Source     string         `json:"source,omitempty"`
		Attributes map[string]any `json:"attributes,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	// Support both single-lead and batch formats.
	leads := payload.Leads
	if len(leads) == 0 && payload.Email != "" {
		leads = append(leads, struct {
			Email      string         `json:"email"`
			Name       string         `json:"name,omitempty"`
			Source     string         `json:"source,omitempty"`
			Attributes map[string]any `json:"attributes,omitempty"`
		}{
			Email:      payload.Email,
			Name:       payload.Name,
			Source:     payload.Source,
			Attributes: payload.Attributes,
		})
	}

	if len(leads) == 0 {
		http.Error(w, `{"error":"at least one lead with an email is required"}`, http.StatusBadRequest)
		return
	}

	if len(leads) > 1000 {
		http.Error(w, `{"error":"max 1000 leads per request"}`, http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	events := make([]storage.Event, 0, len(leads))

	for _, lead := range leads {
		if lead.Email == "" {
			continue
		}

		props := make(map[string]any)
		if lead.Name != "" {
			props["name"] = lead.Name
		}
		if lead.Source != "" {
			props["source"] = lead.Source
		}
		props["email"] = lead.Email
		for k, v := range lead.Attributes {
			props[k] = v
		}

		events = append(events, storage.Event{
			ProjectID:  project.ID,
			SessionID:  "ext_" + lead.Email,
			DistinctID: lead.Email,
			EventType:  "identify",
			URL:        "external://" + lead.Source,
			URLPath:    "/external/" + lead.Source,
			Timestamp:  now,
			Properties: props,
		})
	}

	if len(events) == 0 {
		http.Error(w, `{"error":"no valid leads (email required)"}`, http.StatusBadRequest)
		return
	}

	if err := s.events.InsertEvents(r.Context(), events); err != nil {
		log.Printf("ERROR inserting external leads: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	if s.config.OnEventIngested != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.config.OnEventIngested(ctx, project.ID, int64(len(events)))
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "ok",
		"accepted": len(events),
	})
}

// ---- Lead score history + attribution handlers --------------------------------

func (s *Server) leadScoreHistoryHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	distinctID := r.PathValue("id")
	history, err := s.meta.GetLeadScoreHistory(r.Context(), project.ID, distinctID, 30)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"history": history})
}

func (s *Server) leadAttributionHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	distinctID := r.PathValue("id")
	sources, err := s.events.QueryLeadAttribution(r.Context(), project.ID, distinctID, 90)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"sources": sources})
}

// ---- Segment handlers --------------------------------------------------------

func (s *Server) listSegmentsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	segs, err := s.meta.ListSegments(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"segments": segs})
}

func (s *Server) createSegmentHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		Name       string `json:"name"`
		Conditions string `json:"conditions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	if body.Conditions == "" {
		body.Conditions = "[]"
	}
	seg, err := s.meta.CreateSegment(r.Context(), project.ID, body.Name, body.Conditions)
	if err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(seg)
}

func (s *Server) deleteSegmentHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	if err := s.meta.DeleteSegment(r.Context(), project.ID, id); err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) segmentMembersHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")
	seg, err := s.meta.GetSegment(r.Context(), project.ID, id)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	// Parse segment conditions as scoring rules and run a lead score query.
	var conditions []storage.ScoringRule
	if err := json.Unmarshal([]byte(seg.Conditions), &conditions); err != nil {
		http.Error(w, `{"error":"invalid conditions"}`, http.StatusBadRequest)
		return
	}

	start := time.Now().UTC().Add(-30 * 24 * time.Hour)
	end := time.Now().UTC()
	leads, total, err := s.events.QueryLeadScores(r.Context(), project.ID, conditions, start, end, 200, 0)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	// Filter to only include users with score > 0 (actually matching at least one condition).
	var members []storage.ScoredLead
	for _, l := range leads {
		if l.Score > 0 {
			members = append(members, l)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"members": members, "total": total})
}

// ---- ICP settings handlers ---------------------------------------------------

func (s *Server) getICPSettingsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	autoRefresh, _ := s.meta.GetGrowthSetting(r.Context(), project.ID, "icp_auto_refresh")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"icp_auto_refresh": autoRefresh == "true",
	})
}

func (s *Server) putICPSettingsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		ICPAutoRefresh bool `json:"icp_auto_refresh"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	val := "false"
	if body.ICPAutoRefresh {
		val = "true"
	}
	if err := s.meta.SetGrowthSetting(r.Context(), project.ID, "icp_auto_refresh", val); err != nil {
		http.Error(w, `{"error":"save failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Lead score snapshot background job -------------------------------------

func (s *Server) startLeadScoreSnapshotter() {
	go func() {
		// Delay initial run to let the server warm up, then run at midnight-ish.
		now := time.Now().UTC()
		nextRun := time.Date(now.Year(), now.Month(), now.Day()+1, 3, 0, 0, 0, time.UTC)
		time.Sleep(time.Until(nextRun))
		s.runLeadScoreSnapshot(context.Background())
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			s.runLeadScoreSnapshot(context.Background())
		}
	}()
}

func (s *Server) runLeadScoreSnapshot(ctx context.Context) {
	projects, err := s.meta.ListProjects(ctx)
	if err != nil {
		log.Printf("WARN score snapshot: list projects: %v", err)
		return
	}
	today := time.Now().UTC().Format("2006-01-02")
	start := time.Now().UTC().Add(-30 * 24 * time.Hour)
	end := time.Now().UTC()

	for _, proj := range projects {
		rules, err := s.meta.ListScoringRules(ctx, proj.ID)
		if err != nil || len(rules) == 0 {
			continue // skip projects without scoring rules
		}
		leads, _, err := s.events.QueryLeadScores(ctx, proj.ID, rules, start, end, 1000, 0)
		if err != nil {
			log.Printf("WARN score snapshot: query project %s: %v", proj.ID, err)
			continue
		}
		for _, lead := range leads {
			if lead.Score <= 0 {
				continue
			}
			if err := s.meta.UpsertLeadScoreSnapshot(ctx, proj.ID, lead.DistinctID, today, lead.Score, lead.RawScore); err != nil {
				log.Printf("WARN score snapshot: upsert %s/%s: %v", proj.ID, lead.DistinctID, err)
			}
		}
		log.Printf("Score snapshot: saved %d leads for project %s", len(leads), proj.ID)
	}
}

// ---- ICP auto-refresh background job ----------------------------------------

func (s *Server) startICPAutoRefresh() {
	go func() {
		time.Sleep(5 * time.Minute) // wait for server to be ready
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			s.runICPAutoRefresh(context.Background())
		}
	}()
}

func (s *Server) runICPAutoRefresh(ctx context.Context) {
	projectIDs, err := s.meta.ListProjectsWithSetting(ctx, "icp_auto_refresh", "true")
	if err != nil || len(projectIDs) == 0 {
		return
	}

	const weeklyInterval = 7 * 24 * time.Hour

	for _, projectID := range projectIDs {
		analyses, err := s.meta.ListICPAnalyses(ctx, projectID, 1)
		if err != nil || len(analyses) == 0 {
			continue
		}
		last := analyses[0]
		if time.Since(last.CreatedAt) < weeklyInterval {
			continue // not due yet
		}

		// Re-run with the same conversion pages.
		var convPaths []string
		if err := json.Unmarshal([]byte(last.ConversionPages), &convPaths); err != nil || len(convPaths) == 0 {
			continue
		}

		cfg, err := s.meta.GetLLMConfig(ctx, projectID)
		if err != nil || cfg == nil || cfg.Provider == "" {
			continue // no LLM configured
		}

		now := time.Now().UTC()
		profiles, err := s.events.QueryICPProfiles(ctx, projectID, convPaths, now.Add(-30*24*time.Hour), now, 50)
		if err != nil || len(profiles) == 0 {
			continue
		}

		aiProfiles := make([]ai.ICPProfile, len(profiles))
		for i, p := range profiles {
			aiProfiles[i] = ai.ICPProfile{
				DistinctID:   p.DistinctID,
				SessionCount: p.SessionCount,
				EventCount:   p.EventCount,
				TopPages:     p.TopPages,
				EntrySource:  p.EntrySource,
			}
		}

		proj, _ := s.meta.GetProject(ctx, projectID)
		desc := ""
		if proj != nil {
			desc = proj.Description
		}

		analysis, err := ai.AnalyzeICP(ctx, cfg, aiProfiles, desc)
		if err != nil {
			log.Printf("WARN ICP auto-refresh project %s: %v", projectID, err)
			continue
		}

		analysisID, _ := generateID()
		pagesJSON, _ := json.Marshal(convPaths)
		traitsJSON, _ := json.Marshal(analysis.CommonTraits)
		channelsJSON, _ := json.Marshal(analysis.BestChannels)
		recsJSON, _ := json.Marshal(analysis.Recommendations)
		if err := s.meta.CreateICPAnalysis(ctx, storage.ICPAnalysis{
			ID:              analysisID,
			ProjectID:       projectID,
			ConversionPages: string(pagesJSON),
			Summary:         analysis.Summary,
			Traits:          string(traitsJSON),
			Channels:        string(channelsJSON),
			Recommendations: string(recsJSON),
			ProfileCount:    len(profiles),
		}); err != nil {
			log.Printf("WARN ICP auto-refresh save project %s: %v", projectID, err)
		} else {
			log.Printf("ICP auto-refresh: saved new analysis for project %s", projectID)
		}
	}
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
