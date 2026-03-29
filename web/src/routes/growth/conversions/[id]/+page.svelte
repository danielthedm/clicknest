<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { getConversionGoalResults } from '$lib/api';
	import type { ConversionGoal, ConversionAttribution } from '$lib/types';

	let goal: ConversionGoal | null = $state(null);
	let attributions: ConversionAttribution[] = $state([]);
	let totalConversions = $state(0);
	let totalRevenue = $state(0);
	let model = $state('first_touch');
	let loading = $state(true);

	const models = [
		{ value: 'first_touch', label: 'First Touch' },
		{ value: 'last_touch', label: 'Last Touch' },
		{ value: 'linear', label: 'Linear' },
	];

	onMount(() => load());

	$effect(() => {
		if (goal) load();
	});

	async function load() {
		loading = true;
		try {
			const id = $page.params.id;
			const res = await getConversionGoalResults(id, { model });
			goal = res.goal;
			attributions = res.attributions ?? [];
			totalConversions = res.total_conversions;
			totalRevenue = res.total_revenue;
		} catch {}
		loading = false;
	}

	function formatCurrency(v: number): string {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(v);
	}

	// Compute max conversions for bar chart.
	let maxConversions = $derived(Math.max(1, ...attributions.map((a) => a.conversions)));
</script>

<div class="p-6 max-w-5xl mx-auto space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<a href="/growth/conversions" class="text-sm text-muted-foreground hover:text-foreground">&larr; Back to Goals</a>
			<h1 class="text-2xl font-bold text-foreground mt-1">{goal?.name ?? 'Loading...'}</h1>
			{#if goal}
				<p class="text-sm text-muted-foreground mt-1">
					{goal.event_type}{goal.event_name ? `: ${goal.event_name}` : ''} &middot; Revenue: <span class="font-mono">{goal.value_property}</span>
				</p>
			{/if}
		</div>
	</div>

	<!-- Stats Cards -->
	<div class="grid grid-cols-2 gap-4">
		<div class="border border-border rounded-lg p-4 bg-card">
			<div class="text-sm text-muted-foreground">Total Conversions</div>
			<div class="text-2xl font-bold text-foreground mt-1">{totalConversions.toLocaleString()}</div>
		</div>
		<div class="border border-border rounded-lg p-4 bg-card">
			<div class="text-sm text-muted-foreground">Total Revenue</div>
			<div class="text-2xl font-bold text-foreground mt-1">{formatCurrency(totalRevenue)}</div>
		</div>
	</div>

	<!-- Attribution Model Selector -->
	<div class="flex items-center gap-2">
		<span class="text-sm font-medium text-foreground">Attribution Model:</span>
		{#each models as m}
			<button
				onclick={() => { model = m.value; load(); }}
				class="px-3 py-1.5 text-sm rounded-md border transition-colors {model === m.value
					? 'bg-primary text-primary-foreground border-primary'
					: 'border-border text-muted-foreground hover:text-foreground hover:border-foreground/30'}"
			>{m.label}</button>
		{/each}
	</div>

	{#if loading}
		<div class="text-sm text-muted-foreground">Loading results...</div>
	{:else if attributions.length === 0}
		<div class="text-center py-12 text-muted-foreground">
			<p class="text-lg font-medium">No conversion data yet</p>
			<p class="text-sm mt-1">Conversions will appear here once matching events are tracked.</p>
		</div>
	{:else}
		<!-- Attribution Table -->
		<div class="border border-border rounded-lg overflow-hidden">
			<table class="w-full text-sm">
				<thead class="bg-muted/50">
					<tr>
						<th class="text-left px-4 py-3 font-medium text-muted-foreground">Source</th>
						<th class="text-left px-4 py-3 font-medium text-muted-foreground">Channel</th>
						<th class="text-right px-4 py-3 font-medium text-muted-foreground">Conversions</th>
						<th class="text-right px-4 py-3 font-medium text-muted-foreground">Revenue</th>
						<th class="text-right px-4 py-3 font-medium text-muted-foreground">Users</th>
						<th class="px-4 py-3 w-40"></th>
					</tr>
				</thead>
				<tbody>
					{#each attributions as attr}
						<tr class="border-t border-border hover:bg-muted/30">
							<td class="px-4 py-3 font-medium text-foreground">{attr.source}</td>
							<td class="px-4 py-3">
								<span class="inline-block px-2 py-0.5 rounded text-xs bg-muted text-muted-foreground">{attr.channel}</span>
							</td>
							<td class="px-4 py-3 text-right tabular-nums">{attr.conversions.toLocaleString()}</td>
							<td class="px-4 py-3 text-right tabular-nums">{formatCurrency(attr.revenue)}</td>
							<td class="px-4 py-3 text-right tabular-nums">{attr.users.toLocaleString()}</td>
							<td class="px-4 py-3">
								<div class="w-full bg-muted rounded-full h-2">
									<div class="bg-primary h-2 rounded-full" style="width: {(attr.conversions / maxConversions) * 100}%"></div>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
