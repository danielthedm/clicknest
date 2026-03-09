<script lang="ts">
	import { onMount } from 'svelte';
	import { getPages, analyzeICP } from '$lib/api';
	import type { PageStat, ICPAnalysis, ICPUserProfile } from '$lib/types';

	let pages = $state<PageStat[]>([]);
	let selectedPaths = $state<string[]>([]);
	let analysis = $state<ICPAnalysis | null>(null);
	let profiles = $state<ICPUserProfile[]>([]);
	let loading = $state(false);
	let pagesLoading = $state(true);
	let showProfiles = $state(false);

	onMount(async () => {
		try {
			const now = new Date();
			const monthAgo = new Date(now.getTime() - 30 * 86400000);
			const res = await getPages({ start: monthAgo.toISOString(), end: now.toISOString() });
			pages = res.pages ?? [];
		} catch (e) {
			console.error('Failed to load pages:', e);
		}
		pagesLoading = false;
	});

	function togglePath(path: string) {
		if (selectedPaths.includes(path)) {
			selectedPaths = selectedPaths.filter(p => p !== path);
		} else {
			selectedPaths = [...selectedPaths, path];
		}
	}

	async function handleAnalyze() {
		if (selectedPaths.length === 0) return;
		loading = true;
		analysis = null;
		profiles = [];
		try {
			const res = await analyzeICP(selectedPaths);
			analysis = res.analysis;
			profiles = res.profiles ?? [];
		} catch (e) {
			console.error('ICP analysis failed:', e);
			alert(`Analysis failed: ${e}`);
		}
		loading = false;
	}
</script>

<div class="p-6 space-y-6">
	<div>
		<h2 class="text-xl font-semibold">ICP Discovery</h2>
		<p class="text-sm text-muted-foreground">Identify your Ideal Customer Profile by analyzing users who visit conversion pages</p>
	</div>

	<!-- Step 1: Select conversion pages -->
	<div class="border border-border rounded-lg p-4 bg-card">
		<h3 class="text-sm font-medium mb-2">1. Select conversion pages</h3>
		<p class="text-xs text-muted-foreground mb-3">Pick pages that indicate high intent (pricing, signup, checkout, etc.)</p>

		{#if pagesLoading}
			<p class="text-sm text-muted-foreground">Loading pages...</p>
		{:else if pages.length === 0}
			<p class="text-sm text-muted-foreground">No page data available. Send pageview events to see pages here.</p>
		{:else}
			<div class="flex flex-wrap gap-2 max-h-48 overflow-y-auto">
				{#each pages as page}
					{@const selected = selectedPaths.includes(page.path)}
					<button
						onclick={() => togglePath(page.path)}
						class="px-2.5 py-1 text-xs rounded-full border transition-colors {selected
							? 'bg-primary text-primary-foreground border-primary'
							: 'bg-background border-border text-foreground hover:bg-muted'}"
					>
						{page.path}
						<span class="ml-1 opacity-60">{page.views}</span>
					</button>
				{/each}
			</div>
		{/if}

		<div class="mt-3 flex items-center gap-3">
			<button
				onclick={handleAnalyze}
				disabled={loading || selectedPaths.length === 0}
				class="px-4 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90 disabled:opacity-50"
			>
				{loading ? 'Analyzing...' : 'Analyze ICP'}
			</button>
			{#if selectedPaths.length > 0}
				<span class="text-xs text-muted-foreground">{selectedPaths.length} page{selectedPaths.length !== 1 ? 's' : ''} selected</span>
			{/if}
		</div>
	</div>

	<!-- Results -->
	{#if analysis}
		<div class="space-y-4">
			<div class="border border-border rounded-lg p-4 bg-card">
				<h3 class="text-sm font-medium mb-2">ICP Summary</h3>
				<p class="text-sm">{analysis.summary}</p>
			</div>

			<div class="grid grid-cols-3 gap-4">
				<div class="border border-border rounded-lg p-4 bg-card">
					<h3 class="text-sm font-medium mb-2">Common Traits</h3>
					<ul class="space-y-1">
						{#each analysis.common_traits as trait}
							<li class="text-sm text-muted-foreground flex items-start gap-1.5">
								<span class="text-primary mt-0.5 shrink-0">-</span>
								{trait}
							</li>
						{/each}
					</ul>
				</div>

				<div class="border border-border rounded-lg p-4 bg-card">
					<h3 class="text-sm font-medium mb-2">Best Channels</h3>
					<div class="flex flex-wrap gap-1.5">
						{#each analysis.best_channels as channel}
							<span class="text-xs px-2 py-1 rounded-full bg-primary/10 text-primary font-medium">{channel}</span>
						{/each}
					</div>
				</div>

				<div class="border border-border rounded-lg p-4 bg-card">
					<h3 class="text-sm font-medium mb-2">Recommendations</h3>
					<ul class="space-y-1">
						{#each analysis.recommendations as rec}
							<li class="text-sm text-muted-foreground flex items-start gap-1.5">
								<span class="text-primary mt-0.5 shrink-0">-</span>
								{rec}
							</li>
						{/each}
					</ul>
				</div>
			</div>

			<!-- Raw profiles -->
			<div class="border border-border rounded-lg bg-card">
				<button onclick={() => showProfiles = !showProfiles} class="w-full px-4 py-3 text-left flex items-center justify-between">
					<span class="text-sm font-medium">Raw Profiles ({profiles.length})</span>
					<span class="text-xs text-muted-foreground">{showProfiles ? 'Hide' : 'Show'}</span>
				</button>
				{#if showProfiles}
					<div class="border-t border-border">
						<table class="w-full text-sm">
							<thead>
								<tr class="bg-muted/50 border-b border-border">
									<th class="text-left px-4 py-2 font-medium">User</th>
									<th class="text-right px-4 py-2 font-medium">Sessions</th>
									<th class="text-right px-4 py-2 font-medium">Events</th>
									<th class="text-left px-4 py-2 font-medium">Entry Source</th>
									<th class="text-left px-4 py-2 font-medium">Top Pages</th>
								</tr>
							</thead>
							<tbody>
								{#each profiles as p}
									<tr class="border-b border-border">
										<td class="px-4 py-2 font-mono text-xs">{p.distinct_id}</td>
										<td class="px-4 py-2 text-right">{p.session_count}</td>
										<td class="px-4 py-2 text-right">{p.event_count}</td>
										<td class="px-4 py-2 text-xs text-muted-foreground">{p.entry_source || '-'}</td>
										<td class="px-4 py-2 text-xs text-muted-foreground">{(p.top_pages ?? []).join(', ') || '-'}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>
