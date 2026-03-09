<script lang="ts">
	import { onMount } from 'svelte';
	import { getErrors, getErrorDetail } from '$lib/api';
	import { formatTime, relativeTime } from '$lib/utils';
	import type { ErrorGroup, Event, SourceLink } from '$lib/types';
	import AiInsight from '$lib/components/ui/AiInsight.svelte';
	import Sparkline from '$lib/components/ui/Sparkline.svelte';

	let groups = $state<ErrorGroup[]>([]);
	let totalCount = $state(0);
	let loading = $state(true);
	let range = $state('7d');

	let expandedMessage = $state<string | null>(null);
	let detailEvents = $state<Event[]>([]);
	let detailSourceLink = $state<SourceLink | null>(null);
	let detailLoading = $state(false);

	const typeColors: Record<string, string> = {
		TypeError: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
		ReferenceError: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
		SyntaxError: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400',
		RangeError: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
		UnhandledRejection: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
		Error: 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400',
	};

	onMount(() => loadErrors());

	function getDateRange() {
		const end = new Date();
		const days: Record<string, number> = { '7d': 7, '30d': 30, '90d': 90 };
		const d = days[range] ?? 7;
		return { start: new Date(end.getTime() - d * 86400000), end };
	}

	async function loadErrors() {
		loading = true;
		try {
			const { start, end } = getDateRange();
			const res = await getErrors({
				start: start.toISOString(),
				end: end.toISOString(),
			});
			groups = res.groups ?? [];
			totalCount = res.total_count ?? 0;
		} catch (e) {
			console.error('Failed to load errors:', e);
		}
		loading = false;
	}

	async function toggleExpand(msg: string) {
		if (expandedMessage === msg) {
			expandedMessage = null;
			return;
		}
		expandedMessage = msg;
		detailLoading = true;
		detailEvents = [];
		detailSourceLink = null;
		try {
			const { start, end } = getDateRange();
			const res = await getErrorDetail(msg, {
				start: start.toISOString(),
				end: end.toISOString(),
				limit: '20',
			});
			detailEvents = res.events ?? [];
			detailSourceLink = res.source_link ?? null;
		} catch (e) {
			console.error('Failed to load error detail:', e);
		}
		detailLoading = false;
	}

	let errorsPrompt = $derived(() => {
		if (groups.length === 0) return '';
		const summary = groups.slice(0, 10).map(g =>
			`"${g.message}" (${g.error_type}) — ${g.count} occurrences, ${g.users} users, ${g.sessions} sessions`
		).join('\n');
		return `Analyze these JavaScript errors from the last ${range}. Prioritize by impact, identify patterns, and suggest fixes. Be brief — 3-4 sentences.\n\n${totalCount} total occurrences, ${groups.length} unique errors:\n${summary}`;
	});

	let errorsReady = $derived(!loading && groups.length > 0);

	function getTypeClass(errorType: string): string {
		return typeColors[errorType] || typeColors.Error;
	}

	function exportCSV() {
		if (groups.length === 0) return;
		const header = 'Error Type,Message,Count,Users,Sessions,First Seen,Last Seen';
		const rows = groups.map(g =>
			[g.error_type, `"${g.message.replace(/"/g, '""')}"`, g.count, g.users, g.sessions, g.first_seen, g.last_seen].join(',')
		);
		const csv = [header, ...rows].join('\n');
		const blob = new Blob([csv], { type: 'text/csv' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `errors-${range}.csv`;
		a.click();
		URL.revokeObjectURL(url);
	}
</script>

<div class="p-6 max-w-6xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Errors</h2>
			<p class="text-sm text-muted-foreground mt-1">JavaScript errors and unhandled rejections</p>
		</div>
		<div class="flex items-center gap-2">
			<button
				onclick={exportCSV}
				disabled={groups.length === 0}
				class="px-2 py-1 text-xs rounded border border-border hover:bg-accent transition-colors disabled:opacity-50"
			>Export</button>
			<div class="flex gap-1">
				{#each [['7d', '7D'], ['30d', '30D'], ['90d', '90D']] as [value, label]}
					<button
						onclick={() => { range = value; expandedMessage = null; loadErrors(); }}
						class="px-2 py-1 text-xs rounded border transition-colors {range === value
							? 'bg-primary text-primary-foreground border-primary'
							: 'border-border hover:bg-accent'}"
					>{label}</button>
				{/each}
			</div>
		</div>
	</div>

	<AiInsight cacheKey="errors_{range}" prompt={errorsPrompt()} ready={errorsReady} />

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
		</div>
	{:else if groups.length === 0}
		<div class="border border-border rounded-lg p-12 bg-card text-center">
			<svg class="w-12 h-12 mx-auto text-muted-foreground mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
			</svg>
			<p class="text-muted-foreground font-medium">No errors captured in this period</p>
			<p class="text-xs text-muted-foreground mt-2">Errors are captured automatically when the SDK is loaded.</p>
			<pre class="mt-4 text-left text-xs bg-muted rounded p-3 inline-block">{'<script src="/sdk.js"\n  data-api-key="YOUR_KEY"\n  data-host="https://your-host">\n</' + 'script>'}</pre>
		</div>
	{:else}
		<div class="text-sm text-muted-foreground mb-3">{totalCount} total occurrences across {groups.length} error groups</div>

		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<!-- Header -->
			<div class="grid grid-cols-[1fr_80px_60px_60px_120px_100px] gap-2 px-4 py-2 border-b border-border text-xs font-medium text-muted-foreground">
				<span>Error</span>
				<span class="text-right">Count</span>
				<span class="text-right">Users</span>
				<span class="text-right">Sessions</span>
				<span class="text-center">Trend</span>
				<span class="text-right">Last seen</span>
			</div>

			{#each groups as group}
				<div class="border-b border-border last:border-b-0">
					<button
						onclick={() => toggleExpand(group.message)}
						class="w-full grid grid-cols-[1fr_80px_60px_60px_120px_100px] gap-2 items-center px-4 py-3 text-left hover:bg-accent/50 transition-colors"
					>
						<div class="flex items-center gap-2 min-w-0">
							<span class="px-1.5 py-0.5 text-[10px] font-semibold rounded shrink-0 {getTypeClass(group.error_type)}">{group.error_type}</span>
							<span class="font-mono text-sm truncate">{group.message}</span>
							<svg class="w-3.5 h-3.5 text-muted-foreground shrink-0 transition-transform {expandedMessage === group.message ? 'rotate-180' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
							</svg>
						</div>
						<span class="text-sm font-bold tabular-nums text-right">{group.count}</span>
						<span class="text-xs tabular-nums text-right text-muted-foreground">{group.users}</span>
						<span class="text-xs tabular-nums text-right text-muted-foreground">{group.sessions}</span>
						<div class="flex justify-center">
							{#if group.sparkline && group.sparkline.length > 1}
								<Sparkline data={group.sparkline} width={100} height={24} color={group.error_type === 'TypeError' ? '#ef4444' : group.error_type === 'ReferenceError' ? '#f97316' : '#6366f1'} />
							{/if}
						</div>
						<span class="text-xs text-muted-foreground text-right">{relativeTime(group.last_seen)}</span>
					</button>

					{#if expandedMessage === group.message}
						<div class="border-t border-border bg-muted/30">
							{#if detailSourceLink}
								<div class="px-4 py-2 border-b border-border flex items-center gap-2">
									<svg class="w-4 h-4 text-muted-foreground shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
									</svg>
									<a
										href={detailSourceLink.github_url}
										target="_blank"
										rel="noopener noreferrer"
										class="text-xs font-mono text-primary hover:underline"
									>{detailSourceLink.file_path}{detailSourceLink.line > 0 ? `:${detailSourceLink.line}` : ''}</a>
									<span class="text-[10px] text-muted-foreground">on GitHub</span>
								</div>
							{/if}

							{#if detailLoading}
								<div class="flex items-center justify-center py-8">
									<div class="animate-spin rounded-full h-5 w-5 border-b-2 border-primary"></div>
								</div>
							{:else}
								<div class="divide-y divide-border">
									{#each detailEvents as e}
										<div class="px-4 py-3 text-xs space-y-1.5">
											<div class="flex items-center gap-2 flex-wrap text-muted-foreground">
												<span>{formatTime(e.timestamp)}</span>
												<span class="text-border">|</span>
												<span class="font-mono">{e.url_path}</span>
												{#if e.session_id}
													<span class="text-border">|</span>
													<a href="/analytics/sessions?id={e.session_id}" class="text-primary hover:underline">Session</a>
												{/if}
												{#if e.distinct_id}
													<span class="text-border">|</span>
													<a href="/analytics/users?id={e.distinct_id}" class="text-primary hover:underline">User: {e.distinct_id}</a>
												{/if}
												{#if e.properties?.source}
													<span class="text-border">|</span>
													<span class="font-mono">{e.properties.source}:{e.properties.lineno}</span>
												{/if}
											</div>
											{#if e.properties?.stack}
												<pre class="font-mono text-xs bg-muted rounded p-2 overflow-x-auto whitespace-pre-wrap max-h-40">{e.properties.stack}</pre>
											{/if}
										</div>
									{/each}
									{#if detailEvents.length === 0}
										<div class="px-4 py-6 text-center text-xs text-muted-foreground">No events found</div>
									{/if}
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
