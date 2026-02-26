<script lang="ts">
	import { onMount } from 'svelte';
	import { getTrends, getTrendsBreakdown } from '$lib/api';
	import type { TrendPoint, TrendSeries } from '$lib/types';
	import Chart from '$lib/components/ui/Chart.svelte';
	import { getCssColor, baseLineOptions, type ChartConfiguration } from '$lib/chart-config';

	const PALETTE = [
		'hsl(217 91% 60%)', 'hsl(142 71% 45%)', 'hsl(263 70% 50%)',
		'hsl(25 95% 53%)', 'hsl(0 84% 60%)', 'hsl(180 70% 45%)',
		'hsl(45 93% 55%)', 'hsl(320 70% 55%)',
	];

	let data = $state<TrendPoint[]>([]);
	let series = $state<TrendSeries[]>([]);
	let interval = $state('hour');
	let range = $state('24h');
	let breakdown = $state('none');
	let loading = $state(true);

	onMount(() => loadTrends());

	function getDateRange() {
		const end = new Date();
		let start: Date;
		switch (range) {
			case '1h': start = new Date(end.getTime() - 60 * 60 * 1000); break;
			case '24h': start = new Date(end.getTime() - 24 * 60 * 60 * 1000); break;
			case '7d': start = new Date(end.getTime() - 7 * 24 * 60 * 60 * 1000); break;
			case '30d': start = new Date(end.getTime() - 30 * 24 * 60 * 60 * 1000); break;
			default: start = new Date(end.getTime() - 24 * 60 * 60 * 1000);
		}
		return { start, end };
	}

	async function loadTrends() {
		loading = true;
		try {
			const { start, end } = getDateRange();
			if (breakdown === 'none') {
				const res = await getTrends({ interval, start: start.toISOString(), end: end.toISOString() });
				data = res.data ?? [];
				series = [];
			} else {
				const res = await getTrendsBreakdown({ interval, group_by: breakdown, start: start.toISOString(), end: end.toISOString() });
				series = res.series ?? [];
				data = [];
			}
		} catch (e) {
			console.error('Failed to load trends:', e);
		}
		loading = false;
	}

	function autoInterval(r: string) {
		if (r === '1h') return 'minute';
		if (r === '24h') return 'hour';
		return 'day';
	}

	function totalCount(d: TrendPoint[]): number {
		return d.reduce((sum, p) => sum + p.count, 0);
	}

	function formatBucket(bucket: string): string {
		const d = new Date(bucket);
		if (interval === 'hour' || interval === 'minute') {
			return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
		}
		return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
	}

	// Collect all buckets across all series for the x-axis.
	function allBuckets(ss: TrendSeries[]): string[] {
		const set = new Set<string>();
		for (const s of ss) for (const p of s.data) set.add(p.bucket);
		return [...set].sort();
	}

	let singleTotal = $derived(totalCount(data));

	let seriesTotal = $derived(
		series.reduce((sum, s) => sum + totalCount(s.data), 0)
	);

	let seriesSorted = $derived(
		[...series].sort((a, b) => totalCount(b.data) - totalCount(a.data))
	);

	let chartConfig = $derived<ChartConfiguration>(
		breakdown === 'none'
			? {
				type: 'line',
				data: {
					labels: data.map(p => formatBucket(p.bucket)),
					datasets: [{
						data: data.map(p => p.count),
						borderColor: getCssColor('primary'),
						backgroundColor: getCssColor('primary', 0.12),
						fill: true,
						tension: 0.3,
						borderWidth: 2,
						pointRadius: data.length > 50 ? 0 : 3,
						pointHoverRadius: 4,
					}],
				},
				options: {
					...baseLineOptions(),
					plugins: { ...baseLineOptions().plugins, legend: { display: false },
						tooltip: { ...baseLineOptions().plugins?.tooltip, callbacks: { label: (ctx: any) => `${ctx.parsed.y} events` } } },
				},
			}
			: (() => {
				const buckets = allBuckets(series);
				const bucketIndex = Object.fromEntries(buckets.map((b, i) => [b, i]));
				return {
					type: 'line',
					data: {
						labels: buckets.map(b => formatBucket(b)),
						datasets: seriesSorted.map((s, i) => {
							const color = PALETTE[i % PALETTE.length];
							const pts = new Array(buckets.length).fill(0);
							for (const p of s.data) {
								const idx = bucketIndex[p.bucket];
								if (idx !== undefined) pts[idx] = p.count;
							}
							return {
								label: s.name,
								data: pts,
								borderColor: color,
								backgroundColor: color.replace(')', ' / 0.1)').replace('hsl(', 'hsl('),
								fill: false,
								tension: 0.3,
								borderWidth: 2,
								pointRadius: buckets.length > 50 ? 0 : 2,
								pointHoverRadius: 4,
							};
						}),
					},
					options: {
						...baseLineOptions(),
						plugins: {
							...baseLineOptions().plugins,
							legend: { display: true, position: 'bottom', labels: { color: getCssColor('muted-foreground', 0.8), font: { size: 10 }, boxWidth: 12, padding: 12 } },
							tooltip: { ...baseLineOptions().plugins?.tooltip, mode: 'index' as const, intersect: false },
						},
					},
				};
			})()
	);
</script>

<div class="p-6 max-w-6xl">
	<div class="mb-6">
		<h2 class="text-2xl font-bold tracking-tight">Trends</h2>
		<p class="text-sm text-muted-foreground mt-1">Event volume over time</p>
	</div>

	<!-- Controls -->
	<div class="flex flex-wrap gap-2 mb-6 items-center">
		<div class="flex gap-1">
			{#each [['1h', '1H'], ['24h', '24H'], ['7d', '7D'], ['30d', '30D']] as [value, label]}
				<button
					onclick={() => { range = value; interval = autoInterval(value); loadTrends(); }}
					class="px-3 py-1.5 text-sm rounded-md border transition-colors {range === value
						? 'bg-primary text-primary-foreground border-primary'
						: 'border-border hover:bg-accent'}"
				>{label}</button>
			{/each}
		</div>

		<div class="h-5 w-px bg-border mx-1"></div>

		<div class="flex items-center gap-2">
			<span class="text-xs text-muted-foreground">Break down by</span>
			<select
				bind:value={breakdown}
				onchange={() => loadTrends()}
				class="px-2 py-1.5 text-sm border border-border rounded-md bg-background"
			>
				<option value="none">None</option>
				<option value="event_name">Event name</option>
				<option value="event_type">Event type</option>
				<option value="url_path">URL path</option>
			</select>
		</div>
	</div>

	<!-- Total -->
	<div class="border border-border rounded-lg p-4 bg-card mb-6">
		<p class="text-sm text-muted-foreground">Total Events in Period</p>
		<p class="text-4xl font-bold mt-1">{(breakdown === 'none' ? singleTotal : seriesTotal).toLocaleString()}</p>
	</div>

	<!-- Chart -->
	<div class="border border-border rounded-lg p-6 bg-card mb-6">
		{#if loading}
			<div class="flex items-center justify-center h-64">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
			</div>
		{:else if breakdown === 'none' && data.length === 0}
			<p class="text-sm text-muted-foreground py-16 text-center">No data for this period</p>
		{:else if breakdown !== 'none' && series.length === 0}
			<p class="text-sm text-muted-foreground py-16 text-center">No data for this period</p>
		{:else}
			<Chart config={chartConfig} class={breakdown !== 'none' && series.length > 4 ? 'h-80' : 'h-64'} />
		{/if}
	</div>

	<!-- Breakdown table -->
	{#if breakdown !== 'none' && seriesSorted.length > 0}
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<div class="px-4 py-3 border-b border-border">
				<h3 class="text-sm font-medium">Breakdown by {breakdown.replace('_', ' ')}</h3>
			</div>
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b border-border bg-muted/30">
						<th class="px-4 py-2 text-left text-xs font-medium text-muted-foreground w-4"></th>
						<th class="px-4 py-2 text-left text-xs font-medium text-muted-foreground">Name</th>
						<th class="px-4 py-2 text-right text-xs font-medium text-muted-foreground">Events</th>
						<th class="px-4 py-2 text-right text-xs font-medium text-muted-foreground">% of Total</th>
					</tr>
				</thead>
				<tbody>
					{#each seriesSorted as s, i}
						{@const count = totalCount(s.data)}
						{@const pct = seriesTotal > 0 ? Math.round((count / seriesTotal) * 100) : 0}
						<tr class="border-b border-border/50 hover:bg-accent/30 transition-colors">
							<td class="px-4 py-2.5">
								<div class="w-3 h-3 rounded-full" style="background: {PALETTE[i % PALETTE.length]}"></div>
							</td>
							<td class="px-4 py-2.5 font-medium">{s.name}</td>
							<td class="px-4 py-2.5 text-right tabular-nums">{count.toLocaleString()}</td>
							<td class="px-4 py-2.5 text-right">
								<div class="flex items-center justify-end gap-2">
									<div class="w-16 bg-muted rounded-full h-1.5 overflow-hidden">
										<div class="h-full rounded-full" style="width: {pct}%; background: {PALETTE[i % PALETTE.length]}"></div>
									</div>
									<span class="text-xs text-muted-foreground w-7 text-right">{pct}%</span>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
