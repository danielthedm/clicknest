<script lang="ts">
	import { onMount } from 'svelte';
	import { getPages, getTrends } from '$lib/api';
	import { exportCSV } from '$lib/csv';
	import type { PageStat, TrendPoint } from '$lib/types';
	import Chart from '$lib/components/ui/Chart.svelte';
	import { getCssColor, baseBarOptions, type ChartConfiguration } from '$lib/chart-config';

	let pages = $state<PageStat[]>([]);
	let loading = $state(true);
	let range = $state('30d');
	let sortBy = $state<'views' | 'sessions'>('views');

	onMount(() => loadPages());

	function getDateRange() {
		const end = new Date();
		const ms: Record<string, number> = { '7d': 7, '30d': 30, '90d': 90 };
		const days = ms[range] ?? 30;
		return { start: new Date(end.getTime() - days * 86400000), end };
	}

	async function loadPages() {
		loading = true;
		try {
			const { start, end } = getDateRange();
			const res = await getPages({
				start: start.toISOString(),
				end: end.toISOString(),
				limit: '100',
			});
			pages = res.pages ?? [];
		} catch (e) {
			console.error('Failed to load pages:', e);
		}
		loading = false;
	}

	let sorted = $derived([...pages].sort((a, b) => b[sortBy] - a[sortBy]));

	let totalViews = $derived(pages.reduce((s, p) => s + p.views, 0));
	let totalSessions = $derived(pages.reduce((s, p) => s + p.sessions, 0));

	let barConfig = $derived<ChartConfiguration>({
		type: 'bar',
		data: {
			labels: sorted.slice(0, 10).map(p => p.path.length > 30 ? '…' + p.path.slice(-28) : p.path),
			datasets: [{
				data: sorted.slice(0, 10).map(p => p[sortBy]),
				backgroundColor: getCssColor('primary', 0.7),
				borderColor: getCssColor('primary'),
				borderWidth: 1,
				borderRadius: 4,
			}],
		},
		options: {
			...baseBarOptions(),
			indexAxis: 'y' as const,
			plugins: {
				...baseBarOptions().plugins,
				tooltip: {
					...baseBarOptions().plugins?.tooltip,
					callbacks: {
						label: (ctx: any) => `${ctx.parsed.x.toLocaleString()} ${sortBy}`,
					},
				},
			},
			scales: {
				x: { beginAtZero: true, grid: { display: false }, border: { display: false }, ticks: { font: { size: 10 }, color: getCssColor('muted-foreground', 0.7), precision: 0 } },
				y: { grid: { display: false }, border: { display: false }, ticks: { font: { size: 10 }, color: getCssColor('muted-foreground', 0.8) } },
			},
		},
	});
</script>

<div class="p-6 max-w-6xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Pages</h2>
			<p class="text-sm text-muted-foreground mt-1">URL path traffic analysis</p>
		</div>
		<div class="flex gap-2 items-center">
			<button
				onclick={() => exportCSV(pages as any, 'pages.csv')}
				disabled={pages.length === 0}
				class="px-2 py-1 text-xs rounded border border-border hover:bg-accent disabled:opacity-40 transition-colors"
			>Export CSV</button>
			<div class="flex gap-1">
				{#each [['7d', '7D'], ['30d', '30D'], ['90d', '90D']] as [value, label]}
					<button
						onclick={() => { range = value; loadPages(); }}
						class="px-2 py-1 text-xs rounded border transition-colors {range === value
							? 'bg-primary text-primary-foreground border-primary'
							: 'border-border hover:bg-accent'}"
					>{label}</button>
				{/each}
			</div>
		</div>
	</div>

	<!-- Stats row -->
	<div class="grid grid-cols-2 gap-4 mb-6">
		<div class="border border-border rounded-lg p-4 bg-card">
			<p class="text-sm text-muted-foreground">Total Pageviews</p>
			<p class="text-3xl font-bold mt-1">{totalViews.toLocaleString()}</p>
		</div>
		<div class="border border-border rounded-lg p-4 bg-card">
			<p class="text-sm text-muted-foreground">Unique Pages</p>
			<p class="text-3xl font-bold mt-1">{pages.length.toLocaleString()}</p>
		</div>
	</div>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
		</div>
	{:else if pages.length === 0}
		<div class="border border-border rounded-lg p-12 bg-card text-center">
			<p class="text-muted-foreground text-sm">No pageviews recorded in this period.</p>
		</div>
	{:else}
		<!-- Bar chart: top 10 -->
		<div class="border border-border rounded-lg p-4 bg-card mb-6">
			<div class="flex items-center justify-between mb-3">
				<h3 class="text-sm font-medium">Top 10 Pages</h3>
				<div class="flex gap-1">
					{#each [['views', 'Views'], ['sessions', 'Sessions']] as [value, label]}
						<button
							onclick={() => sortBy = value as 'views' | 'sessions'}
							class="px-2 py-0.5 text-xs rounded border transition-colors {sortBy === value
								? 'bg-primary text-primary-foreground border-primary'
								: 'border-border hover:bg-accent'}"
						>{label}</button>
					{/each}
				</div>
			</div>
			<Chart config={barConfig} class="h-64" />
		</div>

		<!-- Full table -->
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<div class="px-4 py-3 border-b border-border flex items-center justify-between">
				<h3 class="text-sm font-medium">All Pages ({pages.length})</h3>
				<div class="flex gap-1">
					{#each [['views', 'Views'], ['sessions', 'Sessions']] as [value, label]}
						<button
							onclick={() => sortBy = value as 'views' | 'sessions'}
							class="px-2 py-0.5 text-xs rounded border transition-colors {sortBy === value
								? 'bg-primary text-primary-foreground border-primary'
								: 'border-border hover:bg-accent'}"
						>{label}</button>
					{/each}
				</div>
			</div>
			<div class="overflow-x-auto">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border-border bg-muted/30">
							<th class="px-4 py-2 text-left text-xs font-medium text-muted-foreground">Page</th>
							<th class="px-4 py-2 text-right text-xs font-medium text-muted-foreground">Views</th>
							<th class="px-4 py-2 text-right text-xs font-medium text-muted-foreground">Sessions</th>
							<th class="px-4 py-2 text-right text-xs font-medium text-muted-foreground">% of Total</th>
						</tr>
					</thead>
					<tbody>
						{#each sorted as page}
							{@const pct = totalViews > 0 ? Math.round((page.views / totalViews) * 100) : 0}
							<tr class="border-b border-border/50 hover:bg-accent/30 transition-colors">
								<td class="px-4 py-2.5">
									<p class="font-medium font-mono text-xs">{page.path}</p>
									{#if page.title && page.title !== page.path}
										<p class="text-xs text-muted-foreground mt-0.5 truncate max-w-xs">{page.title}</p>
									{/if}
								</td>
								<td class="px-4 py-2.5 text-right tabular-nums">{page.views.toLocaleString()}</td>
								<td class="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{page.sessions.toLocaleString()}</td>
								<td class="px-4 py-2.5 text-right">
									<div class="flex items-center justify-end gap-2">
										<div class="w-16 bg-muted rounded-full h-1.5 overflow-hidden">
											<div class="h-full bg-primary rounded-full" style="width: {pct}%"></div>
										</div>
										<span class="text-xs text-muted-foreground w-7 text-right">{pct}%</span>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}
</div>
