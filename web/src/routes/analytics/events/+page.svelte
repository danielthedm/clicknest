<script lang="ts">
	import { onMount } from 'svelte';
	import { getEvents, liveEvents, getPropertyKeys, getPropertyValues, createDashboard, getEventStats } from '$lib/api';
	import { eventDisplayName, relativeTime, formatTime } from '$lib/utils';
	import { exportCSV } from '$lib/csv';
	import type { Event, EventNameStat } from '$lib/types';
	import Chart from '$lib/components/ui/Chart.svelte';
	import { EVENT_TYPE_COLORS, type ChartConfiguration } from '$lib/chart-config';

	let activeTab = $state<'stream' | 'stats'>('stream');

	// Stream state
	let events = $state<Event[]>([]);
	let loading = $state(true);
	let filter = $state({ event_type: '', limit: '50', property_key: '', property_value: '' });
	let liveMode = $state(false);
	let cleanup: (() => void) | null = null;
	let propertyKeys = $state<string[]>([]);
	let propertyValues = $state<string[]>([]);
	let expandedRow = $state<string | null>(null);

	// Save view state
	let showSave = $state(false);
	let saveName = $state('');
	let saving = $state(false);

	// Stats state
	let stats = $state<EventNameStat[]>([]);
	let statsLoading = $state(false);
	let statsRange = $state('7d');

	onMount(() => {
		loadEvents();
		loadPropertyKeys();
		return () => cleanup?.();
	});

	async function loadPropertyKeys() {
		try {
			const res = await getPropertyKeys();
			propertyKeys = res.keys ?? [];
		} catch { /* ignore */ }
	}

	async function onPropertyKeyChange() {
		propertyValues = [];
		filter.property_value = '';
		if (filter.property_key) {
			try {
				const res = await getPropertyValues(filter.property_key);
				propertyValues = res.values ?? [];
			} catch { /* ignore */ }
		}
		loadEvents();
	}

	async function saveView() {
		if (!saveName.trim()) return;
		saving = true;
		try {
			const config: Record<string, unknown> = { view: 'events', filters: {} };
			const filters: Record<string, string> = {};
			if (filter.event_type) filters.event_type = filter.event_type;
			if (filter.property_key) filters.property_key = filter.property_key;
			if (filter.property_value) filters.property_value = filter.property_value;
			config.filters = filters;
			await createDashboard(saveName.trim(), config);
			showSave = false;
			saveName = '';
		} catch (e) {
			console.error('Failed to save view:', e);
		}
		saving = false;
	}

	async function loadEvents() {
		loading = true;
		try {
			const params: Record<string, string> = { limit: filter.limit };
			if (filter.event_type) params.event_type = filter.event_type;
			if (filter.property_key) params.property_key = filter.property_key;
			if (filter.property_value) params.property_value = filter.property_value;
			const res = await getEvents(params);
			events = res.events ?? [];
		} catch (e) {
			console.error('Failed to load events:', e);
		}
		loading = false;
	}

	async function loadStats() {
		statsLoading = true;
		try {
			const end = new Date();
			let start: Date;
			switch (statsRange) {
				case '7d': start = new Date(end.getTime() - 7 * 24 * 60 * 60 * 1000); break;
				case '30d': start = new Date(end.getTime() - 30 * 24 * 60 * 60 * 1000); break;
				case '90d': start = new Date(end.getTime() - 90 * 24 * 60 * 60 * 1000); break;
				default: start = new Date(end.getTime() - 7 * 24 * 60 * 60 * 1000);
			}
			const res = await getEventStats({
				start: start.toISOString(),
				end: end.toISOString(),
				limit: '50',
			});
			stats = res.stats ?? [];
		} catch (e) {
			console.error('Failed to load stats:', e);
		}
		statsLoading = false;
	}

	function switchTab(tab: 'stream' | 'stats') {
		activeTab = tab;
		if (tab === 'stats' && stats.length === 0) {
			loadStats();
		}
	}

	function toggleLive() {
		if (liveMode) {
			cleanup?.();
			cleanup = null;
			liveMode = false;
		} else {
			cleanup = liveEvents((newEvents) => {
				events = [...newEvents, ...events].slice(0, 200);
			});
			liveMode = true;
		}
	}

	const typeOrder = ['click', 'pageview', 'submit', 'input', 'custom'] as const;

	let typeCounts = $derived(() => {
		const counts: Record<string, number> = {};
		for (const e of events) {
			counts[e.event_type] = (counts[e.event_type] || 0) + 1;
		}
		return counts;
	});

	let distributionConfig = $derived(() => {
		const counts = typeCounts();
		const types = typeOrder.filter(t => counts[t]);
		return {
			type: 'bar' as const,
			data: {
				labels: [''],
				datasets: types.map(t => ({
					label: t.charAt(0).toUpperCase() + t.slice(1),
					data: [counts[t]],
					backgroundColor: EVENT_TYPE_COLORS[t] || EVENT_TYPE_COLORS.custom,
				})),
			},
			options: {
				responsive: true,
				maintainAspectRatio: false,
				indexAxis: 'y' as const,
				animation: false as const,
				plugins: {
					legend: { display: false },
					tooltip: {
						backgroundColor: 'hsl(224 71% 4% / 0.9)',
						titleFont: { size: 11 },
						bodyFont: { size: 11 },
						padding: 8,
						cornerRadius: 4,
						callbacks: {
							label: (ctx: any) => {
								const total = events.length;
								const pct = total > 0 ? Math.round((ctx.parsed.x / total) * 100) : 0;
								return `${ctx.dataset.label}: ${ctx.parsed.x} (${pct}%)`;
							},
						},
					},
				},
				scales: {
					x: { stacked: true, display: false },
					y: { stacked: true, display: false },
				},
			},
		} as ChartConfiguration;
	});

	let maxStatCount = $derived(stats.length > 0 ? stats[0].count : 1);
</script>

<div class="p-6 max-w-6xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Events</h2>
			<p class="text-sm text-muted-foreground mt-1">All captured analytics events</p>
		</div>
		<div class="flex gap-2">
			{#if activeTab === 'stream'}
				<button
					onclick={() => exportCSV(events as any, 'events.csv')}
					disabled={events.length === 0}
					class="px-3 py-1.5 text-sm rounded-md border border-border hover:bg-accent disabled:opacity-40 transition-colors"
				>Export CSV</button>
				<button
					onclick={toggleLive}
					class="px-3 py-1.5 text-sm rounded-md border transition-colors {liveMode
						? 'bg-green-500 text-white border-green-500'
						: 'border-border hover:bg-accent'}"
				>
					{#if liveMode}
						<span class="inline-block w-2 h-2 rounded-full bg-white mr-1.5 animate-pulse"></span>
					{/if}
					{liveMode ? 'Live' : 'Go Live'}
				</button>
			{:else}
				<button
					onclick={() => exportCSV(stats as any, 'event-stats.csv')}
					disabled={stats.length === 0}
					class="px-3 py-1.5 text-sm rounded-md border border-border hover:bg-accent disabled:opacity-40 transition-colors"
				>Export CSV</button>
			{/if}
		</div>
	</div>

	<!-- Tabs -->
	<div class="flex gap-1 mb-6 border-b border-border">
		{#each [['stream', 'Event Stream'], ['stats', 'Top Events']] as [tab, label]}
			<button
				onclick={() => switchTab(tab as 'stream' | 'stats')}
				class="px-4 py-2 text-sm font-medium border-b-2 transition-colors -mb-px {activeTab === tab
					? 'border-primary text-foreground'
					: 'border-transparent text-muted-foreground hover:text-foreground'}"
			>
				{label}
			</button>
		{/each}
	</div>

	{#if activeTab === 'stream'}
		<!-- Event type distribution bar -->
		{#if events.length > 0}
			<div class="mb-4">
				<div class="flex items-center gap-3 mb-2">
					{#each typeOrder.filter(t => typeCounts()[t]) as t}
						<div class="flex items-center gap-1.5 text-xs text-muted-foreground">
							<span class="inline-block w-2.5 h-2.5 rounded-full" style="background: {EVENT_TYPE_COLORS[t]}"></span>
							{t.charAt(0).toUpperCase() + t.slice(1)} ({typeCounts()[t]})
						</div>
					{/each}
				</div>
				<Chart config={distributionConfig()} class="h-6" />
			</div>
		{/if}

		<!-- Filters + Save View -->
		<div class="flex gap-2 mb-4 flex-wrap items-center">
			<select
				bind:value={filter.event_type}
				onchange={loadEvents}
				class="px-3 py-1.5 text-sm border border-border rounded-md bg-background"
			>
				<option value="">All types</option>
				<option value="click">Click</option>
				<option value="pageview">Pageview</option>
				<option value="submit">Submit</option>
				<option value="input">Input</option>
				<option value="custom">Custom</option>
			</select>
			{#if propertyKeys.length > 0}
				<select
					bind:value={filter.property_key}
					onchange={onPropertyKeyChange}
					class="px-3 py-1.5 text-sm border border-border rounded-md bg-background"
				>
					<option value="">All properties</option>
					{#each propertyKeys as key}
						<option value={key}>{key}</option>
					{/each}
				</select>
				{#if filter.property_key && propertyValues.length > 0}
					<select
						bind:value={filter.property_value}
						onchange={loadEvents}
						class="px-3 py-1.5 text-sm border border-border rounded-md bg-background"
					>
						<option value="">All values</option>
						{#each propertyValues as val}
							<option value={val}>{val}</option>
						{/each}
					</select>
				{/if}
			{/if}

			<div class="ml-auto flex items-center gap-2">
				{#if showSave}
					<input
						bind:value={saveName}
						placeholder="Dashboard name..."
						class="px-3 py-1.5 text-sm border border-border rounded-md bg-background w-44"
						onkeydown={(e) => { if (e.key === 'Enter') saveView(); if (e.key === 'Escape') { showSave = false; saveName = ''; } }}
					/>
					<button
						onclick={saveView}
						disabled={saving || !saveName.trim()}
						class="px-3 py-1.5 text-sm rounded-md bg-primary text-primary-foreground disabled:opacity-50"
					>
						{saving ? 'Saving...' : 'Save'}
					</button>
					<button
						onclick={() => { showSave = false; saveName = ''; }}
						class="px-3 py-1.5 text-sm border border-border rounded-md hover:bg-accent"
					>
						Cancel
					</button>
				{:else}
					<button
						onclick={() => showSave = true}
						class="px-3 py-1.5 text-sm border border-border rounded-md hover:bg-accent"
					>
						Save View
					</button>
				{/if}
			</div>
		</div>

		<!-- Events table -->
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b border-border bg-muted/50">
						<th class="px-4 py-2.5 text-left font-medium text-muted-foreground">Type</th>
						<th class="px-4 py-2.5 text-left font-medium text-muted-foreground">Name</th>
						<th class="px-4 py-2.5 text-left font-medium text-muted-foreground">Page</th>
						<th class="px-4 py-2.5 text-left font-medium text-muted-foreground">Session</th>
						<th class="px-4 py-2.5 text-left font-medium text-muted-foreground">Time</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-border">
					{#if loading}
						<tr><td colspan="5" class="px-4 py-8 text-center text-muted-foreground">Loading...</td></tr>
					{:else if events.length === 0}
						<tr><td colspan="5" class="px-4 py-8 text-center text-muted-foreground">No events found</td></tr>
					{:else}
						{#each events as event}
							<tr
								class="hover:bg-accent/50 transition-colors {event.properties && Object.keys(event.properties).length > 0 ? 'cursor-pointer' : ''}"
								onclick={() => { if (event.properties && Object.keys(event.properties).length > 0) expandedRow = expandedRow === event.id ? null : event.id; }}
							>
								<td class="px-4 py-2.5">
									<span class="inline-flex items-center px-2 py-0.5 rounded text-[10px] font-medium uppercase tracking-wider
										{event.event_type === 'click' ? 'bg-blue-100 text-blue-700' :
										 event.event_type === 'pageview' ? 'bg-green-100 text-green-700' :
										 event.event_type === 'submit' ? 'bg-purple-100 text-purple-700' :
										 'bg-gray-100 text-gray-700'}">
										{event.event_type}
									</span>
								</td>
								<td class="px-4 py-2.5 max-w-xs truncate {event.event_name ? 'font-medium' : 'text-muted-foreground'}">
									{eventDisplayName(event)}
								</td>
								<td class="px-4 py-2.5 text-muted-foreground truncate max-w-[200px]">{event.url_path}</td>
								<td class="px-4 py-2.5">
									<a href="/sessions?id={event.session_id}" class="text-xs text-primary hover:underline font-mono">
										{event.session_id.substring(0, 8)}
									</a>
								</td>
								<td class="px-4 py-2.5 text-xs text-muted-foreground whitespace-nowrap" title={formatTime(event.timestamp)}>
									{relativeTime(event.timestamp)}
									{#if event.properties && Object.keys(event.properties).length > 0}
										<span class="ml-1 text-primary">{expandedRow === event.id ? '▾' : '▸'}</span>
									{/if}
								</td>
							</tr>
							{#if expandedRow === event.id && event.properties}
								<tr class="bg-muted/30">
									<td colspan="5" class="px-4 py-3">
										<pre class="text-xs font-mono text-muted-foreground whitespace-pre-wrap">{JSON.stringify(event.properties, null, 2)}</pre>
									</td>
								</tr>
							{/if}
						{/each}
					{/if}
				</tbody>
			</table>
		</div>

	{:else}
		<!-- Stats tab: Top named events -->
		<div class="flex items-center justify-between mb-4">
			<p class="text-sm text-muted-foreground">Most-fired named events in the selected period</p>
			<div class="flex gap-1">
				{#each [['7d', '7D'], ['30d', '30D'], ['90d', '90D']] as [value, label]}
					<button
						onclick={() => { statsRange = value; loadStats(); }}
						class="px-3 py-1.5 text-sm rounded-md border transition-colors {statsRange === value
							? 'bg-primary text-primary-foreground border-primary'
							: 'border-border hover:bg-accent'}"
					>
						{label}
					</button>
				{/each}
			</div>
		</div>

		{#if statsLoading}
			<div class="border border-border rounded-lg bg-card p-12 text-center text-muted-foreground text-sm">Loading...</div>
		{:else if stats.length === 0}
			<div class="border border-border rounded-lg bg-card p-12 text-center">
				<p class="text-muted-foreground text-sm">No named events found in this period.</p>
				<p class="text-xs text-muted-foreground mt-1">Events get names via AI naming or the SDK's <code class="font-mono">track()</code> call.</p>
			</div>
		{:else}
			<div class="border border-border rounded-lg bg-card overflow-hidden">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border-border bg-muted/50">
							<th class="px-4 py-2.5 text-left font-medium text-muted-foreground w-8">#</th>
							<th class="px-4 py-2.5 text-left font-medium text-muted-foreground">Event Name</th>
							<th class="px-4 py-2.5 text-left font-medium text-muted-foreground">Volume</th>
							<th class="px-4 py-2.5 text-right font-medium text-muted-foreground">Count</th>
							<th class="px-4 py-2.5 text-right font-medium text-muted-foreground">Last Fired</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-border">
						{#each stats as stat, i}
							{@const pct = Math.round((stat.count / maxStatCount) * 100)}
							{@const totalCount = stats.reduce((s, x) => s + x.count, 0)}
							{@const sharePct = totalCount > 0 ? ((stat.count / totalCount) * 100).toFixed(1) : '0'}
							<tr class="hover:bg-accent/50 transition-colors">
								<td class="px-4 py-3 text-xs text-muted-foreground">{i + 1}</td>
								<td class="px-4 py-3 font-medium">{stat.name}</td>
								<td class="px-4 py-3">
									<div class="flex items-center gap-2">
										<div class="flex-1 bg-muted rounded-full h-1.5 min-w-[80px]">
											<div
												class="bg-primary rounded-full h-1.5 transition-all"
												style="width: {pct}%"
											></div>
										</div>
										<span class="text-xs text-muted-foreground w-10 text-right">{sharePct}%</span>
									</div>
								</td>
								<td class="px-4 py-3 text-right font-mono text-sm">{stat.count.toLocaleString()}</td>
								<td class="px-4 py-3 text-right text-xs text-muted-foreground">{relativeTime(stat.last_seen)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
</div>
