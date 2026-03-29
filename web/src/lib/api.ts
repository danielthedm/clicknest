import type { Event, TrendPoint, Session, EventName, Project, LLMConfig, GitHubConnection, UserProfile, Funnel, FunnelStep, FunnelResult, FunnelCohortResult, SuggestedFunnel, RetentionCohort, Dashboard, PageStat, TrendSeries, EventNameStat, ChatMessage, FeatureFlag, Alert, PathTransition, HeatmapPoint, AttributionSource, ChannelSummary, RefCode, ErrorGroup, SourceLink, ScoringRule, ScoredLead, CRMWebhook, Campaign, CampaignContent, ConnectorInfo, ICPAnalysis, ICPUserProfile, ABVariation, MeResponse } from './types';

const BASE = '/api/v1';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
	const controller = new AbortController();
	const timeout = setTimeout(() => controller.abort(), 10_000);
	try {
		const resp = await fetch(`${BASE}${path}`, {
			headers: { 'Content-Type': 'application/json' },
			signal: controller.signal,
			...options,
		});
		if (!resp.ok) {
			const body = await resp.text();
			throw new Error(`API error ${resp.status}: ${body}`);
		}
		return resp.json();
	} finally {
		clearTimeout(timeout);
	}
}

export async function getEvents(params?: Record<string, string>): Promise<{ events: Event[]; count: number }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/events${qs}`);
}

export function liveEvents(onEvent: (events: Event[]) => void): () => void {
	let source: EventSource | null = null;
	let timer: ReturnType<typeof setTimeout> | null = null;
	let stopped = false;

	function connect() {
		if (stopped) return;
		source = new EventSource(`${BASE}/events/live`);
		source.onmessage = (e) => {
			try {
				const events: Event[] = JSON.parse(e.data);
				onEvent(events);
			} catch {
				// ignore parse errors
			}
		};
		source.onerror = () => {
			// Close the broken connection and reconnect with backoff
			// to avoid exhausting Chrome's 6-connection-per-origin limit.
			source?.close();
			source = null;
			if (!stopped) {
				timer = setTimeout(connect, 5_000);
			}
		};
	}

	connect();

	return () => {
		stopped = true;
		if (timer) clearTimeout(timer);
		source?.close();
	};
}

export async function getTrends(params?: Record<string, string>): Promise<{ data: TrendPoint[]; interval: string }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/trends${qs}`);
}

export async function getSessions(params?: Record<string, string>): Promise<{ sessions: Session[]; total: number }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/sessions${qs}`);
}

export async function getSessionDetail(id: string): Promise<{ session_id: string; events: Event[]; count: number }> {
	return request(`/sessions/${id}`);
}

export async function getNames(): Promise<{ names: EventName[] }> {
	return request('/names');
}

export async function overrideName(fingerprint: string, name: string): Promise<void> {
	await request(`/names/${fingerprint}`, {
		method: 'PUT',
		body: JSON.stringify({ name }),
	});
}

export async function getProject(): Promise<Project> {
	return request('/project');
}

export async function updateProjectDescription(description: string): Promise<void> {
	await request('/project/description', {
		method: 'PUT',
		body: JSON.stringify({ description }),
	});
}

export async function getLLMConfig(): Promise<{ provider: string; model: string; base_url: string; api_key_set: boolean; api_key_hint: string }> {
	return request('/llm/config');
}

export async function updateLLMConfig(config: Partial<LLMConfig>): Promise<void> {
	await request('/llm/config', {
		method: 'PUT',
		body: JSON.stringify(config),
	});
}

export async function getGitHub(): Promise<GitHubConnection> {
	return request('/github');
}

export async function connectGitHub(params: {
	repo_owner: string;
	repo_name: string;
	access_token?: string;
	default_branch?: string;
}): Promise<void> {
	await request('/github', {
		method: 'PUT',
		body: JSON.stringify(params),
	});
}

export async function getGitHubOAuthURL(): Promise<{ url: string }> {
	return request('/github/oauth/authorize');
}

// Properties
export async function getPropertyKeys(): Promise<{ keys: string[] }> {
	return request('/properties/keys');
}

export async function getPropertyValues(key: string): Promise<{ values: string[] }> {
	return request(`/properties/values?key=${encodeURIComponent(key)}`);
}

// Users
export async function getUsers(params?: Record<string, string>): Promise<{ users: UserProfile[]; total: number }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/users${qs}`);
}

export async function getUserEvents(id: string, params?: Record<string, string>): Promise<{ events: Event[]; count: number }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/users/${encodeURIComponent(id)}/events${qs}`);
}

// Funnels
export async function listFunnels(): Promise<{ funnels: Funnel[] }> {
	return request('/funnels');
}

export async function createFunnel(name: string, steps: FunnelStep[]): Promise<Funnel> {
	return request('/funnels', {
		method: 'POST',
		body: JSON.stringify({ name, steps }),
	});
}

export async function getFunnel(id: string): Promise<Funnel> {
	return request(`/funnels/${id}`);
}

export async function deleteFunnel(id: string): Promise<void> {
	await request(`/funnels/${id}`, { method: 'DELETE' });
}

export async function getFunnelResults(id: string, params?: Record<string, string>): Promise<{ results: FunnelResult[] }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/funnels/${id}/results${qs}`);
}

export async function getFunnelCohorts(id: string, params?: Record<string, string>): Promise<{ cohorts: FunnelCohortResult[] }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/funnels/${id}/cohorts${qs}`);
}

export async function suggestFunnels(): Promise<{ suggestions: SuggestedFunnel[] }> {
	const controller = new AbortController();
	const timeout = setTimeout(() => controller.abort(), 30_000);
	try {
		const resp = await fetch(`${BASE}/funnels/suggest`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			signal: controller.signal,
		});
		if (!resp.ok) {
			const body = await resp.text();
			throw new Error(`API error ${resp.status}: ${body}`);
		}
		return resp.json();
	} finally {
		clearTimeout(timeout);
	}
}

export async function getPages(params?: Record<string, string>): Promise<{ pages: PageStat[] }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/pages${qs}`);
}

export async function getEventStats(params?: Record<string, string>): Promise<{ stats: EventNameStat[] }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/events/stats${qs}`);
}

export async function getTrendsBreakdown(params?: Record<string, string>): Promise<{ series: TrendSeries[]; interval: string; group_by: string }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/trends/breakdown${qs}`);
}

// AI chat
export async function aiChat(message: string, history: ChatMessage[]): Promise<{ reply: string }> {
	const controller = new AbortController();
	const timeout = setTimeout(() => controller.abort(), 60_000);
	try {
		const resp = await fetch(`${BASE}/ai/chat`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ message, history }),
			signal: controller.signal,
		});
		if (!resp.ok) {
			const body = await resp.text();
			throw new Error(`API error ${resp.status}: ${body}`);
		}
		return resp.json();
	} finally {
		clearTimeout(timeout);
	}
}

// Retention
export async function getRetention(params?: Record<string, string>): Promise<{ cohorts: RetentionCohort[] }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/retention${qs}`);
}

// Dashboards
export async function listDashboards(): Promise<{ dashboards: Dashboard[] }> {
	return request('/dashboards');
}

export async function createDashboard(name: string, config: Record<string, unknown>): Promise<Dashboard> {
	return request('/dashboards', {
		method: 'POST',
		body: JSON.stringify({ name, config }),
	});
}

export async function getDashboard(id: string): Promise<Dashboard> {
	return request(`/dashboards/${id}`);
}

export async function updateDashboard(id: string, name: string, config: Record<string, unknown>): Promise<void> {
	await request(`/dashboards/${id}`, {
		method: 'PUT',
		body: JSON.stringify({ name, config }),
	});
}

export async function deleteDashboard(id: string): Promise<void> {
	await request(`/dashboards/${id}`, { method: 'DELETE' });
}

// Errors
export async function getErrors(params?: Record<string, string>): Promise<{ groups: ErrorGroup[]; total_count: number }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/errors${qs}`);
}

export async function getErrorDetail(message: string, params?: Record<string, string>): Promise<{ events: Event[]; source_link: SourceLink | null }> {
	const p = new URLSearchParams({ message, ...params });
	return request(`/errors/detail?${p.toString()}`);
}

// Feature Flags
export async function listFlags(): Promise<{ flags: FeatureFlag[] }> {
	return request('/flags');
}

export async function createFlag(key: string, name: string, rolloutPercentage = 100): Promise<FeatureFlag> {
	return request('/flags', {
		method: 'POST',
		body: JSON.stringify({ key, name, rollout_percentage: rolloutPercentage }),
	});
}

export async function updateFlag(id: string, enabled: boolean, rolloutPercentage: number): Promise<void> {
	await request(`/flags/${id}`, {
		method: 'PUT',
		body: JSON.stringify({ enabled, rollout_percentage: rolloutPercentage }),
	});
}

export async function deleteFlag(id: string): Promise<void> {
	await request(`/flags/${id}`, { method: 'DELETE' });
}

// Alerts
export async function listAlerts(): Promise<{ alerts: Alert[] }> {
	return request('/alerts');
}

export async function createAlert(data: Omit<Alert, 'id' | 'project_id' | 'created_at' | 'last_triggered_at'>): Promise<Alert> {
	return request('/alerts', {
		method: 'POST',
		body: JSON.stringify(data),
	});
}

export async function updateAlert(id: string, enabled: boolean, threshold: number, webhookUrl: string): Promise<void> {
	await request(`/alerts/${id}`, {
		method: 'PUT',
		body: JSON.stringify({ enabled, threshold, webhook_url: webhookUrl }),
	});
}

export async function deleteAlert(id: string): Promise<void> {
	await request(`/alerts/${id}`, { method: 'DELETE' });
}

// Path analysis
export async function getPaths(params?: Record<string, string>): Promise<{ transitions: PathTransition[] }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/paths${qs}`);
}

// Heatmap
export async function getHeatmap(params?: Record<string, string>): Promise<{ points: HeatmapPoint[] }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/heatmap${qs}`);
}

// Backup / restore
export function exportBackupURL(): string {
	return `${BASE}/export`;
}

export async function importBackup(file: File): Promise<{ status: string; message: string }> {
	const form = new FormData();
	form.append('backup', file);
	const resp = await fetch(`${BASE}/import`, { method: 'POST', body: form });
	if (!resp.ok) {
		const body = await resp.text();
		throw new Error(`Import failed: ${body}`);
	}
	return resp.json();
}

export async function getStorage(): Promise<import('./types').StorageInfo> {
	return request('/storage');
}

// Attribution
export async function getAttribution(params?: Record<string, string>): Promise<{ channels: ChannelSummary[] }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/attribution${qs}`);
}

export async function getAttributionSources(params?: Record<string, string>): Promise<{ sources: AttributionSource[] }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/attribution/sources${qs}`);
}

// Ref Codes
export async function listRefCodes(): Promise<{ ref_codes: RefCode[] }> {
	return request('/refcodes');
}

export async function createRefCode(code: string, name: string, notes = ''): Promise<RefCode> {
	return request('/refcodes', {
		method: 'POST',
		body: JSON.stringify({ code, name, notes }),
	});
}

export async function updateRefCode(id: string, name: string, notes: string): Promise<void> {
	await request(`/refcodes/${id}`, {
		method: 'PUT',
		body: JSON.stringify({ name, notes }),
	});
}

export async function deleteRefCode(id: string): Promise<void> {
	await request(`/refcodes/${id}`, { method: 'DELETE' });
}

// Lead Scoring
export async function getLeads(params?: Record<string, string>): Promise<{ leads: ScoredLead[]; total: number }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/leads${qs}`);
}

// Scoring Rules
export async function listScoringRules(): Promise<{ rules: ScoringRule[] }> {
	return request('/scoring-rules');
}

export async function createScoringRule(data: { name: string; rule_type: string; config: string; points: number }): Promise<ScoringRule> {
	return request('/scoring-rules', { method: 'POST', body: JSON.stringify(data) });
}

export async function updateScoringRule(id: string, data: { name: string; rule_type: string; config: string; points: number; enabled: boolean }): Promise<void> {
	await request(`/scoring-rules/${id}`, { method: 'PUT', body: JSON.stringify(data) });
}

export async function deleteScoringRule(id: string): Promise<void> {
	await request(`/scoring-rules/${id}`, { method: 'DELETE' });
}

// CRM Webhooks
export async function listCRMWebhooks(): Promise<{ webhooks: CRMWebhook[] }> {
	return request('/crm-webhooks');
}

export async function createCRMWebhook(data: { name: string; webhook_url: string; min_score: number; payload_template?: string }): Promise<CRMWebhook> {
	return request('/crm-webhooks', { method: 'POST', body: JSON.stringify(data) });
}

export async function updateCRMWebhook(id: string, data: { name: string; webhook_url: string; min_score: number; enabled: boolean; payload_template?: string }): Promise<void> {
	await request(`/crm-webhooks/${id}`, { method: 'PUT', body: JSON.stringify(data) });
}

export async function getDeadLetters(): Promise<{ dead_letters: import('./types').DeadLetter[] }> {
	return request('/crm-webhooks/dead-letters');
}

export async function deleteCRMWebhook(id: string): Promise<void> {
	await request(`/crm-webhooks/${id}`, { method: 'DELETE' });
}

export async function testCRMWebhook(id: string): Promise<{ status: string; http_status: number }> {
	return request(`/crm-webhooks/${id}/test`, { method: 'POST' });
}

// Connectors
export async function listConnectors(): Promise<{ connectors: ConnectorInfo[] }> {
	return request('/connectors');
}

// Campaigns
export async function listCampaigns(): Promise<{
	campaigns: Campaign[];
	stats?: Record<string, { sessions: number; users: number; bounced: number; avg_pages: number }>;
}> {
	return request('/campaigns');
}

export async function createCampaign(data: { name: string; channel: string; content?: string }): Promise<Campaign> {
	return request('/campaigns', { method: 'POST', body: JSON.stringify(data) });
}

export async function getCampaign(id: string): Promise<Campaign> {
	return request(`/campaigns/${id}`);
}

export async function updateCampaign(id: string, data: { name: string; status: string; content: string }): Promise<void> {
	await request(`/campaigns/${id}`, { method: 'PUT', body: JSON.stringify(data) });
}

export async function deleteCampaign(id: string): Promise<void> {
	await request(`/campaigns/${id}`, { method: 'DELETE' });
}

export async function generateCampaign(channel: string, topic: string): Promise<{ campaign: Campaign; content: CampaignContent; ref_code: string }> {
	const controller = new AbortController();
	const timeout = setTimeout(() => controller.abort(), 60_000);
	try {
		const resp = await fetch(`${BASE}/campaigns/generate`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ channel, topic }),
			signal: controller.signal,
		});
		if (!resp.ok) {
			const body = await resp.text();
			throw new Error(`API error ${resp.status}: ${body}`);
		}
		return resp.json();
	} finally {
		clearTimeout(timeout);
	}
}

export async function createABTest(campaignId: string): Promise<{ variations: ABVariation[] }> {
	const controller = new AbortController();
	const timeout = setTimeout(() => controller.abort(), 60_000);
	try {
		const resp = await fetch(`${BASE}/campaigns/${campaignId}/ab-test`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			signal: controller.signal,
		});
		if (!resp.ok) {
			const body = await resp.text();
			throw new Error(`API error ${resp.status}: ${body}`);
		}
		return resp.json();
	} finally {
		clearTimeout(timeout);
	}
}

export async function getABResults(campaignId: string): Promise<{ variations: ABVariation[] }> {
	return request(`/campaigns/${campaignId}/ab-results`);
}

// ICP
// Multi-project
export async function getMe(): Promise<MeResponse> {
	return request('/auth/me');
}

export async function switchProject(projectId: string): Promise<void> {
	await request('/auth/project', {
		method: 'PUT',
		body: JSON.stringify({ project_id: projectId }),
	});
}

export async function listProjects(): Promise<{ projects: Project[] }> {
	return request('/projects');
}

export async function createNewProject(name: string): Promise<Project> {
	return request('/projects', {
		method: 'POST',
		body: JSON.stringify({ name }),
	});
}

export async function analyzeICP(conversionPaths: string[]): Promise<{ analysis: ICPAnalysis; profiles: ICPUserProfile[] }> {
	const controller = new AbortController();
	const timeout = setTimeout(() => controller.abort(), 60_000);
	try {
		const resp = await fetch(`${BASE}/icp/analyze`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ conversion_paths: conversionPaths }),
			signal: controller.signal,
		});
		if (!resp.ok) {
			const body = await resp.text();
			throw new Error(`API error ${resp.status}: ${body}`);
		}
		return resp.json();
	} finally {
		clearTimeout(timeout);
	}
}

// --- ICP History ---

export async function listICPAnalyses(): Promise<{ analyses: import('./types').SavedICPAnalysis[] }> {
	return request('/icp/analyses');
}

export async function getICPAnalysis(id: string): Promise<import('./types').SavedICPAnalysis> {
	return request(`/icp/analyses/${id}`);
}

export async function deleteICPAnalysis(id: string): Promise<void> {
	await request(`/icp/analyses/${id}`, { method: 'DELETE' });
}

// --- Campaign Performance ---

export async function getCampaignPerformance(
	id: string,
	params?: Record<string, string>,
): Promise<{
	campaign: import('./types').Campaign;
	ref_code: string;
	stats?: { sessions: number; users: number; bounced: number; avg_pages: number; event_count: number };
	time_series?: { date: string; sessions: number; users: number }[];
	posts?: import('./types').CampaignPost[];
	channels?: { channel: string; sessions: number; users: number }[];
	conversion_count?: number;
	conversion_rate?: number;
	conversion_event?: string;
}> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/campaigns/${id}/performance${qs}`);
}

// Billing usage (cloud only — gracefully returns null for self-hosted)
export async function getBillingUsage(): Promise<{
	tier: string;
	period_start: string;
	period_end: string;
	usage: { events: number; leads: number; campaigns: number; icp_analyses: number };
	limits: { free_events: number; free_leads: number; free_campaigns: number; free_icp: number };
} | null> {
	try {
		return await request('/billing/usage');
	} catch {
		return null;
	}
}

export async function createBillingPortal(returnURL: string): Promise<{ url: string }> {
	return request('/billing/portal', {
		method: 'POST',
		body: JSON.stringify({ return_url: returnURL }),
	});
}

export async function createCheckout(successURL: string, cancelURL: string): Promise<{ url: string }> {
	return request('/billing/checkout', {
		method: 'POST',
		body: JSON.stringify({ success_url: successURL, cancel_url: cancelURL }),
	});
}

// --- Mentions ---

export async function listMentions(
	params?: Record<string, string>,
): Promise<{ mentions: import('./types').MentionRecord[]; total: number }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/mentions${qs}`);
}

export async function getMention(id: string): Promise<import('./types').MentionRecord> {
	return request(`/mentions/${id}`);
}

export async function updateMention(id: string, data: { status: string }): Promise<void> {
	await request(`/mentions/${id}`, { method: 'PUT', body: JSON.stringify(data) });
}

export async function draftMentionReply(id: string): Promise<{ reply: string }> {
	const controller = new AbortController();
	const t = setTimeout(() => controller.abort(), 30_000);
	try {
		const resp = await fetch(`${BASE}/mentions/${id}/draft`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			signal: controller.signal,
		});
		if (!resp.ok) {
			const b = await resp.text();
			throw new Error(`API error ${resp.status}: ${b}`);
		}
		return resp.json();
	} finally {
		clearTimeout(t);
	}
}

export async function publishMentionReply(
	id: string,
	data: { publisher_name: string; reply_text?: string },
): Promise<void> {
	await request(`/mentions/${id}/reply`, { method: 'POST', body: JSON.stringify(data) });
}

// --- Sources ---

export async function listSources(): Promise<{ sources: import('./types').SourceInfo[] }> {
	return request('/sources');
}

export async function triggerSourceSearch(name: string): Promise<{ found: number }> {
	return request(`/sources/${name}/search`, { method: 'POST' });
}

export async function listSourceConfigs(): Promise<{
	configs: import('./types').SourceConfig[];
}> {
	return request('/source-configs');
}

export async function upsertSourceConfig(data: {
	source_name: string;
	keywords: string[];
	schedule_minutes: number;
	enabled: boolean;
}): Promise<void> {
	await request('/source-configs', { method: 'POST', body: JSON.stringify(data) });
}

// --- Source credentials / OAuth ---

export async function getSourceCredentials(name: string): Promise<import('./types').SourceCredentialStatus> {
	return request(`/sources/${name}/credentials`);
}

export async function saveSourceCredentials(name: string, data: { access_token?: string; refresh_token?: string }): Promise<{ connected: boolean; username: string }> {
	return request(`/sources/${name}/credentials`, { method: 'POST', body: JSON.stringify(data) });
}

export async function deleteSourceCredentials(name: string): Promise<{ ok: boolean }> {
	return request(`/sources/${name}/credentials`, { method: 'DELETE' });
}

export async function getSourceOAuthUrl(name: string): Promise<{ oauth_available: boolean; url?: string }> {
	return request(`/sources/${name}/oauth/authorize`);
}

// --- Campaign publish / engagement ---

export async function publishCampaign(
	id: string,
	data: { publisher_name: string; content_override?: string },
): Promise<{ external_id: string; external_url: string }> {
	return request(`/campaigns/${id}/publish`, { method: 'POST', body: JSON.stringify(data) });
}

export async function refreshCampaignEngagement(id: string): Promise<{ refreshed: number }> {
	return request(`/campaigns/${id}/refresh-engagement`, { method: 'POST' });
}

// --- Webhook deliveries ---

export async function listWebhookDeliveries(
	webhookId: string,
): Promise<{ deliveries: import('./types').WebhookDelivery[] }> {
	return request(`/crm-webhooks/${webhookId}/deliveries`);
}

export async function retryWebhookDelivery(
	webhookId: string,
	deliveryId: string,
): Promise<{ success: boolean; status_code: number }> {
	return request(`/crm-webhooks/${webhookId}/deliveries/${deliveryId}/retry`, { method: 'POST' });
}

// --- ICP → actions ---

export async function icpGenerateCampaign(
	analysisId: string,
	channel: string,
): Promise<{ campaign: import('./types').Campaign; content: import('./types').CampaignContent; ref_code: string }> {
	const controller = new AbortController();
	const timeout = setTimeout(() => controller.abort(), 60_000);
	try {
		const resp = await fetch(`/api/v1/icp/analyses/${analysisId}/generate-campaign`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ channel }),
			signal: controller.signal,
		});
		if (!resp.ok) {
			const b = await resp.text();
			throw new Error(`API error ${resp.status}: ${b}`);
		}
		return resp.json();
	} finally {
		clearTimeout(timeout);
	}
}

export async function icpCreateScoringRules(analysisId: string): Promise<{ created: number }> {
	return request(`/icp/analyses/${analysisId}/create-scoring-rules`, { method: 'POST' });
}

// --- Lead score history + attribution ---

export async function getLeadScoreHistory(distinctId: string): Promise<{ history: import('./types').LeadScoreSnapshot[] }> {
	return request(`/leads/${encodeURIComponent(distinctId)}/score-history`);
}

export async function getLeadAttribution(distinctId: string): Promise<{ sources: import('./types').LeadAttribution[] }> {
	return request(`/leads/${encodeURIComponent(distinctId)}/attribution`);
}

// --- Segments ---

export async function listSegments(): Promise<{ segments: import('./types').Segment[] }> {
	return request('/segments');
}

export async function createSegment(data: { name: string; conditions: string }): Promise<import('./types').Segment> {
	return request('/segments', { method: 'POST', body: JSON.stringify(data) });
}

export async function deleteSegment(id: string): Promise<void> {
	await request(`/segments/${id}`, { method: 'DELETE' });
}

export async function getSegmentMembers(id: string): Promise<{ members: import('./types').ScoredLead[]; total: number }> {
	return request(`/segments/${id}/members`);
}

// --- ICP settings ---

export async function getICPSettings(): Promise<{ icp_auto_refresh: boolean }> {
	return request('/icp/settings');
}

export async function putICPSettings(settings: { icp_auto_refresh: boolean }): Promise<void> {
	await request('/icp/settings', { method: 'PUT', body: JSON.stringify(settings) });
}

// --- Conversion Goals ---

export async function listConversionGoals(): Promise<{ goals: import('./types').ConversionGoal[] }> {
	return request('/conversion-goals');
}

export async function createConversionGoal(data: { name: string; event_type: string; event_name: string; url_pattern: string; value_property: string }): Promise<import('./types').ConversionGoal> {
	return request('/conversion-goals', { method: 'POST', body: JSON.stringify(data) });
}

export async function getConversionGoal(id: string): Promise<import('./types').ConversionGoal> {
	return request(`/conversion-goals/${id}`);
}

export async function updateConversionGoal(id: string, data: { name: string; event_type: string; event_name: string; url_pattern: string; value_property: string }): Promise<void> {
	await request(`/conversion-goals/${id}`, { method: 'PUT', body: JSON.stringify(data) });
}

export async function deleteConversionGoal(id: string): Promise<void> {
	await request(`/conversion-goals/${id}`, { method: 'DELETE' });
}

export async function getConversionGoalResults(id: string, params?: Record<string, string>): Promise<{ goal: import('./types').ConversionGoal; model: string; attributions: import('./types').ConversionAttribution[]; total_conversions: number; total_revenue: number }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/conversion-goals/${id}/results${qs}`);
}

export async function getRevenueAttribution(params?: Record<string, string>): Promise<{ total_conversions: number; total_revenue: number; by_channel: import('./types').ConversionAttribution[] }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/attribution/revenue${qs}`);
}

// --- Experiments ---

export async function listExperiments(): Promise<{ experiments: import('./types').Experiment[] }> {
	return request('/experiments');
}

export async function createExperiment(data: { name: string; flag_key: string; variants: string[]; conversion_goal_id?: string; auto_stop?: boolean }): Promise<import('./types').Experiment> {
	return request('/experiments', { method: 'POST', body: JSON.stringify(data) });
}

export async function getExperiment(id: string): Promise<import('./types').Experiment> {
	return request(`/experiments/${id}`);
}

export async function updateExperiment(id: string, data: { name: string; status: string; auto_stop: boolean; conversion_goal_id: string }): Promise<void> {
	await request(`/experiments/${id}`, { method: 'PUT', body: JSON.stringify(data) });
}

export async function deleteExperiment(id: string): Promise<void> {
	await request(`/experiments/${id}`, { method: 'DELETE' });
}

export async function getExperimentResults(id: string): Promise<import('./types').ExperimentResults> {
	return request(`/experiments/${id}/results`);
}

export async function getExperimentSampleSize(id: string): Promise<{ sample_size_needed: number; current_max_exposures: number; remaining: number }> {
	return request(`/experiments/${id}/sample-size`);
}

export async function stopExperiment(id: string): Promise<void> {
	await request(`/experiments/${id}/stop`, { method: 'POST' });
}

export async function declareWinner(id: string, variant: string): Promise<void> {
	await request(`/experiments/${id}/declare-winner`, { method: 'POST', body: JSON.stringify({ variant }) });
}
