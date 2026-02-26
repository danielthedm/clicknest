<script lang="ts">
	import { onMount } from 'svelte';
	import { listFunnels, createFunnel, deleteFunnel, getFunnelResults, getFunnelCohorts, suggestFunnels, getNames } from '$lib/api';
	import type { Funnel, FunnelStep, FunnelResult, FunnelCohortResult, SuggestedFunnel, EventName } from '$lib/types';
	import { relativeTime } from '$lib/utils';

	let funnels = $state<Funnel[]>([]);
	let selectedFunnel = $state<Funnel | null>(null);
	let results = $state<FunnelResult[]>([]);
	let loading = $state(true);
	let loadingResults = $state(false);
	let creating = $state(false);

	// Create form
	let showCreate = $state(false);
	let newName = $state('');
	let newSteps = $state<FunnelStep[]>([
		{ event_type: 'pageview', event_name: '' },
		{ event_type: 'click', event_name: '' },
	]);

	let range = $state('30d');

	// Cohort view
	let viewMode = $state<'chart' | 'cohorts'>('chart');
	let cohortInterval = $state('week');
	let cohorts = $state<FunnelCohortResult[]>([]);
	let loadingCohorts = $state(false);

	// Known event names for autocomplete
	let eventNames = $state<EventName[]>([]);

	// Suggest funnel
	let suggestions = $state<SuggestedFunnel[]>([]);
	let loadingSuggestions = $state(false);
	let suggestError = $state('');

	onMount(() => {
		loadFunnels();
		loadEventNames();
	});

	async function loadEventNames() {
		try {
			const res = await getNames();
			eventNames = res.names ?? [];
		} catch (e) {
			console.error('Failed to load event names:', e);
		}
	}

	function knownNamesForType(eventType: string): string[] {
		const names = new Set<string>();
		for (const en of eventNames) {
			const display = en.user_name || en.ai_name;
			if (display) names.add(display);
		}
		return [...names].sort();
	}

	async function loadFunnels() {
		loading = true;
		try {
			const res = await listFunnels();
			funnels = res.funnels ?? [];
		} catch (e) {
			console.error('Failed to load funnels:', e);
		}
		loading = false;
	}

	function getDateRange(): { start: Date; end: Date } {
		const end = new Date();
		let start: Date;
		switch (range) {
			case '7d': start = new Date(end.getTime() - 7 * 24 * 60 * 60 * 1000); break;
			case '30d': start = new Date(end.getTime() - 30 * 24 * 60 * 60 * 1000); break;
			case '90d': start = new Date(end.getTime() - 90 * 24 * 60 * 60 * 1000); break;
			default: start = new Date(end.getTime() - 30 * 24 * 60 * 60 * 1000);
		}
		return { start, end };
	}

	async function selectFunnel(funnel: Funnel) {
		selectedFunnel = funnel;
		viewMode = 'chart';
		cohorts = [];
		loadingResults = true;
		try {
			const { start, end } = getDateRange();
			const res = await getFunnelResults(funnel.id, {
				start: start.toISOString(),
				end: end.toISOString(),
			});
			results = res.results ?? [];
		} catch (e) {
			console.error('Failed to load funnel results:', e);
		}
		loadingResults = false;
	}

	async function loadCohorts() {
		if (!selectedFunnel) return;
		loadingCohorts = true;
		try {
			const { start, end } = getDateRange();
			const res = await getFunnelCohorts(selectedFunnel.id, {
				start: start.toISOString(),
				end: end.toISOString(),
				interval: cohortInterval,
			});
			cohorts = res.cohorts ?? [];
		} catch (e) {
			console.error('Failed to load cohorts:', e);
		}
		loadingCohorts = false;
	}

	function switchViewMode(mode: 'chart' | 'cohorts') {
		viewMode = mode;
		if (mode === 'cohorts' && cohorts.length === 0 && selectedFunnel) {
			loadCohorts();
		}
	}

	function addStep() {
		newSteps = [...newSteps, { event_type: 'click', event_name: '' }];
	}

	function removeStep(index: number) {
		if (newSteps.length <= 2) return;
		newSteps = newSteps.filter((_, i) => i !== index);
	}

	async function handleCreate() {
		if (!newName || newSteps.length < 2) return;
		creating = true;
		try {
			await createFunnel(newName, newSteps);
			newName = '';
			newSteps = [
				{ event_type: 'pageview', event_name: '' },
				{ event_type: 'click', event_name: '' },
			];
			showCreate = false;
			await loadFunnels();
		} catch (e) {
			console.error('Failed to create funnel:', e);
		}
		creating = false;
	}

	async function handleDelete(id: string) {
		if (!confirm('Delete this funnel?')) return;
		try {
			await deleteFunnel(id);
			if (selectedFunnel?.id === id) {
				selectedFunnel = null;
				results = [];
				cohorts = [];
			}
			await loadFunnels();
		} catch (e) {
			console.error('Failed to delete funnel:', e);
		}
	}

	async function handleSuggest() {
		loadingSuggestions = true;
		suggestError = '';
		suggestions = [];
		try {
			const res = await suggestFunnels();
			suggestions = res.suggestions ?? [];
		} catch (e: any) {
			const msg = e?.message ?? String(e);
			// Extract error message from API response JSON.
			const match = msg.match(/"error"\s*:\s*"([^"]+)"/);
			suggestError = match ? match[1] : 'Failed to get suggestions. Check LLM configuration.';
		}
		loadingSuggestions = false;
	}

	function useSuggestion(s: SuggestedFunnel) {
		newName = s.name;
		newSteps = s.steps.map(st => ({ ...st }));
		showCreate = true;
		suggestions = [];
	}

	function conversionRate(i: number): string {
		if (i === 0 || results.length === 0) return '100%';
		const prev = results[i - 1]?.count ?? 0;
		if (prev === 0) return '0%';
		return Math.round((results[i].count / prev) * 100) + '%';
	}

	function overallRate(): string {
		if (results.length < 2 || results[0].count === 0) return '0%';
		return Math.round((results[results.length - 1].count / results[0].count) * 100) + '%';
	}

	function cohortCellPct(count: number, firstStepCount: number): number {
		if (firstStepCount === 0) return 0;
		return Math.round((count / firstStepCount) * 100);
	}

	function cohortHeatColor(pct: number): string {
		if (pct >= 80) return 'bg-green-500/30';
		if (pct >= 60) return 'bg-green-500/20';
		if (pct >= 40) return 'bg-yellow-500/20';
		if (pct >= 20) return 'bg-orange-500/20';
		if (pct > 0) return 'bg-red-500/15';
		return '';
	}

	function formatCohortLabel(cohort: string): string {
		try {
			const d = new Date(cohort);
			return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
		} catch {
			return cohort;
		}
	}
</script>

<div class="p-6 max-w-6xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Funnels</h2>
			<p class="text-sm text-muted-foreground mt-1">Conversion funnel analysis</p>
		</div>
		<div class="flex gap-2">
			<button
				onclick={handleSuggest}
				disabled={loadingSuggestions}
				class="px-3 py-1.5 text-sm rounded-md border border-border hover:bg-accent disabled:opacity-50"
			>
				{#if loadingSuggestions}
					<span class="inline-flex items-center gap-1.5">
						<span class="w-3 h-3 border-2 border-current border-t-transparent rounded-full animate-spin"></span>
						Analyzing...
					</span>
				{:else}
					Suggest Funnel
				{/if}
			</button>
			<button
				onclick={() => showCreate = !showCreate}
				class="px-3 py-1.5 text-sm rounded-md border border-border hover:bg-accent"
			>
				{showCreate ? 'Cancel' : '+ New Funnel'}
			</button>
		</div>
	</div>

	<!-- Suggest error -->
	{#if suggestError}
		<div class="border border-red-300 bg-red-50 dark:bg-red-950/30 dark:border-red-800 text-red-700 dark:text-red-400 rounded-lg p-3 mb-4 text-sm flex items-center justify-between">
			<span>{suggestError}</span>
			<button onclick={() => suggestError = ''} class="text-red-500 hover:text-red-700 ml-2 text-xs">Dismiss</button>
		</div>
	{/if}

	<!-- Suggestions -->
	{#if suggestions.length > 0}
		<div class="border border-border rounded-lg p-4 bg-card mb-6">
			<h3 class="text-sm font-medium mb-3">AI-Suggested Funnels</h3>
			<div class="grid gap-3 sm:grid-cols-2">
				{#each suggestions as s}
					<div class="border border-border rounded-lg p-3 hover:border-primary/50 transition-colors">
						<div class="flex items-start justify-between mb-1">
							<p class="text-sm font-medium">{s.name}</p>
							<button
								onclick={() => useSuggestion(s)}
								class="px-2 py-0.5 text-xs rounded border border-primary text-primary hover:bg-primary hover:text-primary-foreground transition-colors shrink-0 ml-2"
							>
								Use
							</button>
						</div>
						<p class="text-xs text-muted-foreground mb-2">{s.description}</p>
						<div class="flex flex-wrap items-center gap-1">
							{#each s.steps as step, i}
								{#if i > 0}
									<span class="text-xs text-muted-foreground">&rarr;</span>
								{/if}
								<span class="px-1.5 py-0.5 text-xs rounded bg-muted">
									{step.event_name || step.event_type}
								</span>
							{/each}
						</div>
					</div>
				{/each}
			</div>
			<button onclick={() => suggestions = []} class="mt-3 text-xs text-muted-foreground hover:text-foreground">Dismiss suggestions</button>
		</div>
	{/if}

	<!-- Create form -->
	{#if showCreate}
		<div class="border border-border rounded-lg p-4 bg-card mb-6">
			<h3 class="text-sm font-medium mb-3">Create Funnel</h3>
			<div class="space-y-3">
				<input
					bind:value={newName}
					placeholder="Funnel name"
					class="w-full px-3 py-1.5 text-sm border border-border rounded-md bg-background"
				/>
				<datalist id="known-event-names">
					{#each knownNamesForType('') as name}
						<option value={name}></option>
					{/each}
				</datalist>
				{#each newSteps as step, i}
					<div class="flex gap-2 items-center">
						<span class="text-xs text-muted-foreground w-16">Step {i + 1}</span>
						<select
							bind:value={step.event_type}
							class="px-3 py-1.5 text-sm border border-border rounded-md bg-background"
						>
							<option value="pageview">Pageview</option>
							<option value="click">Click</option>
							<option value="submit">Submit</option>
							<option value="input">Input</option>
							<option value="custom">Custom</option>
						</select>
						<input
							bind:value={step.event_name}
							list="known-event-names"
							placeholder="e.g. Add to Cart, /checkout"
							class="flex-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background"
						/>
						{#if newSteps.length > 2}
							<button onclick={() => removeStep(i)} class="text-xs text-red-500 hover:text-red-700">Remove</button>
						{/if}
					</div>
				{/each}
				<p class="text-xs text-muted-foreground">
					Each step matches events by type + name. Use a specific name (from the dropdown) to target a particular action — e.g. "Click: Add to Cart" then "Pageview: /checkout".
				</p>
				<div class="flex gap-2">
					<button onclick={addStep} class="px-3 py-1.5 text-sm border border-border rounded-md hover:bg-accent">+ Add Step</button>
					<button
						onclick={handleCreate}
						disabled={creating || !newName || newSteps.length < 2}
						class="px-3 py-1.5 text-sm rounded-md bg-primary text-primary-foreground disabled:opacity-50"
					>
						{creating ? 'Creating...' : 'Create Funnel'}
					</button>
				</div>
			</div>
		</div>
	{/if}

	<div class="grid grid-cols-[1fr_1.5fr] gap-4">
		<!-- Funnel list -->
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<div class="px-4 py-3 border-b border-border">
				<h3 class="text-sm font-medium">Saved Funnels ({funnels.length})</h3>
			</div>
			{#if loading}
				<div class="p-8 text-center text-muted-foreground text-sm">Loading...</div>
			{:else if funnels.length === 0}
				<div class="p-8 text-center text-muted-foreground text-sm">No funnels yet. Create one above.</div>
			{:else}
				<div class="divide-y divide-border max-h-[500px] overflow-y-auto">
					{#each funnels as funnel}
						<div class="flex items-center justify-between px-4 py-3 hover:bg-accent/50 transition-colors {selectedFunnel?.id === funnel.id ? 'bg-accent' : ''}">
							<button
								onclick={() => selectFunnel(funnel)}
								class="text-left flex-1"
							>
								<p class="text-sm font-medium">{funnel.name}</p>
								<p class="text-xs text-muted-foreground mt-0.5">{relativeTime(funnel.created_at)}</p>
							</button>
							<button
								onclick={() => handleDelete(funnel.id)}
								class="text-xs text-red-500 hover:text-red-700 ml-2"
							>Delete</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Funnel results -->
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<div class="px-4 py-3 border-b border-border flex items-center justify-between">
				<h3 class="text-sm font-medium">
					{#if selectedFunnel}
						{selectedFunnel.name} {viewMode === 'chart' ? `— Conversion: ${overallRate()}` : '— Cohorts'}
					{:else}
						Select a funnel
					{/if}
				</h3>
				{#if selectedFunnel}
					<div class="flex gap-1 items-center">
						<!-- View mode toggle -->
						<div class="flex gap-0.5 mr-2 border border-border rounded overflow-hidden">
							<button
								onclick={() => switchViewMode('chart')}
								class="px-2 py-1 text-xs transition-colors {viewMode === 'chart'
									? 'bg-primary text-primary-foreground'
									: 'hover:bg-accent'}"
							>
								Chart
							</button>
							<button
								onclick={() => switchViewMode('cohorts')}
								class="px-2 py-1 text-xs transition-colors {viewMode === 'cohorts'
									? 'bg-primary text-primary-foreground'
									: 'hover:bg-accent'}"
							>
								Cohorts
							</button>
						</div>
						{#if viewMode === 'cohorts'}
							<!-- Interval selector -->
							{#each [['day', 'Day'], ['week', 'Week'], ['month', 'Month']] as [value, label]}
								<button
									onclick={() => { cohortInterval = value; loadCohorts(); }}
									class="px-2 py-1 text-xs rounded border transition-colors {cohortInterval === value
										? 'bg-primary text-primary-foreground border-primary'
										: 'border-border hover:bg-accent'}"
								>
									{label}
								</button>
							{/each}
						{:else}
							<!-- Date range -->
							{#each [['7d', '7D'], ['30d', '30D'], ['90d', '90D']] as [value, label]}
								<button
									onclick={() => { range = value; selectFunnel(selectedFunnel!); }}
									class="px-2 py-1 text-xs rounded border transition-colors {range === value
										? 'bg-primary text-primary-foreground border-primary'
										: 'border-border hover:bg-accent'}"
								>
									{label}
								</button>
							{/each}
						{/if}
					</div>
				{/if}
			</div>

			{#if !selectedFunnel}
				<div class="p-8 text-center text-muted-foreground text-sm">Click a funnel to view results</div>
			{:else if viewMode === 'chart'}
				<!-- Chart view -->
				{#if loadingResults}
					<div class="p-8 text-center text-muted-foreground text-sm">Loading...</div>
				{:else if results.length === 0}
					<div class="p-8 text-center text-muted-foreground text-sm">No results for this period</div>
				{:else}
					<div class="p-4 space-y-3">
						{#each results as result, i}
							{@const maxCount = results[0].count || 1}
							{@const pct = Math.round((result.count / maxCount) * 100)}
							<div>
								<div class="flex items-center justify-between mb-1">
									<span class="text-sm">{result.step}</span>
									<span class="text-sm text-muted-foreground">{result.count.toLocaleString()} {i > 0 ? `(${conversionRate(i)} from prev)` : ''}</span>
								</div>
								<div class="w-full bg-muted rounded-full h-6 overflow-hidden">
									<div
										class="h-full bg-primary rounded-full flex items-center justify-end pr-2 transition-all"
										style="width: {pct}%"
									>
										{#if pct > 10}
											<span class="text-xs text-primary-foreground font-medium">{pct}%</span>
										{/if}
									</div>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			{:else}
				<!-- Cohorts view -->
				{#if loadingCohorts}
					<div class="p-8 text-center text-muted-foreground text-sm">Loading cohorts...</div>
				{:else if cohorts.length === 0}
					<div class="p-8 text-center text-muted-foreground text-sm">No cohort data for this period</div>
				{:else}
					<div class="overflow-x-auto">
						<table class="w-full text-xs">
							<thead>
								<tr class="border-b border-border">
									<th class="px-3 py-2 text-left font-medium text-muted-foreground">Cohort</th>
									{#each cohorts[0].steps as step}
										<th class="px-3 py-2 text-right font-medium text-muted-foreground">{step.step}</th>
									{/each}
								</tr>
							</thead>
							<tbody>
								{#each cohorts as cohort}
									{@const firstCount = cohort.steps[0]?.count ?? 0}
									<tr class="border-b border-border/50">
										<td class="px-3 py-2 font-medium whitespace-nowrap">{formatCohortLabel(cohort.cohort)}</td>
										{#each cohort.steps as step, i}
											{@const pct = i === 0 ? 100 : cohortCellPct(step.count, firstCount)}
											<td class="px-3 py-2 text-right {cohortHeatColor(pct)}">
												<span class="font-medium">{step.count.toLocaleString()}</span>
												{#if i > 0}
													<span class="text-muted-foreground ml-1">{pct}%</span>
												{/if}
											</td>
										{/each}
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			{/if}
		</div>
	</div>
</div>
