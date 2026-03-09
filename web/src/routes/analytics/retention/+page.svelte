<script lang="ts">
	import { onMount } from 'svelte';
	import { getRetention } from '$lib/api';
	import type { RetentionCohort } from '$lib/types';
	import AiInsight from '$lib/components/ui/AiInsight.svelte';

	let cohorts = $state<RetentionCohort[]>([]);
	let loading = $state(true);
	let interval = $state('week');
	let periods = $state(8);
	let range = $state('90d');

	onMount(() => {
		loadRetention();
	});

	async function loadRetention() {
		loading = true;
		try {
			const end = new Date();
			let start: Date;
			switch (range) {
				case '30d': start = new Date(end.getTime() - 30 * 24 * 60 * 60 * 1000); break;
				case '90d': start = new Date(end.getTime() - 90 * 24 * 60 * 60 * 1000); break;
				case '180d': start = new Date(end.getTime() - 180 * 24 * 60 * 60 * 1000); break;
				default: start = new Date(end.getTime() - 90 * 24 * 60 * 60 * 1000);
			}
			const res = await getRetention({
				interval,
				periods: periods.toString(),
				start: start.toISOString(),
				end: end.toISOString(),
			});
			cohorts = res.cohorts ?? [];
		} catch (e) {
			console.error('Failed to load retention:', e);
		}
		loading = false;
	}

	function formatCohort(cohort: string): string {
		const d = new Date(cohort);
		return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: '2-digit' });
	}

	function retentionPct(cohort: RetentionCohort, periodIdx: number): number {
		if (cohort.size === 0 || !cohort.retention[periodIdx]) return 0;
		return Math.round((cohort.retention[periodIdx] / cohort.size) * 100);
	}

	let retentionPrompt = $derived(() => {
		if (cohorts.length === 0) return '';
		const rows = cohorts.slice(0, 8).map(c => {
			const pcts = c.retention.map((v, i) => `${interval} ${i}: ${c.size > 0 ? Math.round((v / c.size) * 100) : 0}%`).join(', ');
			return `${c.cohort} (${c.size} users): ${pcts}`;
		}).join('\n');
		return `Analyze this ${interval}ly retention data (${range} window, ${periods} periods). Give 2-3 brief, actionable insights about user retention trends, which cohorts are strongest/weakest, and what might improve retention.\n\n${rows}`;
	});

	let retentionReady = $derived(!loading && cohorts.length > 0);

	function cellColor(pct: number): string {
		if (pct === 0) return 'background: hsl(215 20% 95%)';
		const intensity = Math.min(pct / 100, 1);
		const lightness = 90 - intensity * 45;
		return `background: hsl(215 70% ${lightness}%); color: ${intensity > 0.5 ? 'white' : 'inherit'}`;
	}
</script>

<div class="p-6 max-w-6xl">
	<div class="mb-6">
		<h2 class="text-2xl font-bold tracking-tight">Retention</h2>
		<p class="text-sm text-muted-foreground mt-1">User retention cohort analysis</p>
	</div>

	<!-- Controls -->
	<div class="flex gap-4 mb-6 flex-wrap">
		<div class="flex gap-2 items-center">
			<span class="text-sm text-muted-foreground">Interval:</span>
			{#each [['day', 'Day'], ['week', 'Week'], ['month', 'Month']] as [value, label]}
				<button
					onclick={() => { interval = value; loadRetention(); }}
					class="px-3 py-1.5 text-sm rounded-md border transition-colors {interval === value
						? 'bg-primary text-primary-foreground border-primary'
						: 'border-border hover:bg-accent'}"
				>
					{label}
				</button>
			{/each}
		</div>
		<div class="flex gap-2 items-center">
			<span class="text-sm text-muted-foreground">Periods:</span>
			{#each [4, 8, 12] as p}
				<button
					onclick={() => { periods = p; loadRetention(); }}
					class="px-3 py-1.5 text-sm rounded-md border transition-colors {periods === p
						? 'bg-primary text-primary-foreground border-primary'
						: 'border-border hover:bg-accent'}"
				>
					{p}
				</button>
			{/each}
		</div>
		<div class="flex gap-2 items-center">
			<span class="text-sm text-muted-foreground">Range:</span>
			{#each [['30d', '30D'], ['90d', '90D'], ['180d', '180D']] as [value, label]}
				<button
					onclick={() => { range = value; loadRetention(); }}
					class="px-3 py-1.5 text-sm rounded-md border transition-colors {range === value
						? 'bg-primary text-primary-foreground border-primary'
						: 'border-border hover:bg-accent'}"
				>
					{label}
				</button>
			{/each}
		</div>
	</div>

	<AiInsight cacheKey="retention_{interval}_{range}_{periods}" prompt={retentionPrompt()} ready={retentionReady} />

	<!-- Retention table -->
	<div class="border border-border rounded-lg bg-card overflow-x-auto">
		{#if loading}
			<div class="flex items-center justify-center h-64">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
			</div>
		{:else if cohorts.length === 0}
			<p class="text-sm text-muted-foreground py-16 text-center">No retention data for this period</p>
		{:else}
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b border-border bg-muted/50">
						<th class="px-3 py-2.5 text-left font-medium text-muted-foreground whitespace-nowrap">Cohort</th>
						<th class="px-3 py-2.5 text-center font-medium text-muted-foreground">Users</th>
						{#each Array(periods + 1) as _, i}
							<th class="px-3 py-2.5 text-center font-medium text-muted-foreground whitespace-nowrap">
								{i === 0 ? `${interval} 0` : `${interval} ${i}`}
							</th>
						{/each}
					</tr>
				</thead>
				<tbody class="divide-y divide-border">
					{#each cohorts as cohort}
						<tr>
							<td class="px-3 py-2 whitespace-nowrap text-muted-foreground">{formatCohort(cohort.cohort)}</td>
							<td class="px-3 py-2 text-center font-medium">{cohort.size}</td>
							{#each Array(periods + 1) as _, i}
								{@const pct = retentionPct(cohort, i)}
								<td class="px-3 py-2 text-center text-xs font-medium" style={cellColor(pct)}>
									{pct}%
								</td>
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</div>
