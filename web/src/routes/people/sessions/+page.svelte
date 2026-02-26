<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { getSessions, getSessionDetail } from '$lib/api';
	import { eventDisplayName, formatTime, relativeTime } from '$lib/utils';
	import { exportCSV } from '$lib/csv';
	import type { Session, Event } from '$lib/types';
	import Chart from '$lib/components/ui/Chart.svelte';
	import { getCssColor, baseBarOptions, type ChartConfiguration } from '$lib/chart-config';

	let sessions = $state<Session[]>([]);
	let selectedSession = $state<string | null>(null);
	let sessionEvents = $state<Event[]>([]);
	let loading = $state(true);
	let loadingDetail = $state(false);
	let range = $state('7d');
	let search = $state('');

	onMount(async () => {
		// Check if a session ID was passed via URL ?id=...
		const id = $page.url.searchParams.get('id');
		await loadSessions();
		if (id) {
			selectedSession = id;
			await loadSessionDetail(id);
		}
	});

	async function loadSessions() {
		loading = true;
		try {
			const end = new Date();
			let start: Date;
			switch (range) {
				case '7d': start = new Date(end.getTime() - 7 * 24 * 60 * 60 * 1000); break;
				case '30d': start = new Date(end.getTime() - 30 * 24 * 60 * 60 * 1000); break;
				case '90d': start = new Date(end.getTime() - 90 * 24 * 60 * 60 * 1000); break;
				default: start = new Date(end.getTime() - 7 * 24 * 60 * 60 * 1000);
			}
			const res = await getSessions({
				limit: '200',
				start: start.toISOString(),
				end: end.toISOString(),
			});
			sessions = res.sessions ?? [];
		} catch (e) {
			console.error('Failed to load sessions:', e);
		}
		loading = false;
	}

	async function loadSessionDetail(id: string) {
		loadingDetail = true;
		try {
			const res = await getSessionDetail(id);
			sessionEvents = res.events ?? [];
		} catch (e) {
			console.error('Failed to load session detail:', e);
		}
		loadingDetail = false;
	}

	async function selectSession(id: string) {
		selectedSession = id;
		await loadSessionDetail(id);
	}

	function duration(first: string, last: string): string {
		const diff = new Date(last).getTime() - new Date(first).getTime();
		const seconds = Math.floor(diff / 1000);
		if (seconds < 60) return `${seconds}s`;
		const minutes = Math.floor(seconds / 60);
		if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
		const hours = Math.floor(minutes / 60);
		return `${hours}h ${minutes % 60}m`;
	}

	function durationMs(first: string, last: string): number {
		return new Date(last).getTime() - new Date(first).getTime();
	}

	let filteredSessions = $derived(() => {
		const q = search.trim().toLowerCase();
		if (!q) return sessions;
		return sessions.filter(s =>
			s.session_id.toLowerCase().includes(q) ||
			(s.distinct_id && s.distinct_id.toLowerCase().includes(q))
		);
	});

	let histogramData = $derived(() => {
		const buckets = [0, 0, 0, 0, 0]; // <1m, 1-5m, 5-15m, 15-30m, 30m+
		for (const s of sessions) {
			const ms = durationMs(s.first_seen, s.last_seen);
			const min = ms / 60000;
			if (min < 1) buckets[0]++;
			else if (min < 5) buckets[1]++;
			else if (min < 15) buckets[2]++;
			else if (min < 30) buckets[3]++;
			else buckets[4]++;
		}
		return buckets;
	});

	let histogramConfig = $derived<ChartConfiguration>({
		type: 'bar',
		data: {
			labels: ['< 1m', '1–5m', '5–15m', '15–30m', '30m+'],
			datasets: [{
				data: histogramData(),
				backgroundColor: getCssColor('primary', 0.7),
				hoverBackgroundColor: getCssColor('primary'),
				borderRadius: 3,
			}],
		},
		options: {
			...baseBarOptions(),
			plugins: {
				...baseBarOptions().plugins,
				tooltip: {
					...baseBarOptions().plugins?.tooltip,
					callbacks: {
						label: (ctx) => `${ctx.parsed.y} sessions`,
					},
				},
			},
		},
	});

	// Compute event type breakdown for selected session
	let sessionEventTypes = $derived(() => {
		if (!sessionEvents.length) return {};
		const counts: Record<string, number> = {};
		for (const e of sessionEvents) {
			counts[e.event_type] = (counts[e.event_type] || 0) + 1;
		}
		return counts;
	});
</script>

<div class="p-6 max-w-6xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Sessions</h2>
			<p class="text-sm text-muted-foreground mt-1">User session timelines</p>
		</div>
		<div class="flex gap-2 items-center">
			<button
				onclick={() => exportCSV(sessions as any, 'sessions.csv')}
				disabled={sessions.length === 0}
				class="px-2 py-1 text-xs rounded border border-border hover:bg-accent disabled:opacity-40 transition-colors"
			>Export CSV</button>
			<div class="flex gap-1">
				{#each [['7d', '7D'], ['30d', '30D'], ['90d', '90D']] as [value, label]}
					<button
						onclick={() => { range = value; loadSessions(); }}
						class="px-3 py-1.5 text-sm rounded-md border transition-colors {range === value
							? 'bg-primary text-primary-foreground border-primary'
							: 'border-border hover:bg-accent'}"
					>
						{label}
					</button>
				{/each}
			</div>
		</div>
	</div>

	<!-- Duration histogram -->
	{#if sessions.length >= 3}
		<div class="border border-border rounded-lg p-4 bg-card mb-6">
			<h3 class="text-sm font-medium mb-3">Session Duration Distribution</h3>
			<Chart config={histogramConfig} class="h-40" />
		</div>
	{/if}

	<div class="grid grid-cols-[1fr_1.5fr] gap-4">
		<!-- Session list -->
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<div class="px-4 py-3 border-b border-border flex items-center gap-2">
				<h3 class="text-sm font-medium">Sessions ({filteredSessions().length}{filteredSessions().length !== sessions.length ? `/${sessions.length}` : ''})</h3>
				<input
					bind:value={search}
					placeholder="Search by ID or user..."
					class="ml-auto px-2 py-1 text-xs border border-border rounded bg-background w-40"
				/>
			</div>
			{#if loading}
				<div class="p-8 text-center text-muted-foreground text-sm">Loading...</div>
			{:else if sessions.length === 0}
				<div class="p-8 text-center text-muted-foreground text-sm">No sessions found</div>
			{:else if filteredSessions().length === 0}
				<div class="p-6 text-center text-muted-foreground text-sm">No sessions match "{search}"</div>
			{:else}
				<div class="divide-y divide-border max-h-[600px] overflow-y-auto">
					{#each filteredSessions() as session}
						<button
							onclick={() => selectSession(session.session_id)}
							class="w-full px-4 py-3 text-left hover:bg-accent/50 transition-colors text-sm {selectedSession === session.session_id ? 'bg-accent' : ''}"
						>
							<div class="flex items-center justify-between">
								<span class="font-mono text-xs text-primary">{session.session_id.substring(0, 12)}</span>
								<span class="text-[10px] text-muted-foreground">{relativeTime(session.first_seen)}</span>
							</div>
							<div class="flex items-center gap-3 mt-1.5">
								<span class="text-xs text-muted-foreground">{session.event_count} events</span>
								<span class="text-xs text-muted-foreground">{duration(session.first_seen, session.last_seen)}</span>
							</div>
							{#if session.distinct_id}
								<div class="text-xs text-muted-foreground mt-1 truncate">
									<span class="text-foreground font-medium">{session.distinct_id}</span>
								</div>
							{/if}
						</button>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Session detail -->
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<div class="px-4 py-3 border-b border-border">
				<h3 class="text-sm font-medium">
					{#if selectedSession}
						Session Timeline
					{:else}
						Select a session
					{/if}
				</h3>
			</div>
			{#if !selectedSession}
				<div class="p-8 text-center text-muted-foreground text-sm">Click a session to view its event timeline</div>
			{:else if loadingDetail}
				<div class="p-8 text-center text-muted-foreground text-sm">Loading...</div>
			{:else}
				<!-- Event type summary pills -->
				{#if Object.keys(sessionEventTypes()).length > 0}
					<div class="px-4 py-2 flex gap-2 flex-wrap border-b border-border bg-muted/30">
						{#each Object.entries(sessionEventTypes()).sort((a, b) => b[1] - a[1]) as [type, count]}
							<span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-medium
								{type === 'click' ? 'bg-blue-100 text-blue-700' :
								 type === 'pageview' ? 'bg-green-100 text-green-700' :
								 type === 'submit' ? 'bg-purple-100 text-purple-700' :
								 'bg-gray-100 text-gray-700'}">
								{type} × {count}
							</span>
						{/each}
					</div>
				{/if}
				<div class="divide-y divide-border max-h-[560px] overflow-y-auto">
					{#each sessionEvents as event, i}
						<div class="px-4 py-3 flex gap-3">
							<!-- Timeline dot -->
							<div class="flex flex-col items-center">
								<div class="w-2.5 h-2.5 rounded-full mt-1 flex-shrink-0 {event.event_type === 'click' ? 'bg-blue-500' :
									event.event_type === 'pageview' ? 'bg-green-500' :
									event.event_type === 'submit' ? 'bg-purple-500' :
									'bg-gray-400'}"></div>
								{#if i < sessionEvents.length - 1}
									<div class="w-px flex-1 bg-border mt-1"></div>
								{/if}
							</div>
							<div class="flex-1 min-w-0">
								<div class="flex items-center gap-2">
									<span class="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">{event.event_type}</span>
									<span class="text-xs text-muted-foreground">{formatTime(event.timestamp)}</span>
								</div>
								<p class="text-sm mt-0.5 {event.event_name ? 'font-medium' : 'text-muted-foreground'}">
									{eventDisplayName(event)}
								</p>
								<p class="text-xs text-muted-foreground mt-0.5 truncate">{event.url_path}</p>
								{#if event.properties && Object.keys(event.properties).length > 0}
									<details class="mt-1">
										<summary class="text-xs text-primary cursor-pointer">Properties</summary>
										<pre class="text-xs font-mono text-muted-foreground mt-1 whitespace-pre-wrap">{JSON.stringify(event.properties, null, 2)}</pre>
									</details>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>
</div>
