<script lang="ts">
	import { onMount } from 'svelte';
	import { getPaths } from '$lib/api';
	import type { PathTransition } from '$lib/types';

	let transitions = $state<PathTransition[]>([]);
	let loading = $state(true);
	let range = $state('7d');
	let filterFrom = $state<string | null>(null);

	onMount(() => loadPaths());

	function getDateRange() {
		const end = new Date();
		const days: Record<string, number> = { '7d': 7, '30d': 30, '90d': 90 };
		const d = days[range] ?? 7;
		return { start: new Date(end.getTime() - d * 86400000), end };
	}

	async function loadPaths() {
		loading = true;
		filterFrom = null;
		try {
			const { start, end } = getDateRange();
			const res = await getPaths({
				start: start.toISOString(),
				end: end.toISOString(),
				limit: '50',
			});
			transitions = res.transitions ?? [];
		} catch (e) {
			console.error('Failed to load paths:', e);
		}
		loading = false;
	}

	let displayed = $derived(() => {
		if (filterFrom) return transitions.filter(t => t.from === filterFrom);
		return transitions;
	});

	let maxCount = $derived(displayed().length > 0 ? displayed()[0].count : 1);
</script>

<div class="p-6 max-w-4xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Path Analysis</h2>
			<p class="text-sm text-muted-foreground mt-1">Page transition flows — where do users go next?</p>
		</div>
		<div class="flex gap-1">
			{#each [['7d', '7D'], ['30d', '30D'], ['90d', '90D']] as [value, label]}
				<button
					onclick={() => { range = value; loadPaths(); }}
					class="px-2 py-1 text-xs rounded border transition-colors {range === value
						? 'bg-primary text-primary-foreground border-primary'
						: 'border-border hover:bg-accent'}"
				>{label}</button>
			{/each}
		</div>
	</div>

	{#if filterFrom}
		<div class="flex items-center gap-2 mb-4 p-2 bg-primary/10 rounded-md text-sm">
			<span class="text-muted-foreground">Showing outbound from</span>
			<span class="font-mono font-medium text-primary">{filterFrom}</span>
			<button onclick={() => filterFrom = null} class="ml-auto text-xs hover:underline text-muted-foreground">Clear filter</button>
		</div>
	{/if}

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
		</div>
	{:else if transitions.length === 0}
		<div class="border border-border rounded-lg p-12 bg-card text-center">
			<p class="text-muted-foreground">No path transitions found in this period.</p>
			<p class="text-xs text-muted-foreground mt-1">Paths are built from pageview events with session tracking.</p>
		</div>
	{:else}
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<div class="px-4 py-2 border-b border-border bg-muted/50 grid grid-cols-[1fr_auto_1fr_100px] gap-4 text-xs font-medium text-muted-foreground">
				<span>From</span>
				<span></span>
				<span>To</span>
				<span class="text-right">Sessions</span>
			</div>
			<div class="divide-y divide-border">
				{#each displayed() as t}
					<div class="px-4 py-2.5 grid grid-cols-[1fr_auto_1fr_100px] gap-4 items-center hover:bg-accent/30 transition-colors text-sm">
						<button
							onclick={() => filterFrom = filterFrom === t.from ? null : t.from}
							class="font-mono text-xs text-left hover:text-primary transition-colors truncate"
						>{t.from}</button>
						<svg class="w-4 h-4 text-muted-foreground shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 8l4 4m0 0l-4 4m4-4H3" />
						</svg>
						<span class="font-mono text-xs truncate text-muted-foreground">{t.to}</span>
						<div class="flex items-center gap-2 justify-end">
							<div class="h-1.5 rounded-full bg-primary/20 flex-1 max-w-16">
								<div
									class="h-1.5 rounded-full bg-primary"
									style="width: {Math.round((t.count / maxCount) * 100)}%"
								></div>
							</div>
							<span class="tabular-nums text-xs font-medium w-8 text-right">{t.count}</span>
						</div>
					</div>
				{/each}
			</div>
		</div>
		{#if filterFrom}
			<p class="text-xs text-muted-foreground mt-2 text-center">
				{displayed().length} destination{displayed().length !== 1 ? 's' : ''} from <span class="font-mono">{filterFrom}</span>
			</p>
		{:else}
			<p class="text-xs text-muted-foreground mt-2 text-center">Top {transitions.length} transitions · click a "From" path to filter</p>
		{/if}
	{/if}
</div>
