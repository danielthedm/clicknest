<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { getCampaignPerformance, refreshCampaignEngagement } from '$lib/api';
	import { formatTime } from '$lib/utils';

	let campaignId = $derived($page.params.id);
	let loading = $state(true);
	let campaign = $state<any>(null);
	let refCode = $state('');
	let stats = $state<any>(null);
	let timeSeries = $state<any[]>([]);
	let posts = $state<any[]>([]);
	let channels = $state<{ channel: string; sessions: number; users: number }[]>([]);
	let conversionEvent = $state('');
	let conversionCount = $state<number | null>(null);
	let conversionRate = $state<number | null>(null);
	let refreshing = $state(false);
	let conversionLoading = $state(false);
	let campaignRevenue = $state<number | null>(null);
	let campaignROI = $state<number | null>(null);

	let maxTimeSeriesSessions = $derived(Math.max(...timeSeries.map((d: any) => d.sessions), 1));
	let maxChannelSessions = $derived(Math.max(...channels.map((c) => c.sessions), 1));

	const conversionPresets = ['signup', 'purchase', 'upgrade', 'pageview', 'submit'];

	onMount(() => {
		loadPerformance();
	});

	async function loadPerformance(params?: Record<string, string>) {
		loading = true;
		try {
			const res = await getCampaignPerformance(campaignId, params);
			campaign = res.campaign;
			refCode = res.ref_code ?? '';
			stats = res.stats ?? null;
			timeSeries = res.time_series ?? [];
			posts = res.posts ?? [];
			channels = res.channels ?? [];
			conversionCount = res.conversion_count ?? null;
			conversionRate = res.conversion_rate ?? null;
			campaignRevenue = res.revenue ?? null;
			campaignROI = res.roi ?? null;
		} catch (e) {
			console.error('Failed to load campaign performance:', e);
		}
		loading = false;
	}

	async function applyConversionEvent() {
		if (!conversionEvent.trim()) {
			conversionCount = null;
			conversionRate = null;
			await loadPerformance();
			return;
		}
		conversionLoading = true;
		try {
			const res = await getCampaignPerformance(campaignId, { conversion_event: conversionEvent.trim() });
			conversionCount = res.conversion_count ?? null;
			conversionRate = res.conversion_rate ?? null;
		} catch (e) {
			console.error('Conversion event query failed:', e);
		}
		conversionLoading = false;
	}

	function parseEngagement(json: string): any {
		try {
			return JSON.parse(json);
		} catch {
			return {};
		}
	}

	async function handleRefreshEngagement() {
		refreshing = true;
		try {
			await refreshCampaignEngagement(campaignId);
			await loadPerformance();
		} catch (e) {
			console.error('Refresh failed:', e);
		}
		refreshing = false;
	}

	function bounceRate(s: any): string {
		if (!s || s.sessions === 0) return '0%';
		return ((s.bounced / s.sessions) * 100).toFixed(1) + '%';
	}

	const statusColors: Record<string, string> = {
		draft: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400',
		published: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
		archived: 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400',
	};
</script>

<div class="p-6 space-y-6">
	{#if loading}
		<p class="text-sm text-muted-foreground">Loading...</p>
	{:else if !campaign}
		<p class="text-sm text-muted-foreground">Campaign not found.</p>
	{:else}
		<!-- Header -->
		<div class="flex items-center gap-3">
			<a href="/growth/campaigns" class="text-sm text-muted-foreground hover:text-foreground">&larr; Campaigns</a>
		</div>
		<div class="flex items-center justify-between gap-3">
			<div class="flex items-center gap-3">
				<h2 class="text-lg font-semibold">{campaign.name}</h2>
				<span class="text-xs px-1.5 py-0.5 rounded {statusColors[campaign.status] ?? ''}">
					{campaign.status}
				</span>
				<span class="text-xs text-muted-foreground">{campaign.channel}</span>
				{#if refCode}
					<code class="text-xs bg-muted px-1.5 py-0.5 rounded">?ref={refCode}</code>
				{/if}
			</div>
			{#if posts.length > 0}
				<button
					onclick={handleRefreshEngagement}
					disabled={refreshing}
					class="text-xs px-3 py-1.5 border border-border rounded hover:bg-muted disabled:opacity-50"
				>{refreshing ? 'Refreshing...' : 'Refresh Engagement'}</button>
			{/if}
		</div>

		<!-- Stats cards -->
		{#if stats}
			<div class="grid grid-cols-2 md:grid-cols-5 gap-4">
				<div class="border border-border rounded-lg p-4 bg-card">
					<p class="text-xs text-muted-foreground">Sessions</p>
					<p class="text-2xl font-semibold">{stats.sessions.toLocaleString()}</p>
				</div>
				<div class="border border-border rounded-lg p-4 bg-card">
					<p class="text-xs text-muted-foreground">Unique Users</p>
					<p class="text-2xl font-semibold">{stats.users.toLocaleString()}</p>
				</div>
				<div class="border border-border rounded-lg p-4 bg-card">
					<p class="text-xs text-muted-foreground">Bounce Rate</p>
					<p class="text-2xl font-semibold">{bounceRate(stats)}</p>
				</div>
				<div class="border border-border rounded-lg p-4 bg-card">
					<p class="text-xs text-muted-foreground">Avg Pages</p>
					<p class="text-2xl font-semibold">{stats.avg_pages.toFixed(1)}</p>
				</div>
				<div class="border border-border rounded-lg p-4 bg-card">
					<p class="text-xs text-muted-foreground">Total Events</p>
					<p class="text-2xl font-semibold">{stats.event_count.toLocaleString()}</p>
				</div>
			</div>
		{:else if !refCode}
			<div class="border border-border rounded-lg p-6 bg-card text-center">
				<p class="text-sm text-muted-foreground">No ref code associated with this campaign. Generate the campaign with AI to automatically create a tracking ref code.</p>
			</div>
		{:else}
			<div class="border border-border rounded-lg p-6 bg-card text-center">
				<p class="text-sm text-muted-foreground">No traffic recorded for this campaign yet.</p>
			</div>
		{/if}

		<!-- Cost & ROI -->
		{#if stats}
			<div class="grid grid-cols-2 md:grid-cols-3 gap-4">
				<div class="border border-border rounded-lg p-4 bg-card">
					<p class="text-xs text-muted-foreground mb-1">Campaign Cost</p>
					<p class="text-lg font-semibold tabular-nums">${(campaign.cost ?? 0).toLocaleString(undefined, { minimumFractionDigits: 2 })}</p>
				</div>
				{#if campaignRevenue !== null}
					<div class="border border-border rounded-lg p-4 bg-card">
						<p class="text-xs text-muted-foreground mb-1">Revenue</p>
						<p class="text-lg font-semibold tabular-nums">${campaignRevenue.toLocaleString(undefined, { minimumFractionDigits: 2 })}</p>
					</div>
				{/if}
				{#if campaignROI !== null}
					<div class="border border-border rounded-lg p-4 bg-card">
						<p class="text-xs text-muted-foreground mb-1">ROI</p>
						<p class="text-lg font-semibold tabular-nums {campaignROI >= 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}">{campaignROI.toFixed(1)}%</p>
					</div>
				{/if}
			</div>
		{/if}

		<!-- Time series chart (simple bar representation) -->
		{#if timeSeries.length > 0}
			<div class="border border-border rounded-lg p-4 bg-card">
				<h3 class="text-sm font-medium mb-3">Daily Sessions</h3>
				<div class="flex items-end gap-1 h-32">
					{#each timeSeries as day}
						<div class="flex-1 flex flex-col items-center gap-1">
							<div
								class="w-full bg-primary rounded-t"
								style="height: {(day.sessions / maxTimeSeriesSessions) * 100}%"
								title="{day.date}: {day.sessions} sessions"
							></div>
						</div>
					{/each}
				</div>
				<div class="flex gap-1 mt-1">
					{#each timeSeries as day, i}
						{#if i === 0 || i === timeSeries.length - 1 || i === Math.floor(timeSeries.length / 2)}
							<span class="flex-1 text-center text-[10px] text-muted-foreground">{day.date.slice(5)}</span>
						{:else}
							<span class="flex-1"></span>
						{/if}
					{/each}
				</div>
			</div>
		{/if}

		<!-- Conversion event selector -->
		{#if refCode}
			<div class="border border-border rounded-lg p-4 bg-card">
				<h3 class="text-sm font-medium mb-3">Conversion Tracking</h3>
				<div class="flex items-center gap-2">
					<select
						bind:value={conversionEvent}
						class="text-sm border border-border rounded px-2 py-1.5 bg-background"
					>
						<option value="">— pick a conversion event —</option>
						{#each conversionPresets as p}
							<option value={p}>{p}</option>
						{/each}
					</select>
					<input
						bind:value={conversionEvent}
						placeholder="or type custom event name"
						class="flex-1 text-sm border border-border rounded px-2 py-1.5 bg-background"
					/>
					<button
						onclick={applyConversionEvent}
						disabled={conversionLoading}
						class="px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded hover:opacity-90 disabled:opacity-50"
					>{conversionLoading ? 'Calculating...' : 'Calculate'}</button>
				</div>
				{#if conversionCount !== null}
					<div class="mt-3 flex items-center gap-6">
						<div>
							<p class="text-xs text-muted-foreground">Conversions</p>
							<p class="text-2xl font-semibold">{conversionCount.toLocaleString()}</p>
						</div>
						<div>
							<p class="text-xs text-muted-foreground">Conversion Rate</p>
							<p class="text-2xl font-semibold">{conversionRate?.toFixed(1)}%</p>
						</div>
					</div>
				{/if}
			</div>
		{/if}

		<!-- Channel breakdown -->
		{#if channels.length > 0}
			<div class="border border-border rounded-lg p-4 bg-card">
				<h3 class="text-sm font-medium mb-3">Traffic Sources</h3>
				<div class="space-y-2">
					{#each channels as ch}
						<div class="flex items-center gap-3">
							<span class="text-xs text-muted-foreground w-20 shrink-0 capitalize">{ch.channel}</span>
							<div class="flex-1 bg-muted rounded-full h-1.5">
								<div
									class="bg-primary h-1.5 rounded-full"
									style="width: {(ch.sessions / maxChannelSessions) * 100}%"
								></div>
							</div>
							<span class="text-xs text-muted-foreground w-20 text-right shrink-0">{ch.sessions.toLocaleString()} sessions</span>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Published posts + engagement -->
		{#if posts.length > 0}
			<div class="space-y-3">
				<h3 class="text-sm font-medium">Published Posts</h3>
				{#each posts as post}
					{@const eng = parseEngagement(post.last_engagement)}
					<div class="border border-border rounded-lg p-4 bg-card">
						<div class="flex items-center justify-between mb-2">
							<div class="flex items-center gap-2">
								<span class="text-xs font-medium px-1.5 py-0.5 rounded bg-muted text-muted-foreground">{post.connector_name}</span>
								{#if post.external_url}
									<a href={post.external_url} target="_blank" rel="noopener" class="text-xs text-primary hover:underline">View post</a>
								{/if}
							</div>
							<span class="text-xs text-muted-foreground">{formatTime(post.posted_at)}</span>
						</div>
						{#if eng && (eng.views || eng.likes || eng.comments || eng.shares)}
							<div class="grid grid-cols-4 gap-4 mt-2">
								{#if eng.views !== undefined}
									<div>
										<p class="text-xs text-muted-foreground">Views</p>
										<p class="text-sm font-medium">{eng.views.toLocaleString()}</p>
									</div>
								{/if}
								<div>
									<p class="text-xs text-muted-foreground">Likes</p>
									<p class="text-sm font-medium">{(eng.likes ?? 0).toLocaleString()}</p>
								</div>
								<div>
									<p class="text-xs text-muted-foreground">Comments</p>
									<p class="text-sm font-medium">{(eng.comments ?? 0).toLocaleString()}</p>
								</div>
								<div>
									<p class="text-xs text-muted-foreground">Shares</p>
									<p class="text-sm font-medium">{(eng.shares ?? 0).toLocaleString()}</p>
								</div>
							</div>
							{#if post.last_fetched_at}
								<p class="text-[10px] text-muted-foreground mt-2">Last updated: {formatTime(post.last_fetched_at)}</p>
							{/if}
						{:else}
							<p class="text-xs text-muted-foreground mt-1">No engagement data yet.</p>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>
