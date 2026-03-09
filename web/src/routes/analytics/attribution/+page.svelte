<script lang="ts">
	import { onMount } from 'svelte';
	import { getAttribution, getAttributionSources, listRefCodes, createRefCode, deleteRefCode } from '$lib/api';
	import { exportCSV } from '$lib/csv';
	import type { AttributionSource, ChannelSummary, RefCode } from '$lib/types';
	import Chart from '$lib/components/ui/Chart.svelte';
	import { getCssColor, baseDoughnutOptions, type ChartConfiguration } from '$lib/chart-config';
	import AiInsight from '$lib/components/ui/AiInsight.svelte';

	let activeTab = $state<'sources' | 'refcodes'>('sources');

	// Sources state
	let channels = $state<ChannelSummary[]>([]);
	let sources = $state<AttributionSource[]>([]);
	let loading = $state(true);
	let range = $state('30d');

	// Ref codes state
	let refCodes = $state<RefCode[]>([]);
	let refLoading = $state(false);
	let newCode = $state('');
	let newName = $state('');
	let newNotes = $state('');
	let creating = $state(false);
	let copied = $state<string | null>(null);

	const CHANNEL_COLORS = [
		'hsl(217 91% 60%)',
		'hsl(142 71% 45%)',
		'hsl(263 70% 50%)',
		'hsl(25 95% 53%)',
		'hsl(339 90% 51%)',
		'hsl(173 80% 40%)',
		'hsl(48 96% 53%)',
		'hsl(220 9% 46%)',
	];

	onMount(() => {
		loadSources();
		loadRefCodes();
	});

	function getDateRange() {
		const end = new Date();
		const ms: Record<string, number> = { '7d': 7, '30d': 30, '90d': 90 };
		const days = ms[range] ?? 30;
		return { start: new Date(end.getTime() - days * 86400000), end };
	}

	async function loadSources() {
		loading = true;
		try {
			const { start, end } = getDateRange();
			const params = { start: start.toISOString(), end: end.toISOString() };
			const [chRes, srcRes] = await Promise.all([
				getAttribution(params),
				getAttributionSources({ ...params, limit: '100' }),
			]);
			channels = chRes.channels ?? [];
			sources = srcRes.sources ?? [];
		} catch (e) {
			console.error('Failed to load attribution:', e);
		}
		loading = false;
	}

	async function loadRefCodes() {
		refLoading = true;
		try {
			const res = await listRefCodes();
			refCodes = res.ref_codes ?? [];
		} catch (e) {
			console.error('Failed to load ref codes:', e);
		}
		refLoading = false;
	}

	async function handleCreateRefCode() {
		if (!newCode.trim() || !newName.trim()) return;
		creating = true;
		try {
			await createRefCode(newCode.trim(), newName.trim(), newNotes.trim());
			newCode = '';
			newName = '';
			newNotes = '';
			await loadRefCodes();
		} catch (e) {
			console.error('Failed to create ref code:', e);
		}
		creating = false;
	}

	async function handleDeleteRefCode(id: string) {
		try {
			await deleteRefCode(id);
			await loadRefCodes();
		} catch (e) {
			console.error('Failed to delete ref code:', e);
		}
	}

	function copyUrl(code: string) {
		const url = `${window.location.origin}?ref=${encodeURIComponent(code)}`;
		navigator.clipboard.writeText(url);
		copied = code;
		setTimeout(() => { if (copied === code) copied = null; }, 2000);
	}

	let totalSessions = $derived(channels.reduce((s, c) => s + c.sessions, 0));
	let totalUsers = $derived(channels.reduce((s, c) => s + c.users, 0));

	function bounceRate(bounced: number, sessions: number): string {
		if (sessions === 0) return '0%';
		return Math.round((bounced / sessions) * 100) + '%';
	}

	let attrPrompt = $derived(() => {
		if (channels.length === 0) return '';
		const chSummary = channels.map(c => {
			const pct = totalSessions > 0 ? Math.round((c.sessions / totalSessions) * 100) : 0;
			return `${c.channel}: ${c.sessions} sessions (${pct}%), ${c.users} users, ${c.sessions > 0 ? Math.round((c.bounced / c.sessions) * 100) : 0}% bounce, ${c.avg_pages.toFixed(1)} avg pages`;
		}).join('\n');
		const topSrc = sources.slice(0, 5).map(s => `${s.source} (${s.channel}): ${s.sessions} sessions`).join('\n');
		return `Analyze this traffic attribution data (${range} window). Which channels perform best? What should I double down on or improve? Be brief — 3-4 sentences.\n\nChannels:\n${chSummary}\n\nTop sources:\n${topSrc}`;
	});

	let attrReady = $derived(!loading && channels.length > 0);

	let doughnutConfig = $derived<ChartConfiguration>({
		type: 'doughnut',
		data: {
			labels: channels.map(c => c.channel),
			datasets: [{
				data: channels.map(c => c.sessions),
				backgroundColor: channels.map((_, i) => CHANNEL_COLORS[i % CHANNEL_COLORS.length]),
				borderWidth: 0,
			}],
		},
		options: {
			...baseDoughnutOptions(),
			plugins: {
				...baseDoughnutOptions().plugins,
				tooltip: {
					...baseDoughnutOptions().plugins?.tooltip,
					callbacks: {
						label: (ctx: any) => {
							const val = ctx.parsed;
							const pct = totalSessions > 0 ? Math.round((val / totalSessions) * 100) : 0;
							return ` ${ctx.label}: ${val.toLocaleString()} sessions (${pct}%)`;
						},
					},
				},
			},
		},
	});
</script>

<div class="p-6 max-w-6xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Attribution</h2>
			<p class="text-sm text-muted-foreground mt-1">Traffic sources and referral tracking</p>
		</div>
		<div class="flex gap-2 items-center">
			{#each [['sources', 'Sources'], ['refcodes', 'Ref Codes']] as [value, label]}
				<button
					onclick={() => activeTab = value as 'sources' | 'refcodes'}
					class="px-3 py-1.5 text-xs rounded border transition-colors {activeTab === value
						? 'bg-primary text-primary-foreground border-primary'
						: 'border-border hover:bg-accent'}"
				>{label}</button>
			{/each}
		</div>
	</div>

	{#if activeTab === 'sources'}
		<!-- Date range + CSV export -->
		<div class="flex gap-2 items-center justify-end mb-4">
			<button
				onclick={() => exportCSV(sources as any, 'attribution-sources.csv')}
				disabled={sources.length === 0}
				class="px-2 py-1 text-xs rounded border border-border hover:bg-accent disabled:opacity-40 transition-colors"
			>Export CSV</button>
			<div class="flex gap-1">
				{#each [['7d', '7D'], ['30d', '30D'], ['90d', '90D']] as [value, label]}
					<button
						onclick={() => { range = value; loadSources(); }}
						class="px-2 py-1 text-xs rounded border transition-colors {range === value
							? 'bg-primary text-primary-foreground border-primary'
							: 'border-border hover:bg-accent'}"
					>{label}</button>
				{/each}
			</div>
		</div>

		{#if loading}
			<div class="flex items-center justify-center h-64">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
			</div>
		{:else if channels.length === 0}
			<div class="border border-border rounded-lg p-12 bg-card text-center">
				<p class="text-muted-foreground text-sm">No attribution data in this period.</p>
			</div>
		{:else}
			<AiInsight cacheKey="attribution_{range}" prompt={attrPrompt()} ready={attrReady} />

			<!-- Stat cards -->
			<div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
				{#each channels as ch, i}
					{@const pct = totalSessions > 0 ? Math.round((ch.sessions / totalSessions) * 100) : 0}
					<div class="border border-border rounded-lg p-4 bg-card">
						<div class="flex items-center gap-2 mb-1">
							<div class="w-2 h-2 rounded-full" style="background: {CHANNEL_COLORS[i % CHANNEL_COLORS.length]}"></div>
							<p class="text-xs text-muted-foreground">{ch.channel}</p>
						</div>
						<p class="text-2xl font-bold">{ch.sessions.toLocaleString()}</p>
						<p class="text-xs text-muted-foreground">{pct}% of sessions</p>
					</div>
				{/each}
			</div>

			<!-- Chart + Summary -->
			<div class="grid grid-cols-1 md:grid-cols-2 gap-6 mb-6">
				<div class="border border-border rounded-lg p-4 bg-card">
					<h3 class="text-sm font-medium mb-3">Sessions by Channel</h3>
					<Chart config={doughnutConfig} class="h-56" />
				</div>
				<div class="border border-border rounded-lg p-4 bg-card">
					<h3 class="text-sm font-medium mb-3">Channel Summary</h3>
					<div class="space-y-3">
						{#each channels as ch, i}
							{@const pct = totalSessions > 0 ? Math.round((ch.sessions / totalSessions) * 100) : 0}
							<div>
								<div class="flex items-center justify-between text-sm mb-1">
									<div class="flex items-center gap-2">
										<div class="w-2 h-2 rounded-full" style="background: {CHANNEL_COLORS[i % CHANNEL_COLORS.length]}"></div>
										<span>{ch.channel}</span>
									</div>
									<span class="tabular-nums text-muted-foreground">{ch.sessions.toLocaleString()}</span>
								</div>
								<div class="w-full bg-muted rounded-full h-1.5 overflow-hidden">
									<div class="h-full rounded-full" style="width: {pct}%; background: {CHANNEL_COLORS[i % CHANNEL_COLORS.length]}"></div>
								</div>
							</div>
						{/each}
					</div>
				</div>
			</div>

			<!-- Sources table -->
			<div class="border border-border rounded-lg bg-card overflow-hidden">
				<div class="px-4 py-3 border-b border-border">
					<h3 class="text-sm font-medium">All Sources ({sources.length})</h3>
				</div>
				<div class="overflow-x-auto">
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-border bg-muted/30">
								<th class="px-4 py-2 text-left text-xs font-medium text-muted-foreground">Source</th>
								<th class="px-4 py-2 text-left text-xs font-medium text-muted-foreground">Channel</th>
								<th class="px-4 py-2 text-right text-xs font-medium text-muted-foreground">Sessions</th>
								<th class="px-4 py-2 text-right text-xs font-medium text-muted-foreground">Users</th>
								<th class="px-4 py-2 text-right text-xs font-medium text-muted-foreground">Bounce Rate</th>
								<th class="px-4 py-2 text-right text-xs font-medium text-muted-foreground">Avg Pages</th>
							</tr>
						</thead>
						<tbody>
							{#each sources as src}
								<tr class="border-b border-border/50 hover:bg-accent/30 transition-colors">
									<td class="px-4 py-2.5 font-mono text-xs max-w-xs truncate">{src.source}</td>
									<td class="px-4 py-2.5">
										<span class="px-1.5 py-0.5 text-xs rounded bg-muted">{src.channel}</span>
									</td>
									<td class="px-4 py-2.5 text-right tabular-nums">{src.sessions.toLocaleString()}</td>
									<td class="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{src.users.toLocaleString()}</td>
									<td class="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{bounceRate(src.bounced, src.sessions)}</td>
									<td class="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{src.avg_pages.toFixed(1)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		{/if}

	{:else}
		<!-- Ref Codes tab -->
		<div class="border border-border rounded-lg p-4 bg-card mb-6">
			<h3 class="text-sm font-medium mb-3">Create Ref Code</h3>
			<form onsubmit={(e) => { e.preventDefault(); handleCreateRefCode(); }} class="flex flex-wrap gap-3 items-end">
				<div class="flex-1 min-w-[140px]">
					<label for="rc-name" class="block text-xs text-muted-foreground mb-1">Name</label>
					<input
						id="rc-name"
						bind:value={newName}
						placeholder="Reddit Launch Post"
						class="w-full px-2.5 py-1.5 text-sm rounded border border-border bg-background focus:outline-none focus:ring-1 focus:ring-ring"
					/>
				</div>
				<div class="flex-1 min-w-[120px]">
					<label for="rc-code" class="block text-xs text-muted-foreground mb-1">Code slug</label>
					<input
						id="rc-code"
						bind:value={newCode}
						placeholder="reddit-launch"
						class="w-full px-2.5 py-1.5 text-sm rounded border border-border bg-background focus:outline-none focus:ring-1 focus:ring-ring font-mono"
					/>
				</div>
				<div class="flex-1 min-w-[140px]">
					<label for="rc-notes" class="block text-xs text-muted-foreground mb-1">Notes (optional)</label>
					<input
						id="rc-notes"
						bind:value={newNotes}
						placeholder="Posted on r/webdev"
						class="w-full px-2.5 py-1.5 text-sm rounded border border-border bg-background focus:outline-none focus:ring-1 focus:ring-ring"
					/>
				</div>
				<button
					type="submit"
					disabled={creating || !newCode.trim() || !newName.trim()}
					class="px-4 py-1.5 text-sm rounded bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-40 transition-colors"
				>{creating ? 'Creating...' : 'Create'}</button>
			</form>
		</div>

		{#if refLoading}
			<div class="flex items-center justify-center h-32">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
			</div>
		{:else if refCodes.length === 0}
			<div class="border border-border rounded-lg p-12 bg-card text-center">
				<p class="text-muted-foreground text-sm">No ref codes yet. Create one above to start tracking.</p>
			</div>
		{:else}
			<div class="border border-border rounded-lg bg-card overflow-hidden">
				<div class="px-4 py-3 border-b border-border">
					<h3 class="text-sm font-medium">Ref Codes ({refCodes.length})</h3>
				</div>
				<div class="overflow-x-auto">
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-border bg-muted/30">
								<th class="px-4 py-2 text-left text-xs font-medium text-muted-foreground">Name</th>
								<th class="px-4 py-2 text-left text-xs font-medium text-muted-foreground">Code</th>
								<th class="px-4 py-2 text-left text-xs font-medium text-muted-foreground">Notes</th>
								<th class="px-4 py-2 text-left text-xs font-medium text-muted-foreground">Created</th>
								<th class="px-4 py-2 text-right text-xs font-medium text-muted-foreground">Actions</th>
							</tr>
						</thead>
						<tbody>
							{#each refCodes as rc}
								<tr class="border-b border-border/50 hover:bg-accent/30 transition-colors">
									<td class="px-4 py-2.5 font-medium">{rc.name}</td>
									<td class="px-4 py-2.5 font-mono text-xs">
										<span class="px-1.5 py-0.5 rounded bg-muted">{rc.code}</span>
									</td>
									<td class="px-4 py-2.5 text-muted-foreground text-xs max-w-xs truncate">{rc.notes || '-'}</td>
									<td class="px-4 py-2.5 text-muted-foreground text-xs tabular-nums">{new Date(rc.created_at).toLocaleDateString()}</td>
									<td class="px-4 py-2.5 text-right">
										<div class="flex gap-1 justify-end">
											<button
												onclick={() => copyUrl(rc.code)}
												class="px-2 py-0.5 text-xs rounded border border-border hover:bg-accent transition-colors"
											>{copied === rc.code ? 'Copied!' : 'Copy URL'}</button>
											<button
												onclick={() => handleDeleteRefCode(rc.id)}
												class="px-2 py-0.5 text-xs rounded border border-destructive/30 text-destructive hover:bg-destructive/10 transition-colors"
											>Delete</button>
										</div>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		{/if}
	{/if}
</div>
