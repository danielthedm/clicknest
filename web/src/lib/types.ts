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

export interface StorageInfo {
	events_bytes: number;
	meta_bytes: number;
	total_bytes: number;
	volume_bytes: number;
	free_bytes: number;
}
