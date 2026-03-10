<script lang="ts">
	import { onMount } from 'svelte';
	import { listCampaigns, generateCampaign, deleteCampaign, createABTest, getABResults } from '$lib/api';
	import { formatTime, relativeTime } from '$lib/utils';
	import type { Campaign, CampaignContent, ABVariation } from '$lib/types';

	let campaigns = $state<Campaign[]>([]);
	let loading = $state(true);
	let generating = $state(false);
	let showGenerate = $state(false);

	let genChannel = $state('reddit');
	let genTopic = $state('');

	let expandedId = $state<string | null>(null);
	let abResults = $state<ABVariation[]>([]);
	let abLoading = $state(false);

	const channels = [
		{ value: 'reddit', label: 'Reddit' },
		{ value: 'linkedin', label: 'LinkedIn' },
		{ value: 'twitter', label: 'Twitter/X' },
		{ value: 'youtube', label: 'YouTube' },
		{ value: 'blog', label: 'Blog' },
	];

	const statusColors: Record<string, string> = {
		draft: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400',
		published: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
		archived: 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400',
	};

	onMount(() => loadCampaigns());

	async function loadCampaigns() {
		loading = true;
		try {
			const res = await listCampaigns();
			campaigns = res.campaigns ?? [];
		} catch (e) {
			console.error('Failed to load campaigns:', e);
		}
		loading = false;
	}

	async function handleGenerate() {
		if (!genTopic) return;
		generating = true;
		try {
			await generateCampaign(genChannel, genTopic);
			showGenerate = false;
			genTopic = '';
			await loadCampaigns();
		} catch (e) {
			console.error('Failed to generate campaign:', e);
			alert(`Generation failed: ${e}`);
		}
		generating = false;
	}

	async function handleDelete(id: string) {
		await deleteCampaign(id);
		loadCampaigns();
	}

	function parseContent(content: string): CampaignContent | null {
		try {
			return JSON.parse(content);
		} catch {
			return null;
		}
	}

	function copyContent(campaign: Campaign) {
		const content = parseContent(campaign.content);
		if (!content) return;
		const text = [content.title, '', content.body, '', content.url, '', (content.tags ?? []).map(t => '#' + t).join(' ')].filter(Boolean).join('\n');
		navigator.clipboard.writeText(text);
	}

	async function handleABTest(id: string) {
		abLoading = true;
		try {
			await createABTest(id);
			await loadCampaigns();
		} catch (e) {
			alert(`A/B test failed: ${e}`);
		}
		abLoading = false;
	}

	async function toggleExpand(id: string) {
		if (expandedId === id) {
			expandedId = null;
			return;
		}
		expandedId = id;
		try {
			const res = await getABResults(id);
			abResults = res.variations ?? [];
		} catch {
			abResults = [];
		}
	}
</script>

<div class="p-6 space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-xl font-semibold">AI Campaigns</h2>
			<p class="text-sm text-muted-foreground">Generate channel-specific content with AI, track with ref codes</p>
		</div>
		<button onclick={() => showGenerate = !showGenerate} class="px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90">
			{showGenerate ? 'Cancel' : 'Generate Campaign'}
		</button>
	</div>

	{#if showGenerate}
		<div class="border border-border rounded-lg p-4 space-y-3 bg-card">
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label class="text-xs font-medium text-muted-foreground">Channel</label>
					<select bind:value={genChannel} class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background">
						{#each channels as ch}
							<option value={ch.value}>{ch.label}</option>
						{/each}
					</select>
				</div>
				<div>
					<label class="text-xs font-medium text-muted-foreground">Topic / Angle</label>
					<input bind:value={genTopic} placeholder="e.g. How we solved X problem" class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
				</div>
			</div>
			<p class="text-xs text-muted-foreground">AI will use your project description, top pages, and event data to generate relevant content.</p>
			<button onclick={handleGenerate} disabled={generating || !genTopic} class="px-4 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90 disabled:opacity-50">
				{generating ? 'Generating...' : 'Generate with AI'}
			</button>
		</div>
	{/if}

	{#if loading}
		<div class="text-sm text-muted-foreground py-8 text-center">Loading campaigns...</div>
	{:else if campaigns.length === 0}
		<div class="text-center py-12 text-muted-foreground">
			<p class="text-lg font-medium">No campaigns yet</p>
			<p class="text-sm mt-1">Generate your first AI-powered campaign to start driving growth.</p>
		</div>
	{:else}
		<div class="space-y-3">
			{#each campaigns as campaign}
				{@const content = parseContent(campaign.content)}
				<div class="border border-border rounded-lg bg-card">
					<div class="px-4 py-3 flex items-center justify-between">
						<div class="flex items-center gap-3">
							<div>
								<div class="flex items-center gap-2">
									<span class="font-medium text-sm">{campaign.name || 'Untitled'}</span>
									<span class="text-xs px-1.5 py-0.5 rounded {statusColors[campaign.status] ?? statusColors.draft}">{campaign.status}</span>
									<span class="text-xs px-1.5 py-0.5 rounded bg-muted text-muted-foreground">{campaign.channel}</span>
								</div>
								<p class="text-xs text-muted-foreground mt-0.5">{relativeTime(campaign.created_at)}</p>
							</div>
						</div>
						<div class="flex items-center gap-2">
							<button onclick={() => copyContent(campaign)} class="px-2 py-1 text-xs border border-border rounded hover:bg-muted">Copy</button>
							<button onclick={() => handleABTest(campaign.id)} disabled={abLoading} class="px-2 py-1 text-xs border border-border rounded hover:bg-muted">A/B Test</button>
							<button onclick={() => toggleExpand(campaign.id)} class="px-2 py-1 text-xs border border-border rounded hover:bg-muted">
								{expandedId === campaign.id ? 'Collapse' : 'Details'}
							</button>
							<button onclick={() => handleDelete(campaign.id)} class="text-xs text-muted-foreground hover:text-red-500">Delete</button>
						</div>
					</div>

					{#if expandedId === campaign.id && content}
						<div class="border-t border-border px-4 py-3 space-y-3">
							{#if content.title}
								<div>
									<label class="text-xs font-medium text-muted-foreground">Title</label>
									<p class="text-sm mt-0.5 font-medium">{content.title}</p>
								</div>
							{/if}
							{#if content.body}
								<div>
									<label class="text-xs font-medium text-muted-foreground">Body</label>
									<pre class="text-sm mt-0.5 whitespace-pre-wrap font-sans bg-muted/50 rounded p-3">{content.body}</pre>
								</div>
							{/if}
							{#if content.url}
								<div>
									<label class="text-xs font-medium text-muted-foreground">URL</label>
									<p class="text-sm mt-0.5 font-mono">{content.url}</p>
								</div>
							{/if}
							{#if content.tags && content.tags.length > 0}
								<div>
									<label class="text-xs font-medium text-muted-foreground">Tags</label>
									<div class="flex gap-1 mt-0.5">
										{#each content.tags as tag}
											<span class="text-xs px-1.5 py-0.5 rounded bg-muted text-muted-foreground">#{tag}</span>
										{/each}
									</div>
								</div>
							{/if}

							{#if abResults.length > 0}
								<div>
									<label class="text-xs font-medium text-muted-foreground">A/B Test Results</label>
									<div class="border border-border rounded-lg overflow-hidden mt-1">
										<table class="w-full text-sm">
											<thead>
												<tr class="bg-muted/50 border-b border-border">
													<th class="text-left px-3 py-2 font-medium">Variation</th>
													<th class="text-right px-3 py-2 font-medium">Impressions</th>
													<th class="text-right px-3 py-2 font-medium">Conversions</th>
													<th class="text-right px-3 py-2 font-medium">Rate</th>
												</tr>
											</thead>
											<tbody>
												{#each abResults as v, i}
													{@const best = abResults.reduce((a, b) => a.conversion_rate > b.conversion_rate ? a : b)}
													<tr class="border-b border-border {v === best ? 'bg-green-50 dark:bg-green-900/10' : ''}">
														<td class="px-3 py-2 font-mono text-xs">{v.flag_key}</td>
														<td class="px-3 py-2 text-right">{v.impressions}</td>
														<td class="px-3 py-2 text-right">{v.conversions}</td>
														<td class="px-3 py-2 text-right font-medium">{v.conversion_rate.toFixed(1)}%</td>
													</tr>
												{/each}
											</tbody>
										</table>
									</div>
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
