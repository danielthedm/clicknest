package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	APIKey      string    `json:"api_key"`
	CreatedAt   time.Time `json:"created_at"`
}

type EventName struct {
	Fingerprint string   `json:"fingerprint"`
	ProjectID   string   `json:"project_id"`
	AIName      string   `json:"ai_name"`
	UserName    *string  `json:"user_name,omitempty"`
	SourceFile  *string  `json:"source_file,omitempty"`
	Confidence  *float64 `json:"confidence,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type LLMConfig struct {
	ProjectID string  `json:"project_id"`
	Provider  string  `json:"provider"`
	APIKey    *string `json:"api_key,omitempty"`
	Model     string  `json:"model"`
	BaseURL   *string `json:"base_url,omitempty"`
}

type GitHubConnection struct {
	ProjectID     string     `json:"project_id"`
	RepoOwner     string     `json:"repo_owner"`
	RepoName      string     `json:"repo_name"`
	AccessToken   string     `json:"-"`
	DefaultBranch string     `json:"default_branch"`
	LastSyncedAt  *time.Time `json:"last_synced_at,omitempty"`
}

type SQLite struct {
	db  *sql.DB
	enc *Encryptor
}

// NewSQLite opens a SQLite database at the given path and runs migrations.
// The enc parameter may be nil to disable encryption.
func NewSQLite(path string, enc *Encryptor) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("enabling WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	if err := RunMigrations(db, sqliteMigrations, "migrations/sqlite"); err != nil {
		return nil, fmt.Errorf("running sqlite migrations: %w", err)
	}

	return &SQLite{db: db, enc: enc}, nil
}

// --- Projects ---

// CreateProject creates a new project with a generated API key.
func (s *SQLite) CreateProject(ctx context.Context, id, name string) (*Project, error) {
	apiKey, err := generateAPIKey()
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO projects (id, name, api_key) VALUES (?, ?, ?)`,
		id, name, apiKey,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting project: %w", err)
	}

	return &Project{ID: id, Name: name, APIKey: apiKey, CreatedAt: time.Now()}, nil
}

func (s *SQLite) GetProject(ctx context.Context, id string) (*Project, error) {
	var p Project
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, api_key, created_at FROM projects WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.Description, &p.APIKey, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *SQLite) GetProjectByAPIKey(ctx context.Context, apiKey string) (*Project, error) {
	var p Project
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, api_key, created_at FROM projects WHERE api_key = ?`, apiKey,
	).Scan(&p.ID, &p.Name, &p.Description, &p.APIKey, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *SQLite) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, api_key, created_at FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.APIKey, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (s *SQLite) UpdateProjectDescription(ctx context.Context, id, description string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE projects SET description = ? WHERE id = ?`,
		description, id,
	)
	return err
}

// --- Event Names ---

func (s *SQLite) GetEventName(ctx context.Context, projectID, fingerprint string) (*EventName, error) {
	var en EventName
	err := s.db.QueryRowContext(ctx,
		`SELECT fingerprint, project_id, ai_name, user_name, source_file, confidence, created_at
		 FROM event_names WHERE project_id = ? AND fingerprint = ?`,
		projectID, fingerprint,
	).Scan(&en.Fingerprint, &en.ProjectID, &en.AIName, &en.UserName, &en.SourceFile, &en.Confidence, &en.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &en, nil
}

func (s *SQLite) BatchGetEventNames(ctx context.Context, projectID string, fingerprints []string) (map[string]*EventName, error) {
	if len(fingerprints) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(fingerprints))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(fingerprints)+1)
	args = append(args, projectID)
	for _, fp := range fingerprints {
		args = append(args, fp)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT fingerprint, project_id, ai_name, user_name, source_file, confidence, created_at
		 FROM event_names WHERE project_id = ? AND fingerprint IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]*EventName, len(fingerprints))
	for rows.Next() {
		var en EventName
		if err := rows.Scan(&en.Fingerprint, &en.ProjectID, &en.AIName, &en.UserName, &en.SourceFile, &en.Confidence, &en.CreatedAt); err != nil {
			return nil, err
		}
		result[en.Fingerprint] = &en
	}
	return result, rows.Err()
}

func (s *SQLite) SetEventName(ctx context.Context, en EventName) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO event_names (fingerprint, project_id, ai_name, source_file, confidence)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (fingerprint, project_id)
		 DO UPDATE SET ai_name = excluded.ai_name, source_file = excluded.source_file, confidence = excluded.confidence`,
		en.Fingerprint, en.ProjectID, en.AIName, en.SourceFile, en.Confidence,
	)
	return err
}

// OverrideEventName sets a user-provided name that takes priority over the AI name.
func (s *SQLite) OverrideEventName(ctx context.Context, projectID, fingerprint, userName string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE event_names SET user_name = ? WHERE project_id = ? AND fingerprint = ?`,
		userName, projectID, fingerprint,
	)
	return err
}

func (s *SQLite) ListEventNames(ctx context.Context, projectID string) ([]EventName, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT fingerprint, project_id, ai_name, user_name, source_file, confidence, created_at
		 FROM event_names WHERE project_id = ? ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []EventName
	for rows.Next() {
		var en EventName
		if err := rows.Scan(&en.Fingerprint, &en.ProjectID, &en.AIName, &en.UserName, &en.SourceFile, &en.Confidence, &en.CreatedAt); err != nil {
			return nil, err
		}
		names = append(names, en)
	}
	return names, rows.Err()
}

// --- LLM Config ---

func (s *SQLite) GetLLMConfig(ctx context.Context, projectID string) (*LLMConfig, error) {
	var c LLMConfig
	err := s.db.QueryRowContext(ctx,
		`SELECT project_id, provider, api_key, model, base_url FROM llm_config WHERE project_id = ?`,
		projectID,
	).Scan(&c.ProjectID, &c.Provider, &c.APIKey, &c.Model, &c.BaseURL)
	if err != nil {
		return nil, err
	}
	decKey, err := s.enc.DecryptPtr(c.APIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting llm api key: %w", err)
	}
	c.APIKey = decKey
	return &c, nil
}

func (s *SQLite) SetLLMConfig(ctx context.Context, c LLMConfig) error {
	encKey, err := s.enc.EncryptPtr(c.APIKey)
	if err != nil {
		return fmt.Errorf("encrypting llm api key: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO llm_config (project_id, provider, api_key, model, base_url)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (project_id)
		 DO UPDATE SET provider = excluded.provider, api_key = excluded.api_key, model = excluded.model, base_url = excluded.base_url`,
		c.ProjectID, c.Provider, encKey, c.Model, c.BaseURL,
	)
	return err
}

// --- GitHub Connections ---

func (s *SQLite) GetGitHubConnection(ctx context.Context, projectID string) (*GitHubConnection, error) {
	var g GitHubConnection
	err := s.db.QueryRowContext(ctx,
		`SELECT project_id, repo_owner, repo_name, access_token, default_branch, last_synced_at
		 FROM github_connections WHERE project_id = ?`,
		projectID,
	).Scan(&g.ProjectID, &g.RepoOwner, &g.RepoName, &g.AccessToken, &g.DefaultBranch, &g.LastSyncedAt)
	if err != nil {
		return nil, err
	}
	decToken, err := s.enc.Decrypt(g.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("decrypting github access token: %w", err)
	}
	g.AccessToken = decToken
	return &g, nil
}

func (s *SQLite) SetGitHubConnection(ctx context.Context, g GitHubConnection) error {
	encToken, err := s.enc.Encrypt(g.AccessToken)
	if err != nil {
		return fmt.Errorf("encrypting github access token: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO github_connections (project_id, repo_owner, repo_name, access_token, default_branch)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (project_id)
		 DO UPDATE SET repo_owner = excluded.repo_owner, repo_name = excluded.repo_name,
		   access_token = excluded.access_token, default_branch = excluded.default_branch`,
		g.ProjectID, g.RepoOwner, g.RepoName, encToken, g.DefaultBranch,
	)
	return err
}

// --- OAuth State ---

// SetOAuthState stores a CSRF state token for the OAuth flow.
func (s *SQLite) SetOAuthState(ctx context.Context, state, projectID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO oauth_state (state, project_id) VALUES (?, ?)`,
		state, projectID,
	)
	return err
}

// ValidateOAuthState checks that a state token exists and was created less than 10 minutes ago.
// On success it deletes the token (one-time use) and returns the associated project ID.
func (s *SQLite) ValidateOAuthState(ctx context.Context, state string) (string, error) {
	var projectID string
	err := s.db.QueryRowContext(ctx,
		`SELECT project_id FROM oauth_state
		 WHERE state = ? AND created_at > datetime('now', '-10 minutes')`,
		state,
	).Scan(&projectID)
	if err != nil {
		return "", fmt.Errorf("invalid or expired oauth state")
	}

	// Delete after use (one-time).
	s.db.ExecContext(ctx, `DELETE FROM oauth_state WHERE state = ?`, state)

	return projectID, nil
}

func (s *SQLite) DB() *sql.DB {
	return s.db
}

func (s *SQLite) UpsertSourceIndex(ctx context.Context, projectID, filePath, componentName, selectors, contentHash string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO source_index (project_id, file_path, component_name, selectors, content_hash)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (project_id, file_path)
		 DO UPDATE SET component_name = excluded.component_name, selectors = excluded.selectors,
		   content_hash = excluded.content_hash, updated_at = CURRENT_TIMESTAMP`,
		projectID, filePath, componentName, selectors, contentHash,
	)
	return err
}

// --- Funnels ---

type Funnel struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	Steps     string    `json:"steps"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *SQLite) CreateFunnel(ctx context.Context, f Funnel) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO funnels (id, project_id, name, steps) VALUES (?, ?, ?, ?)`,
		f.ID, f.ProjectID, f.Name, f.Steps,
	)
	return err
}

func (s *SQLite) GetFunnel(ctx context.Context, projectID, id string) (*Funnel, error) {
	var f Funnel
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, steps, created_at FROM funnels WHERE project_id = ? AND id = ?`,
		projectID, id,
	).Scan(&f.ID, &f.ProjectID, &f.Name, &f.Steps, &f.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *SQLite) ListFunnels(ctx context.Context, projectID string) ([]Funnel, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, steps, created_at FROM funnels WHERE project_id = ? ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var funnels []Funnel
	for rows.Next() {
		var f Funnel
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Name, &f.Steps, &f.CreatedAt); err != nil {
			return nil, err
		}
		funnels = append(funnels, f)
	}
	return funnels, rows.Err()
}

func (s *SQLite) DeleteFunnel(ctx context.Context, projectID, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM funnels WHERE project_id = ? AND id = ?`,
		projectID, id,
	)
	return err
}

// --- Dashboards ---

type Dashboard struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	Config    string    `json:"config"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *SQLite) CreateDashboard(ctx context.Context, d Dashboard) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO dashboards (id, project_id, name, config) VALUES (?, ?, ?, ?)`,
		d.ID, d.ProjectID, d.Name, d.Config,
	)
	return err
}

func (s *SQLite) GetDashboard(ctx context.Context, projectID, id string) (*Dashboard, error) {
	var d Dashboard
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, config, created_at, updated_at FROM dashboards WHERE project_id = ? AND id = ?`,
		projectID, id,
	).Scan(&d.ID, &d.ProjectID, &d.Name, &d.Config, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *SQLite) ListDashboards(ctx context.Context, projectID string) ([]Dashboard, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, config, created_at, updated_at FROM dashboards WHERE project_id = ? ORDER BY updated_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dashboards []Dashboard
	for rows.Next() {
		var d Dashboard
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Name, &d.Config, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		dashboards = append(dashboards, d)
	}
	return dashboards, rows.Err()
}

func (s *SQLite) UpdateDashboard(ctx context.Context, d Dashboard) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE dashboards SET name = ?, config = ?, updated_at = CURRENT_TIMESTAMP WHERE project_id = ? AND id = ?`,
		d.Name, d.Config, d.ProjectID, d.ID,
	)
	return err
}

func (s *SQLite) DeleteDashboard(ctx context.Context, projectID, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM dashboards WHERE project_id = ? AND id = ?`,
		projectID, id,
	)
	return err
}

// --- Feature Flags ---

type FeatureFlag struct {
	ID                string    `json:"id"`
	ProjectID         string    `json:"project_id"`
	Key               string    `json:"key"`
	Name              string    `json:"name"`
	Enabled           bool      `json:"enabled"`
	RolloutPercentage int       `json:"rollout_percentage"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (s *SQLite) CreateFeatureFlag(ctx context.Context, f FeatureFlag) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO feature_flags (id, project_id, key, name, enabled, rollout_percentage) VALUES (?, ?, ?, ?, ?, ?)`,
		f.ID, f.ProjectID, f.Key, f.Name, b2i(f.Enabled), f.RolloutPercentage,
	)
	return err
}

func (s *SQLite) ListFeatureFlags(ctx context.Context, projectID string) ([]FeatureFlag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, key, name, enabled, rollout_percentage, created_at, updated_at
		 FROM feature_flags WHERE project_id = ? ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flags []FeatureFlag
	for rows.Next() {
		var f FeatureFlag
		var enabledInt int
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Key, &f.Name, &enabledInt, &f.RolloutPercentage, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.Enabled = enabledInt != 0
		flags = append(flags, f)
	}
	return flags, rows.Err()
}

func (s *SQLite) UpdateFeatureFlag(ctx context.Context, projectID, id string, enabled bool, rolloutPct int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE feature_flags SET enabled = ?, rollout_percentage = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE project_id = ? AND id = ?`,
		b2i(enabled), rolloutPct, projectID, id,
	)
	return err
}

func (s *SQLite) DeleteFeatureFlag(ctx context.Context, projectID, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM feature_flags WHERE project_id = ? AND id = ?`,
		projectID, id,
	)
	return err
}

// --- Alerts ---

type Alert struct {
	ID              string     `json:"id"`
	ProjectID       string     `json:"project_id"`
	Name            string     `json:"name"`
	Metric          string     `json:"metric"`
	EventName       string     `json:"event_name,omitempty"`
	Threshold       int        `json:"threshold"`
	WindowMinutes   int        `json:"window_minutes"`
	WebhookURL      string     `json:"webhook_url"`
	Enabled         bool       `json:"enabled"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (s *SQLite) CreateAlert(ctx context.Context, a Alert) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO alerts (id, project_id, name, metric, event_name, threshold, window_minutes, webhook_url, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.ProjectID, a.Name, a.Metric, a.EventName, a.Threshold, a.WindowMinutes, a.WebhookURL, b2i(a.Enabled),
	)
	return err
}

func (s *SQLite) ListAlerts(ctx context.Context, projectID string) ([]Alert, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, metric, event_name, threshold, window_minutes, webhook_url, enabled, last_triggered_at, created_at
		 FROM alerts WHERE project_id = ? ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlerts(rows)
}

// ListAllEnabledAlerts returns all enabled alerts across all projects (for background checker).
func (s *SQLite) ListAllEnabledAlerts(ctx context.Context) ([]Alert, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, metric, event_name, threshold, window_minutes, webhook_url, enabled, last_triggered_at, created_at
		 FROM alerts WHERE enabled = 1 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlerts(rows)
}

func scanAlerts(rows *sql.Rows) ([]Alert, error) {
	var alerts []Alert
	for rows.Next() {
		var a Alert
		var enabledInt int
		var eventName sql.NullString
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Metric, &eventName, &a.Threshold,
			&a.WindowMinutes, &a.WebhookURL, &enabledInt, &a.LastTriggeredAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Enabled = enabledInt != 0
		if eventName.Valid {
			a.EventName = eventName.String
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (s *SQLite) UpdateAlert(ctx context.Context, projectID, id string, enabled bool, threshold int, webhookURL string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alerts SET enabled = ?, threshold = ?, webhook_url = ? WHERE project_id = ? AND id = ?`,
		b2i(enabled), threshold, webhookURL, projectID, id,
	)
	return err
}

func (s *SQLite) DeleteAlert(ctx context.Context, projectID, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM alerts WHERE project_id = ? AND id = ?`,
		projectID, id,
	)
	return err
}

func (s *SQLite) UpdateAlertTriggered(ctx context.Context, id string, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alerts SET last_triggered_at = ? WHERE id = ?`,
		t, id,
	)
	return err
}

// --- Ref Codes ---

type RefCode struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *SQLite) CreateRefCode(ctx context.Context, rc RefCode) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ref_codes (id, project_id, code, name, notes) VALUES (?, ?, ?, ?, ?)`,
		rc.ID, rc.ProjectID, rc.Code, rc.Name, rc.Notes,
	)
	return err
}

func (s *SQLite) ListRefCodes(ctx context.Context, projectID string) ([]RefCode, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, code, name, notes, created_at, updated_at
		 FROM ref_codes WHERE project_id = ? ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codes []RefCode
	for rows.Next() {
		var rc RefCode
		if err := rows.Scan(&rc.ID, &rc.ProjectID, &rc.Code, &rc.Name, &rc.Notes, &rc.CreatedAt, &rc.UpdatedAt); err != nil {
			return nil, err
		}
		codes = append(codes, rc)
	}
	return codes, rows.Err()
}

func (s *SQLite) UpdateRefCode(ctx context.Context, projectID, id, name, notes string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE ref_codes SET name = ?, notes = ?, updated_at = CURRENT_TIMESTAMP WHERE project_id = ? AND id = ?`,
		name, notes, projectID, id,
	)
	return err
}

func (s *SQLite) DeleteRefCode(ctx context.Context, projectID, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM ref_codes WHERE project_id = ? AND id = ?`,
		projectID, id,
	)
	return err
}

// --- Scoring Rules ---

type ScoringRule struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	RuleType  string    `json:"rule_type"`
	Config    string    `json:"config"`
	Points    int       `json:"points"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *SQLite) CreateScoringRule(ctx context.Context, r ScoringRule) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO scoring_rules (id, project_id, name, rule_type, config, points, enabled) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProjectID, r.Name, r.RuleType, r.Config, r.Points, b2i(r.Enabled),
	)
	return err
}

func (s *SQLite) ListScoringRules(ctx context.Context, projectID string) ([]ScoringRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, rule_type, config, points, enabled, created_at, updated_at
		 FROM scoring_rules WHERE project_id = ? ORDER BY created_at DESC`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []ScoringRule
	for rows.Next() {
		var r ScoringRule
		var enabledInt int
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Name, &r.RuleType, &r.Config, &r.Points, &enabledInt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabledInt != 0
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *SQLite) UpdateScoringRule(ctx context.Context, projectID, id string, name, ruleType, config string, points int, enabled bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE scoring_rules SET name = ?, rule_type = ?, config = ?, points = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE project_id = ? AND id = ?`,
		name, ruleType, config, points, b2i(enabled), projectID, id,
	)
	return err
}

func (s *SQLite) DeleteScoringRule(ctx context.Context, projectID, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM scoring_rules WHERE project_id = ? AND id = ?`, projectID, id,
	)
	return err
}

// --- CRM Webhooks ---

type CRMWebhook struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"project_id"`
	Name         string     `json:"name"`
	WebhookURL   string     `json:"webhook_url"`
	MinScore     int        `json:"min_score"`
	Enabled      bool       `json:"enabled"`
	LastPushedAt *time.Time `json:"last_pushed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (s *SQLite) CreateCRMWebhook(ctx context.Context, w CRMWebhook) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO crm_webhooks (id, project_id, name, webhook_url, min_score, enabled) VALUES (?, ?, ?, ?, ?, ?)`,
		w.ID, w.ProjectID, w.Name, w.WebhookURL, w.MinScore, b2i(w.Enabled),
	)
	return err
}

func (s *SQLite) ListCRMWebhooks(ctx context.Context, projectID string) ([]CRMWebhook, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, webhook_url, min_score, enabled, last_pushed_at, created_at, updated_at
		 FROM crm_webhooks WHERE project_id = ? ORDER BY created_at DESC`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var webhooks []CRMWebhook
	for rows.Next() {
		var wh CRMWebhook
		var enabledInt int
		if err := rows.Scan(&wh.ID, &wh.ProjectID, &wh.Name, &wh.WebhookURL, &wh.MinScore, &enabledInt, &wh.LastPushedAt, &wh.CreatedAt, &wh.UpdatedAt); err != nil {
			return nil, err
		}
		wh.Enabled = enabledInt != 0
		webhooks = append(webhooks, wh)
	}
	return webhooks, rows.Err()
}

func (s *SQLite) ListAllEnabledCRMWebhooks(ctx context.Context) ([]CRMWebhook, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, webhook_url, min_score, enabled, last_pushed_at, created_at, updated_at
		 FROM crm_webhooks WHERE enabled = 1 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var webhooks []CRMWebhook
	for rows.Next() {
		var wh CRMWebhook
		var enabledInt int
		if err := rows.Scan(&wh.ID, &wh.ProjectID, &wh.Name, &wh.WebhookURL, &wh.MinScore, &enabledInt, &wh.LastPushedAt, &wh.CreatedAt, &wh.UpdatedAt); err != nil {
			return nil, err
		}
		wh.Enabled = enabledInt != 0
		webhooks = append(webhooks, wh)
	}
	return webhooks, rows.Err()
}

func (s *SQLite) UpdateCRMWebhook(ctx context.Context, projectID, id string, name, webhookURL string, minScore int, enabled bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE crm_webhooks SET name = ?, webhook_url = ?, min_score = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE project_id = ? AND id = ?`,
		name, webhookURL, minScore, b2i(enabled), projectID, id,
	)
	return err
}

func (s *SQLite) UpdateCRMWebhookPushed(ctx context.Context, id string, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE crm_webhooks SET last_pushed_at = ? WHERE id = ?`, t, id,
	)
	return err
}

func (s *SQLite) DeleteCRMWebhook(ctx context.Context, projectID, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM crm_webhooks WHERE project_id = ? AND id = ?`, projectID, id,
	)
	return err
}

// --- Campaigns ---

type Campaign struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	Channel   string    `json:"channel"`
	RefCodeID string    `json:"ref_code_id,omitempty"`
	Status    string    `json:"status"`
	Content   string    `json:"content"`
	AIPrompt  string    `json:"ai_prompt"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *SQLite) CreateCampaign(ctx context.Context, c Campaign) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO campaigns (id, project_id, name, channel, ref_code_id, status, content, ai_prompt)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProjectID, c.Name, c.Channel, c.RefCodeID, c.Status, c.Content, c.AIPrompt,
	)
	return err
}

func (s *SQLite) ListCampaigns(ctx context.Context, projectID string) ([]Campaign, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, channel, COALESCE(ref_code_id, ''), status, content, ai_prompt, created_at, updated_at
		 FROM campaigns WHERE project_id = ? ORDER BY created_at DESC`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var campaigns []Campaign
	for rows.Next() {
		var c Campaign
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Name, &c.Channel, &c.RefCodeID, &c.Status, &c.Content, &c.AIPrompt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, rows.Err()
}

func (s *SQLite) GetCampaign(ctx context.Context, projectID, id string) (*Campaign, error) {
	var c Campaign
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, channel, COALESCE(ref_code_id, ''), status, content, ai_prompt, created_at, updated_at
		 FROM campaigns WHERE project_id = ? AND id = ?`, projectID, id,
	).Scan(&c.ID, &c.ProjectID, &c.Name, &c.Channel, &c.RefCodeID, &c.Status, &c.Content, &c.AIPrompt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *SQLite) UpdateCampaign(ctx context.Context, projectID, id, name, status, content string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE campaigns SET name = ?, status = ?, content = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE project_id = ? AND id = ?`,
		name, status, content, projectID, id,
	)
	return err
}

func (s *SQLite) DeleteCampaign(ctx context.Context, projectID, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM campaigns WHERE project_id = ? AND id = ?`, projectID, id,
	)
	return err
}

// --- Campaign Posts ---

type CampaignPost struct {
	ID             string     `json:"id"`
	CampaignID     string     `json:"campaign_id"`
	ProjectID      string     `json:"project_id"`
	ConnectorName  string     `json:"connector_name"`
	ExternalID     string     `json:"external_id"`
	ExternalURL    string     `json:"external_url"`
	PostedAt       time.Time  `json:"posted_at"`
	LastEngagement string     `json:"last_engagement"`
	LastFetchedAt  *time.Time `json:"last_fetched_at,omitempty"`
}

func (s *SQLite) CreateCampaignPost(ctx context.Context, cp CampaignPost) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO campaign_posts (id, campaign_id, project_id, connector_name, external_id, external_url, last_engagement)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cp.ID, cp.CampaignID, cp.ProjectID, cp.ConnectorName, cp.ExternalID, cp.ExternalURL, cp.LastEngagement,
	)
	return err
}

func (s *SQLite) ListCampaignPosts(ctx context.Context, projectID string) ([]CampaignPost, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, campaign_id, project_id, connector_name, external_id, external_url, posted_at, last_engagement, last_fetched_at
		 FROM campaign_posts WHERE project_id = ? ORDER BY posted_at DESC`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []CampaignPost
	for rows.Next() {
		var cp CampaignPost
		if err := rows.Scan(&cp.ID, &cp.CampaignID, &cp.ProjectID, &cp.ConnectorName, &cp.ExternalID, &cp.ExternalURL, &cp.PostedAt, &cp.LastEngagement, &cp.LastFetchedAt); err != nil {
			return nil, err
		}
		posts = append(posts, cp)
	}
	return posts, rows.Err()
}

func (s *SQLite) UpdateCampaignPostEngagement(ctx context.Context, id, engagement string, fetchedAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE campaign_posts SET last_engagement = ?, last_fetched_at = ? WHERE id = ?`,
		engagement, fetchedAt, id,
	)
	return err
}

// --- Project Members ---

type ProjectMember struct {
	UserID    string    `json:"user_id"`
	ProjectID string    `json:"project_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *SQLite) AddProjectMember(ctx context.Context, userID, projectID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO project_members (user_id, project_id, role) VALUES (?, ?, ?)`,
		userID, projectID, role,
	)
	return err
}

func (s *SQLite) RemoveProjectMember(ctx context.Context, userID, projectID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM project_members WHERE user_id = ? AND project_id = ?`,
		userID, projectID,
	)
	return err
}

func (s *SQLite) ListProjectMembers(ctx context.Context, projectID string) ([]ProjectMember, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id, project_id, role, created_at FROM project_members WHERE project_id = ? ORDER BY created_at`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []ProjectMember
	for rows.Next() {
		var m ProjectMember
		if err := rows.Scan(&m.UserID, &m.ProjectID, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (s *SQLite) ListUserProjects(ctx context.Context, userID string) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.id, p.name, p.description, p.api_key, p.created_at
		 FROM projects p
		 JOIN project_members pm ON pm.project_id = p.id
		 WHERE pm.user_id = ?
		 ORDER BY p.created_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.APIKey, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (s *SQLite) GetUserProjectRole(ctx context.Context, userID, projectID string) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT role FROM project_members WHERE user_id = ? AND project_id = ?`,
		userID, projectID,
	).Scan(&role)
	return role, err
}

func (s *SQLite) SwitchSessionProject(ctx context.Context, token, projectID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_sessions SET project_id = ? WHERE token = ?`,
		projectID, token,
	)
	return err
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *SQLite) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *SQLite) CreateUser(ctx context.Context, email, passwordHash string) (*User, error) {
	id, err := generateRandomHex(16)
	if err != nil {
		return nil, err
	}
	u := &User{ID: id, Email: email, PasswordHash: passwordHash, CreatedAt: time.Now().UTC()}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		u.ID, u.Email, u.PasswordHash, u.CreatedAt)
	return u, err
}

func (s *SQLite) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	u := &User{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, created_at FROM users WHERE email = ?`, email).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *SQLite) CreateUserSession(ctx context.Context, userID string, expires time.Time, projectID string) (string, error) {
	token, err := generateRandomHex(32)
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO user_sessions (token, user_id, expires_at, created_at, project_id) VALUES (?, ?, ?, ?, ?)`,
		token, userID, expires, time.Now().UTC(), projectID)
	return token, err
}

// GetUserSession returns (userID, projectID, error). projectID may be empty for legacy sessions.
func (s *SQLite) GetUserSession(ctx context.Context, token string) (string, string, error) {
	var userID string
	var projectID sql.NullString
	var expiresAt time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, expires_at, project_id FROM user_sessions WHERE token = ?`, token).
		Scan(&userID, &expiresAt, &projectID)
	if err != nil {
		return "", "", err
	}
	if time.Now().UTC().After(expiresAt) {
		return "", "", sql.ErrNoRows
	}
	return userID, projectID.String, nil
}

func (s *SQLite) DeleteUserSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE token = ?`, token)
	return err
}

func generateRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func generateAPIKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating api key: %w", err)
	}
	return "cn_" + hex.EncodeToString(b), nil
}
