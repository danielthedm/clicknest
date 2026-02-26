<script lang="ts">
	import { onMount } from 'svelte';
	import { getErrors } from '$lib/api';
	import { formatTime, relativeTime } from '$lib/utils';
	import type { Event } from '$lib/types';

	let errors = $state<Event[]>([]);
	let loading = $state(true);
	let range = $state('7d');
	let expandedMessage = $state<string | null>(null);

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
				limit: '500',
			});
			errors = res.errors ?? [];
		} catch (e) {
			console.error('Failed to load errors:', e);
		}
		loading = false;
	}

	// Group errors by message.
	let grouped = $derived(() => {
		const map = new Map<string, { message: string; events: Event[]; firstSeen: string; lastSeen: string }>();
		for (const e of errors) {
			const msg = String(e.properties?.message ?? 'Unknown error');
			if (!map.has(msg)) {
				map.set(msg, { message: msg, events: [], firstSeen: e.timestamp, lastSeen: e.timestamp });
			}
			const g = map.get(msg)!;
			g.events.push(e);
			if (e.timestamp < g.firstSeen) g.firstSeen = e.timestamp;
			if (e.timestamp > g.lastSeen) g.lastSeen = e.timestamp;
		}
		return Array.from(map.values()).sort((a, b) => b.events.length - a.events.length);
	});

	function toggleExpand(msg: string) {
		expandedMessage = expandedMessage === msg ? null : msg;
	}
</script>

<div class="p-6 max-w-5xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Errors</h2>
			<p class="text-sm text-muted-foreground mt-1">JavaScript errors and unhandled rejections</p>
		</div>
		<div class="flex gap-1">
			{#each [['7d', '7D'], ['30d', '30D'], ['90d', '90D']] as [value, label]}
				<button
					onclick={() => { range = value; loadErrors(); }}
					class="px-2 py-1 text-xs rounded border transition-colors {range === value
						? 'bg-primary text-primary-foreground border-primary'
						: 'border-border hover:bg-accent'}"
				>{label}</button>
			{/each}
		</div>
	</div>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
		</div>
	{:else if errors.length === 0}
		<div class="border border-border rounded-lg p-12 bg-card text-center">
			<svg class="w-12 h-12 mx-auto text-muted-foreground mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
			</svg>
			<p class="text-muted-foreground font-medium">No errors captured in this period</p>
			<p class="text-xs text-muted-foreground mt-2">Errors are captured automatically when the SDK is loaded.</p>
			<pre class="mt-4 text-left text-xs bg-muted rounded p-3 inline-block">{'<script src="/sdk.js"\n  data-api-key="YOUR_KEY"\n  data-host="https://your-host">\n</' + 'script>'}</pre>
		</div>
	{:else}
		<div class="text-sm text-muted-foreground mb-3">{errors.length} occurrences · {grouped().length} unique errors</div>
		<div class="space-y-2">
			{#each grouped() as group}
				<div class="border border-border rounded-lg bg-card overflow-hidden">
					<button
						onclick={() => toggleExpand(group.message)}
						class="w-full flex items-start gap-3 p-4 text-left hover:bg-accent/50 transition-colors"
					>
						<div class="flex-1 min-w-0">
							<p class="font-mono text-sm font-medium text-destructive truncate">{group.message}</p>
							<p class="text-xs text-muted-foreground mt-1">
								{group.events.length} occurrence{group.events.length !== 1 ? 's' : ''} ·
								first {relativeTime(group.firstSeen)} ·
								last {relativeTime(group.lastSeen)}
							</p>
						</div>
						<div class="flex items-center gap-2 shrink-0">
							<span class="text-sm font-bold tabular-nums">{group.events.length}</span>
							<svg class="w-4 h-4 text-muted-foreground transition-transform {expandedMessage === group.message ? 'rotate-180' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
							</svg>
						</div>
					</button>

					{#if expandedMessage === group.message}
						<div class="border-t border-border divide-y divide-border">
							{#each group.events.slice(0, 10) as e}
								<div class="p-4 text-xs space-y-1">
									<div class="flex items-center gap-2 text-muted-foreground">
										<span>{formatTime(e.timestamp)}</span>
										<span>·</span>
										<span class="font-mono">{e.url_path}</span>
										{#if e.properties?.source}
											<span>·</span>
											<span class="font-mono">{e.properties.source}:{e.properties.lineno}</span>
										{/if}
									</div>
									{#if e.properties?.stack}
										<pre class="font-mono text-xs bg-muted rounded p-2 overflow-x-auto whitespace-pre-wrap max-h-40">{e.properties.stack}</pre>
									{/if}
								</div>
							{/each}
							{#if group.events.length > 10}
								<div class="p-3 text-xs text-center text-muted-foreground">
									+{group.events.length - 10} more occurrences
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
