export interface Event {
	id: string;
	project_id: string;
	session_id: string;
	distinct_id?: string;
	event_type: string;
	fingerprint: string;
	event_name?: string;
	element_tag?: string;
	element_id?: string;
	element_classes?: string;
	element_text?: string;
	aria_label?: string;
	data_attributes?: Record<string, string>;
	parent_path?: string;
	url: string;
	url_path: string;
	page_title?: string;
	referrer?: string;
	screen_width?: number;
	screen_height?: number;
	user_agent?: string;
	timestamp: string;
	received_at: string;
	properties?: Record<string, unknown>;
}

export interface TrendPoint {
	bucket: string;
	count: number;
}

export interface Session {
	session_id: string;
	distinct_id?: string;
	event_count: number;
	first_seen: string;
	last_seen: string;
	entry_url: string;
}

export interface EventName {
	fingerprint: string;
	project_id: string;
	ai_name: string;
	user_name?: string;
	source_file?: string;
	confidence?: number;
	created_at: string;
}

export interface Project {
	id: string;
	name: string;
	description: string;
	api_key: string;
	created_at: string;
}

export interface LLMConfig {
	project_id: string;
	provider: string;
	api_key?: string;
	model: string;
	base_url?: string;
}

export interface GitHubConnection {
	connected: boolean;
	repo_owner?: string;
	repo_name?: string;
	default_branch?: string;
	last_synced_at?: string;
	oauth_enabled?: boolean;
}

export interface UserProfile {
	distinct_id: string;
	event_count: number;
	first_seen: string;
	last_seen: string;
}

export interface Funnel {
	id: string;
	project_id: string;
	name: string;
	steps: string;
	created_at: string;
}

export interface FunnelStep {
	event_type: string;
	event_name: string;
}

export interface FunnelResult {
	step: string;
	count: number;
}

export interface FunnelCohortStep {
	step: string;
	count: number;
}

export interface FunnelCohortResult {
	cohort: string;
	steps: FunnelCohortStep[];
}

export interface SuggestedFunnel {
	name: string;
	description: string;
	steps: FunnelStep[];
}

export interface RetentionCohort {
	cohort: string;
	size: number;
	retention: number[];
}

export interface PageStat {
	path: string;
	title: string;
	views: number;
	sessions: number;
}

export interface TrendSeries {
	name: string;
	data: TrendPoint[];
}

export interface EventNameStat {
	name: string;
	count: number;
	last_seen: string;
}

export interface ChatMessage {
	role: 'user' | 'assistant';
	content: string;
}

export interface Dashboard {
	id: string;
	project_id: string;
	name: string;
	config: string;
	created_at: string;
	updated_at: string;
}

export interface FeatureFlag {
	id: string;
	project_id: string;
	key: string;
	name: string;
	enabled: boolean;
	rollout_percentage: number;
	created_at: string;
	updated_at: string;
}

export interface Alert {
	id: string;
	project_id: string;
	name: string;
	metric: string;
	event_name?: string;
	threshold: number;
	window_minutes: number;
	webhook_url: string;
	enabled: boolean;
	last_triggered_at?: string;
	created_at: string;
}

export interface PathTransition {
	from: string;
	to: string;
	count: number;
}

export interface HeatmapPoint {
	x: number;
	y: number;
	count: number;
}

export interface ErrorGroup {
	message: string;
	error_type: string;
	count: number;
	users: number;
	sessions: number;
	first_seen: string;
	last_seen: string;
	sample_id: string;
	sparkline: TrendPoint[];
}

export interface SourceLink {
	file_path: string;
	github_url: string;
	line: number;
}

export interface StorageInfo {
	events_bytes: number;
	meta_bytes: number;
	total_bytes: number;
	volume_bytes: number;
	free_bytes: number;
}

export interface AttributionSource {
	source: string;
	channel: string;
	sessions: number;
	users: number;
	bounced: number;
	avg_pages: number;
}

export interface ChannelSummary {
	channel: string;
	sessions: number;
	users: number;
	bounced: number;
}

export interface RefCode {
	id: string;
	project_id: string;
	code: string;
	name: string;
	notes: string;
	created_at: string;
	updated_at: string;
}

export interface ScoringRule {
	id: string;
	project_id: string;
	name: string;
	rule_type: string;
	config: string;
	points: number;
	enabled: boolean;
	created_at: string;
	updated_at: string;
}

export interface ScoredLead {
	distinct_id: string;
	score: number;
	raw_score: number;
	score_delta?: number; // change vs yesterday's snapshot; undefined if no snapshot
	days_since_last_seen: number;
	event_count: number;
	session_count: number;
	page_views: number;
	first_seen: string;
	last_seen: string;
	top_pages?: string[];
	properties?: Record<string, unknown>;
}

export interface LeadScoreSnapshot {
	id: string;
	project_id: string;
	distinct_id: string;
	score: number;
	raw_score: number;
	snapshot_date: string;
	created_at: string;
}

export interface LeadAttribution {
	source: string;
	channel: string;
	campaign?: string;
	sessions: number;
	first_touch: string;
	last_touch: string;
}

export interface Segment {
	id: string;
	project_id: string;
	name: string;
	conditions: string; // JSON
	created_at: string;
	updated_at: string;
}

export interface CRMWebhook {
	id: string;
	project_id: string;
	name: string;
	webhook_url: string;
	min_score: number;
	enabled: boolean;
	payload_template?: string;
	last_pushed_at?: string;
	created_at: string;
	updated_at: string;
}

export interface DeadLetter {
	id: string;
	webhook_id: string;
	webhook_name: string;
	project_id: string;
	lead_count: number;
	status_code: number;
	response_body: string;
	error: string;
	success: boolean;
	attempt: number;
	created_at: string;
}

export interface Campaign {
	id: string;
	project_id: string;
	name: string;
	channel: string;
	ref_code_id?: string;
	status: string;
	content: string;
	ai_prompt: string;
	cost: number;
	created_at: string;
	updated_at: string;
}

export interface CampaignPost {
	id: string;
	campaign_id: string;
	project_id: string;
	connector_name: string;
	external_id: string;
	external_url: string;
	posted_at: string;
	last_engagement: string;
	last_fetched_at?: string;
}

export interface CampaignContent {
	title: string;
	body: string;
	url?: string;
	tags?: string[];
}

export interface ConnectorInfo {
	name: string;
	display_name: string;
}

export interface ICPAnalysis {
	summary: string;
	common_traits: string[];
	best_channels: string[];
	recommendations: string[];
}

export interface ICPUserProfile {
	distinct_id: string;
	session_count: number;
	event_count: number;
	top_pages: string[];
	entry_source: string;
}

export interface SavedICPAnalysis {
	id: string;
	project_id: string;
	conversion_pages: string;
	summary: string;
	traits: string;
	channels: string;
	recommendations: string;
	profile_count: number;
	created_at: string;
}

export interface ABVariation {
	flag_key: string;
	content: string;
	impressions: number;
	conversions: number;
	conversion_rate: number;
}

export interface ProjectMember {
	user_id: string;
	project_id: string;
	role: string;
	created_at: string;
}

export interface MeResponse {
	user_id: string;
	active_project?: { id: string; name: string };
	projects: { id: string; name: string }[];
}

export interface MentionRecord {
	id: string;
	project_id: string;
	source_name: string;
	external_id: string;
	external_url: string;
	author: string;
	title: string;
	content: string;
	relevance_score: number;
	status: 'new' | 'reviewed' | 'replied' | 'dismissed' | 'lead';
	suggested_reply: string;
	parent_id: string;
	metadata: string;
	posted_at?: string;
	created_at: string;
	updated_at: string;
}

export interface SourceInfo {
	name: string;
	display_name: string;
}

export interface SourceCredentialStatus {
	connected: boolean;
	username?: string;
	connected_at?: string;
}

export interface SourceConfig {
	id: string;
	project_id: string;
	source_name: string;
	keywords: string;
	filters: string;
	schedule_minutes: number;
	enabled: boolean;
	last_run_at?: string;
	created_at: string;
	updated_at: string;
}

export interface WebhookDelivery {
	id: string;
	webhook_id: string;
	project_id: string;
	lead_count: number;
	status_code: number;
	response_body: string;
	error: string;
	success: boolean;
	attempt: number;
	created_at: string;
}

export interface ConversionGoal {
	id: string;
	project_id: string;
	name: string;
	event_type: string;
	event_name: string;
	url_pattern: string;
	value_property: string;
	created_at: string;
	updated_at: string;
}

export interface ConversionAttribution {
	source: string;
	channel: string;
	campaign: string;
	conversions: number;
	revenue: number;
	users: number;
}

export interface Experiment {
	id: string;
	project_id: string;
	name: string;
	flag_key: string;
	variants: string;
	conversion_goal_id?: string;
	status: string;
	auto_stop: boolean;
	started_at: string;
	ended_at?: string;
	winner_variant?: string;
	created_at: string;
	updated_at: string;
}

export interface ExperimentVariantResult {
	variant: string;
	exposures: number;
	conversions: number;
	conversion_rate: number;
	revenue: number;
	confidence_low: number;
	confidence_high: number;
}

export interface ExperimentResults {
	experiment: Experiment;
	variants: ExperimentVariantResult[];
	significance?: {
		z_score: number;
		p_value: number;
		significant: boolean;
	};
	chi_squared?: {
		chi_squared: number;
		p_value: number;
		significant: boolean;
	};
	sample_size_needed: number;
	winner?: string;
}
