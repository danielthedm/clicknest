import type { Event, TrendPoint, Session, EventName, Project, LLMConfig, GitHubConnection, UserProfile, Funnel, FunnelStep, FunnelResult, FunnelCohortResult, SuggestedFunnel, RetentionCohort, Dashboard, PageStat, TrendSeries, EventNameStat, ChatMessage, FeatureFlag, Alert, PathTransition, HeatmapPoint } from './types';

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
export async function getErrors(params?: Record<string, string>): Promise<{ errors: Event[]; count: number }> {
	const qs = params ? '?' + new URLSearchParams(params).toString() : '';
	return request(`/errors${qs}`);
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
