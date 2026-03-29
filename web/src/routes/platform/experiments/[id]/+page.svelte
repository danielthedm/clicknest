<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { getExperimentResults, stopExperiment, declareWinner, updateExperiment } from '$lib/api';
	import type { ExperimentResults, ExperimentVariantResult } from '$lib/types';

	let results: ExperimentResults | null = $state(null);
	let loading = $state(true);
	let declaring = $state(false);

	onMount(load);

	async function load() {
		loading = true;
		try {
			results = await getExperimentResults($page.params.id);
		} catch {}
		loading = false;
	}

	async function handleStop() {
		try {
			await stopExperiment($page.params.id);
			await load();
		} catch {}
	}

	async function handleDeclareWinner(variant: string) {
		declaring = true;
		try {
			await declareWinner($page.params.id, variant);
			await load();
		} catch {}
		declaring = false;
	}

	async function toggleAutoStop() {
		if (!results?.experiment) return;
		const exp = results.experiment;
		try {
			await updateExperiment($page.params.id, {
				name: exp.name,
				status: exp.status,
				auto_stop: !exp.auto_stop,
				conversion_goal_id: exp.conversion_goal_id ?? '',
			});
			await load();
		} catch {}
	}

	function formatPct(v: number): string {
		return v.toFixed(2) + '%';
	}

	function formatCurrency(v: number): string {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(v);
	}

	let isSignificant = $derived(
		(results?.significance?.significant ?? false) || (results?.chi_squared?.significant ?? false)
	);

	let pValue = $derived(
		results?.significance?.p_value ?? results?.chi_squared?.p_value ?? null
	);

	let bestVariant = $derived(
		results?.variants?.reduce<ExperimentVariantResult | null>((best, v) =>
			!best || v.conversion_rate > best.conversion_rate ? v : best, null
		) ?? null
	);
</script>

<div class="p-6 max-w-5xl mx-auto space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<a href="/platform/experiments" class="text-sm text-muted-foreground hover:text-foreground">&larr; Back to Experiments</a>
			<h1 class="text-2xl font-bold text-foreground mt-1">{results?.experiment?.name ?? 'Loading...'}</h1>
			{#if results?.experiment}
				<p class="text-sm text-muted-foreground mt-1">
					Flag: <span class="font-mono">{results.experiment.flag_key}</span>
					&middot; Started: {new Date(results.experiment.started_at).toLocaleDateString()}
					{#if results.experiment.ended_at}
						&middot; Ended: {new Date(results.experiment.ended_at).toLocaleDateString()}
					{/if}
				</p>
			{/if}
		</div>
		{#if results?.experiment?.status === 'running'}
			<div class="flex gap-2">
				<button onclick={toggleAutoStop}
					class="px-3 py-2 text-sm rounded-md border border-border hover:bg-muted {results.experiment.auto_stop ? 'bg-blue-50 dark:bg-blue-900/20 border-blue-200 dark:border-blue-800' : ''}"
				>{results.experiment.auto_stop ? 'Auto-stop ON' : 'Auto-stop OFF'}</button>
				<button onclick={handleStop} class="px-4 py-2 text-sm rounded-md border border-border hover:bg-muted text-foreground">Stop Experiment</button>
			</div>
		{/if}
	</div>

	{#if loading}
		<div class="text-sm text-muted-foreground">Loading results...</div>
	{:else if !results}
		<div class="text-center py-12 text-muted-foreground">Failed to load experiment results.</div>
	{:else}
		<!-- Status & Significance -->
		<div class="grid grid-cols-3 gap-4">
			<div class="border border-border rounded-lg p-4 bg-card">
				<div class="text-sm text-muted-foreground">Status</div>
				<div class="text-lg font-bold text-foreground mt-1 capitalize">{results.experiment.status}</div>
				{#if results.experiment.winner_variant}
					<div class="text-sm text-green-600 dark:text-green-400 mt-1">Winner: {results.experiment.winner_variant}</div>
				{/if}
			</div>
			<div class="border border-border rounded-lg p-4 bg-card">
				<div class="text-sm text-muted-foreground">Statistical Significance</div>
				<div class="text-lg font-bold mt-1 {isSignificant ? 'text-green-600 dark:text-green-400' : 'text-yellow-600 dark:text-yellow-400'}">
					{isSignificant ? 'Significant' : 'Not yet significant'}
				</div>
				{#if pValue !== null}
					<div class="text-sm text-muted-foreground mt-1">p-value: {pValue.toFixed(4)}</div>
				{/if}
			</div>
			<div class="border border-border rounded-lg p-4 bg-card">
				<div class="text-sm text-muted-foreground">Sample Size</div>
				{#if results.sample_size_needed > 0}
					<div class="text-lg font-bold text-foreground mt-1">{results.sample_size_needed.toLocaleString()} more needed</div>
					<div class="text-sm text-muted-foreground mt-1">for 95% confidence at 10% MDE</div>
				{:else}
					<div class="text-lg font-bold text-green-600 dark:text-green-400 mt-1">Sufficient</div>
				{/if}
			</div>
		</div>

		<!-- Variant Results Table -->
		<div class="border border-border rounded-lg overflow-hidden">
			<table class="w-full text-sm">
				<thead class="bg-muted/50">
					<tr>
						<th class="text-left px-4 py-3 font-medium text-muted-foreground">Variant</th>
						<th class="text-right px-4 py-3 font-medium text-muted-foreground">Exposures</th>
						<th class="text-right px-4 py-3 font-medium text-muted-foreground">Conversions</th>
						<th class="text-right px-4 py-3 font-medium text-muted-foreground">Conv. Rate</th>
						<th class="text-right px-4 py-3 font-medium text-muted-foreground">95% CI</th>
						<th class="text-right px-4 py-3 font-medium text-muted-foreground">Revenue</th>
						{#if results.experiment.status === 'running' && isSignificant}
							<th class="text-right px-4 py-3 font-medium text-muted-foreground">Action</th>
						{/if}
					</tr>
				</thead>
				<tbody>
					{#each results.variants as variant}
						{@const isBest = bestVariant?.variant === variant.variant}
						<tr class="border-t border-border hover:bg-muted/30 {isBest ? 'bg-green-50/50 dark:bg-green-900/10' : ''}">
							<td class="px-4 py-3 font-medium text-foreground">
								{variant.variant}
								{#if isBest && isSignificant}
									<span class="ml-1 text-xs text-green-600 dark:text-green-400">best</span>
								{/if}
							</td>
							<td class="px-4 py-3 text-right tabular-nums">{variant.exposures.toLocaleString()}</td>
							<td class="px-4 py-3 text-right tabular-nums">{variant.conversions.toLocaleString()}</td>
							<td class="px-4 py-3 text-right tabular-nums font-medium">{formatPct(variant.conversion_rate)}</td>
							<td class="px-4 py-3 text-right tabular-nums text-muted-foreground text-xs">[{formatPct(variant.confidence_low)}, {formatPct(variant.confidence_high)}]</td>
							<td class="px-4 py-3 text-right tabular-nums">{formatCurrency(variant.revenue)}</td>
							{#if results.experiment.status === 'running' && isSignificant}
								<td class="px-4 py-3 text-right">
									<button
										onclick={() => handleDeclareWinner(variant.variant)}
										disabled={declaring}
										class="px-3 py-1 text-xs rounded-md bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
									>Declare Winner</button>
								</td>
							{/if}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		{#if results.experiment.status === 'running' && !isSignificant}
			<div class="border border-yellow-200 dark:border-yellow-800 bg-yellow-50 dark:bg-yellow-900/20 rounded-lg p-4 text-sm text-yellow-800 dark:text-yellow-300">
				The experiment has not yet reached statistical significance. Continue collecting data before declaring a winner.
				{#if results.sample_size_needed > 0}
					You need approximately <span class="font-bold">{results.sample_size_needed.toLocaleString()}</span> more exposures per variant for 95% confidence.
				{/if}
			</div>
		{/if}
	{/if}
</div>
