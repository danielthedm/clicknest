<script lang="ts">
	import { onMount } from 'svelte';
	import { getPages, analyzeICP, listICPAnalyses, deleteICPAnalysis, icpGenerateCampaign, icpCreateScoringRules, getICPSettings, putICPSettings } from '$lib/api';
	import type { PageStat, ICPAnalysis, ICPUserProfile, SavedICPAnalysis } from '$lib/types';
	import Select from '$lib/components/ui/Select.svelte';

	let activeTab = $state<'analyze' | 'history'>('analyze');
	let pages = $state<PageStat[]>([]);
	let selectedPaths = $state<string[]>([]);
	let analysis = $state<ICPAnalysis | null>(null);
	let profiles = $state<ICPUserProfile[]>([]);
	let loading = $state(false);
	let pagesLoading = $state(true);
	let showProfiles = $state(false);

	let savedAnalyses = $state<SavedICPAnalysis[]>([]);
	let historyLoading = $state(true);
	let expandedId = $state<string | null>(null);

	// ICP → actions
	const icpChannels = ['reddit', 'linkedin', 'twitter', 'blog'];
	let icpActionChannel = $state('reddit');
	let icpActionId = $state<string | null>(null);
	let icpActionType = $state<'campaign' | 'rules' | null>(null);

	// Compare mode
	let compareMode = $state(false);
	let compareIds = $state<string[]>([]);

	// Auto-refresh
	let autoRefresh = $state(false);
	let autoRefreshSaving = $state(false);

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

		loadHistory();
		try {
			const s = await getICPSettings();
			autoRefresh = s.icp_auto_refresh;
		} catch { /* ignore */ }
	});

	async function loadHistory() {
		historyLoading = true;
		try {
			const res = await listICPAnalyses();
			savedAnalyses = res.analyses ?? [];
		} catch (e) {
			console.error('Failed to load ICP history:', e);
		}
		historyLoading = false;
	}

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
			// Refresh history since the backend auto-saved
			loadHistory();
		} catch (e) {
			console.error('ICP analysis failed:', e);
			alert(`Analysis failed: ${e}`);
		}
		loading = false;
	}

	async function handleDelete(id: string) {
		if (!confirm('Delete this analysis?')) return;
		try {
			await deleteICPAnalysis(id);
			savedAnalyses = savedAnalyses.filter(a => a.id !== id);
			if (expandedId === id) expandedId = null;
		} catch (e) {
			console.error('Delete failed:', e);
		}
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	function parseJSON(s: string): string[] {
		try { return JSON.parse(s); } catch { return []; }
	}

	function formatDate(iso: string): string {
		const d = new Date(iso);
		return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
	}

	function truncate(s: string, max: number): string {
		return s.length > max ? s.slice(0, max) + '...' : s;
	}

	async function handleGenerateCampaign(id: string) {
		icpActionId = id;
		icpActionType = 'campaign';
		try {
			const res = await icpGenerateCampaign(id, icpActionChannel);
			alert(`Campaign "${res.campaign.name}" created! View it in Campaigns.`);
		} catch (e) {
			alert(`Failed to generate campaign: ${e}`);
		}
		icpActionId = null;
		icpActionType = null;
	}

	function toggleCompare(id: string) {
		if (compareIds.includes(id)) {
			compareIds = compareIds.filter(x => x !== id);
		} else if (compareIds.length < 2) {
			compareIds = [...compareIds, id];
		}
	}

	function getComparison(): [SavedICPAnalysis, SavedICPAnalysis] | null {
		if (compareIds.length !== 2) return null;
		const a = savedAnalyses.find(s => s.id === compareIds[0]);
		const b = savedAnalyses.find(s => s.id === compareIds[1]);
		return a && b ? [a, b] : null;
	}

	async function handleToggleAutoRefresh() {
		autoRefreshSaving = true;
		try {
			autoRefresh = !autoRefresh;
			await putICPSettings({ icp_auto_refresh: autoRefresh });
		} catch { autoRefresh = !autoRefresh; }
		autoRefreshSaving = false;
	}

	async function handleCreateScoringRules(id: string) {
		icpActionId = id;
		icpActionType = 'rules';
		try {
			const res = await icpCreateScoringRules(id);
			alert(`Created ${res.created} scoring rule${res.created !== 1 ? 's' : ''} from ICP conversion pages.`);
		} catch (e) {
			alert(`Failed to create scoring rules: ${e}`);
		}
		icpActionId = null;
		icpActionType = null;
	}
</script>

<div class="p-6 space-y-6">
	<div>
		<h2 class="text-xl font-semibold">ICP Discovery</h2>
		<p class="text-sm text-muted-foreground">Identify your Ideal Customer Profile by analyzing users who visit conversion pages</p>
	</div>

	<!-- Tabs -->
	<div class="flex gap-1 border-b border-border">
		<button
			onclick={() => activeTab = 'analyze'}
			class="px-4 py-2 text-sm font-medium border-b-2 transition-colors {activeTab === 'analyze'
				? 'border-primary text-primary'
				: 'border-transparent text-muted-foreground hover:text-foreground'}"
		>
			Analyze
		</button>
		<button
			onclick={() => activeTab = 'history'}
			class="px-4 py-2 text-sm font-medium border-b-2 transition-colors {activeTab === 'history'
				? 'border-primary text-primary'
				: 'border-transparent text-muted-foreground hover:text-foreground'}"
		>
			History
			{#if savedAnalyses.length > 0}
				<span class="ml-1.5 text-xs bg-muted px-1.5 py-0.5 rounded-full">{savedAnalyses.length}</span>
			{/if}
		</button>
	</div>

	{#if activeTab === 'analyze'}
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
	{:else}
		<!-- History Tab -->
		{#if historyLoading}
			<p class="text-sm text-muted-foreground">Loading history...</p>
		{:else if savedAnalyses.length === 0}
			<div class="border border-border rounded-lg p-8 bg-card text-center">
				<p class="text-sm text-muted-foreground">No saved analyses yet. Run an analysis from the Analyze tab to get started.</p>
			</div>
		{:else}
			<!-- Controls: compare mode + auto-refresh -->
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-3">
					<button
						onclick={() => { compareMode = !compareMode; compareIds = []; }}
						class="px-3 py-1.5 text-sm border border-border rounded-md hover:bg-muted {compareMode ? 'bg-primary/10 border-primary text-primary' : ''}"
					>{compareMode ? 'Exit Compare' : 'Compare'}</button>
					{#if compareMode}
						<span class="text-xs text-muted-foreground">Select 2 analyses to compare side-by-side</span>
					{/if}
				</div>
				<label class="flex items-center gap-2 text-sm cursor-pointer">
					<span class="text-muted-foreground">Auto-refresh weekly</span>
					<button
						onclick={handleToggleAutoRefresh}
						disabled={autoRefreshSaving}
						class="w-8 h-5 rounded-full transition-colors disabled:opacity-50 {autoRefresh ? 'bg-primary' : 'bg-muted-foreground/30'}"
					>
						<div class="w-4 h-4 rounded-full bg-white shadow-sm transform transition-transform {autoRefresh ? 'translate-x-3.5' : 'translate-x-0.5'}"></div>
					</button>
				</label>
			</div>

			<!-- Comparison view -->
			{#if compareMode && compareIds.length === 2}
				{@const pair = getComparison()}
				{#if pair}
					{@const [a, b] = pair}
					{@const aTraits = parseJSON(a.traits)}
					{@const bTraits = parseJSON(b.traits)}
					{@const aChannels = parseJSON(a.channels)}
					{@const bChannels = parseJSON(b.channels)}
					{@const aRecs = parseJSON(a.recommendations)}
					{@const bRecs = parseJSON(b.recommendations)}
					<div class="border border-primary/30 rounded-lg overflow-hidden">
						<div class="grid grid-cols-2 divide-x divide-border">
							{#each [[a, aTraits, aChannels, aRecs], [b, bTraits, bChannels, bRecs]] as [sa, traits, channels, recs]}
								{@const sav = sa as SavedICPAnalysis}
								<div class="p-4 space-y-3">
									<div class="flex items-center gap-2">
										<span class="text-xs text-muted-foreground">{formatDate(sav.created_at)}</span>
										<span class="text-xs bg-muted px-1.5 py-0.5 rounded">{sav.profile_count} profiles</span>
									</div>
									<p class="text-sm">{sav.summary}</p>
									<div>
										<h4 class="text-xs font-medium text-muted-foreground mb-1">Traits</h4>
										<ul class="space-y-0.5">
											{#each traits as t}
												<li class="text-xs text-muted-foreground flex gap-1.5"><span class="text-primary">-</span>{t}</li>
											{/each}
										</ul>
									</div>
									<div>
										<h4 class="text-xs font-medium text-muted-foreground mb-1">Channels</h4>
										<div class="flex flex-wrap gap-1">
											{#each channels as ch}
												<span class="text-xs px-1.5 py-0.5 rounded bg-primary/10 text-primary">{ch}</span>
											{/each}
										</div>
									</div>
								</div>
							{/each}
						</div>
					</div>
				{/if}
			{/if}

			<div class="space-y-3">
				{#each savedAnalyses as sa}
					{@const convPages = parseJSON(sa.conversion_pages)}
					{@const traits = parseJSON(sa.traits)}
					{@const channels = parseJSON(sa.channels)}
					{@const recs = parseJSON(sa.recommendations)}
					{@const isExpanded = expandedId === sa.id}
					<div class="border border-border rounded-lg bg-card">
						<div class="flex items-center px-4 pt-3 pb-0">
						{#if compareMode}
							<input
								type="checkbox"
								checked={compareIds.includes(sa.id)}
								disabled={!compareIds.includes(sa.id) && compareIds.length >= 2}
								onchange={() => toggleCompare(sa.id)}
								class="mr-3 w-4 h-4 rounded accent-primary"
							/>
						{/if}
					</div>
					<button onclick={() => toggleExpand(sa.id)} class="w-full px-4 py-3 text-left flex items-center justify-between gap-4">
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-2 mb-1">
									<span class="text-xs text-muted-foreground">{formatDate(sa.created_at)}</span>
									<span class="text-xs bg-muted px-1.5 py-0.5 rounded">{sa.profile_count} profiles</span>
								</div>
								<p class="text-sm truncate">{truncate(sa.summary, 120)}</p>
								<div class="flex flex-wrap gap-1 mt-1.5">
									{#each convPages as cp}
										<span class="text-xs px-1.5 py-0.5 rounded bg-muted text-muted-foreground">{cp}</span>
									{/each}
								</div>
							</div>
							<span class="text-xs text-muted-foreground shrink-0">{isExpanded ? 'Collapse' : 'Expand'}</span>
						</button>

						{#if isExpanded}
							<div class="border-t border-border px-4 py-4 space-y-4">
								<div>
									<h4 class="text-sm font-medium mb-1">Summary</h4>
									<p class="text-sm text-muted-foreground">{sa.summary}</p>
								</div>

								<div class="grid grid-cols-3 gap-4">
									<div>
										<h4 class="text-sm font-medium mb-1">Common Traits</h4>
										<ul class="space-y-1">
											{#each traits as trait}
												<li class="text-sm text-muted-foreground flex items-start gap-1.5">
													<span class="text-primary mt-0.5 shrink-0">-</span>
													{trait}
												</li>
											{/each}
										</ul>
									</div>

									<div>
										<h4 class="text-sm font-medium mb-1">Best Channels</h4>
										<div class="flex flex-wrap gap-1.5">
											{#each channels as ch}
												<span class="text-xs px-2 py-1 rounded-full bg-primary/10 text-primary font-medium">{ch}</span>
											{/each}
										</div>
									</div>

									<div>
										<h4 class="text-sm font-medium mb-1">Recommendations</h4>
										<ul class="space-y-1">
											{#each recs as rec}
												<li class="text-sm text-muted-foreground flex items-start gap-1.5">
													<span class="text-primary mt-0.5 shrink-0">-</span>
													{rec}
												</li>
											{/each}
										</ul>
									</div>
								</div>

								<div class="flex items-center justify-between pt-2 border-t border-border">
									<div class="flex items-center gap-2">
										<Select
											bind:value={icpActionChannel}
											options={icpChannels.map(ch => ({ value: ch, label: ch }))}
											size="sm"
											fullWidth={false}
										/>
										<button
											onclick={() => handleGenerateCampaign(sa.id)}
											disabled={icpActionId === sa.id}
											class="px-3 py-1 text-xs bg-primary text-primary-foreground rounded hover:opacity-90 disabled:opacity-50"
										>
											{icpActionId === sa.id && icpActionType === 'campaign' ? 'Generating...' : 'Generate Campaign'}
										</button>
										<button
											onclick={() => handleCreateScoringRules(sa.id)}
											disabled={icpActionId === sa.id}
											class="px-3 py-1 text-xs border border-border rounded hover:bg-muted disabled:opacity-50"
										>
											{icpActionId === sa.id && icpActionType === 'rules' ? 'Creating...' : 'Add as Scoring Rules'}
										</button>
									</div>
									<button
										onclick={() => handleDelete(sa.id)}
										class="px-3 py-1 text-xs text-destructive border border-destructive/30 rounded hover:bg-destructive/10 transition-colors"
									>
										Delete
									</button>
								</div>
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>
