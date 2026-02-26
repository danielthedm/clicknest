<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { getEvents, getTrends, getSessions, getPages, getNames, liveEvents, aiChat, getProject } from '$lib/api';
	import { eventDisplayName, relativeTime } from '$lib/utils';
	import type { Event, TrendPoint, Session, PageStat, EventName, ChatMessage, Project } from '$lib/types';
	import Chart from '$lib/components/ui/Chart.svelte';
	import { getCssColor, baseLineOptions, type ChartConfiguration } from '$lib/chart-config';

	let recentEvents = $state<Event[]>([]);
	let todayTrend = $state<TrendPoint[]>([]);
	let yesterdayTotal = $state(0);
	let sessions = $state<Session[]>([]);
	let totalSessions = $state(0);
	let topPages = $state<PageStat[]>([]);
	let eventNames = $state<EventName[]>([]);
	let project = $state<Project | null>(null);
	let loading = $state(true);
	let cleanup: (() => void) | null = null;
	let snippetCopied = $state(false);

	// AI chat state
	let chatHistory = $state<ChatMessage[]>([]);
	let chatInput = $state('');
	let chatLoading = $state(false);
	let chatError = $state('');
	let chatScrollEl = $state<HTMLDivElement | null>(null);
	let chatEnabled = $state(false); // Only true after first successful check

	onMount(() => {
		(async () => {
			const now = new Date();
			const todayStart = new Date(now);
			todayStart.setHours(0, 0, 0, 0);
			const yesterdayStart = new Date(todayStart.getTime() - 86400000);
			const weekAgo = new Date(now.getTime() - 7 * 86400000);

			try {
				const [eventsRes, todayRes, yesterdayRes, sessionsRes, pagesRes, namesRes, projRes] = await Promise.all([
					getEvents({ limit: '10' }),
					getTrends({ interval: 'hour', start: todayStart.toISOString(), end: now.toISOString() }),
					getTrends({ interval: 'day', start: yesterdayStart.toISOString(), end: todayStart.toISOString() }),
					getSessions({ limit: '5', start: weekAgo.toISOString(), end: now.toISOString() }),
					getPages({ start: weekAgo.toISOString(), end: now.toISOString(), limit: '5' }),
					getNames(),
					getProject(),
				]);

				recentEvents = eventsRes.events ?? [];
				todayTrend = todayRes.data ?? [];
				yesterdayTotal = (yesterdayRes.data ?? []).reduce((s, p) => s + p.count, 0);
				sessions = sessionsRes.sessions ?? [];
				totalSessions = sessionsRes.total;
				topPages = pagesRes.pages ?? [];
				eventNames = namesRes.names ?? [];
				project = projRes;
			} catch (e) {
				console.error('Failed to load dashboard:', e);
			}
			loading = false;

			cleanup = liveEvents((newEvents) => {
				recentEvents = [...newEvents, ...recentEvents].slice(0, 20);
				todayTotal += newEvents.length;
			});

			// Auto-load initial AI insight
			loadInitialInsight();
		})();

		return () => cleanup?.();
	});

	async function loadInitialInsight() {
		chatLoading = true;
		chatEnabled = true;
		try {
			const res = await aiChat(
				'Give me a quick overview of this product\'s analytics. What\'s working well and what needs attention? Be brief — 3-4 sentences max.',
				[]
			);
			chatHistory = [
				{ role: 'user', content: 'Analyze my analytics data' },
				{ role: 'assistant', content: res.reply },
			];
			chatError = '';
		} catch (e: any) {
			const msg = e.message || '';
			if (msg.includes('LLM not configured')) {
				chatEnabled = false; // Silently hide the chat panel
			} else {
				chatError = msg.replace('API error 500: ', '').replace(/^{"error":"/, '').replace(/"}$/, '');
			}
		}
		chatLoading = false;
		await tick();
		scrollChat();
	}

	async function sendMessage() {
		const msg = chatInput.trim();
		if (!msg || chatLoading) return;
		chatInput = '';

		// Only add previous assistant/user messages (not the initial auto-trigger)
		const historyToSend = chatHistory.slice(1); // skip the auto-trigger user message
		chatHistory = [...chatHistory, { role: 'user', content: msg }];
		chatLoading = true;
		chatError = '';
		await tick();
		scrollChat();

		try {
			const res = await aiChat(msg, historyToSend);
			chatHistory = [...chatHistory, { role: 'assistant', content: res.reply }];
		} catch (e: any) {
			chatError = (e.message || 'Request failed').replace('API error 500: ', '').replace(/^{"error":"/, '').replace(/"}$/, '');
		}
		chatLoading = false;
		await tick();
		scrollChat();
	}

	function scrollChat() {
		if (chatScrollEl) {
			chatScrollEl.scrollTop = chatScrollEl.scrollHeight;
		}
	}

	let todayTotal = $derived(todayTrend.reduce((s, p) => s + p.count, 0));

	let changeVsYesterday = $derived((() => {
		if (yesterdayTotal === 0) return null;
		return Math.round(((todayTotal - yesterdayTotal) / yesterdayTotal) * 100);
	})());

	let namedPct = $derived((() => {
		if (recentEvents.length === 0) return 0;
		return Math.round((recentEvents.filter(e => e.event_name).length / recentEvents.length) * 100);
	})());

	let areaConfig = $derived<ChartConfiguration>({
		type: 'line',
		data: {
			labels: todayTrend.map(p => {
				const d = new Date(p.bucket);
				return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
			}),
			datasets: [{
				data: todayTrend.map(p => p.count),
				borderColor: getCssColor('primary'),
				backgroundColor: getCssColor('primary', 0.15),
				fill: true,
				tension: 0.3,
				borderWidth: 2,
				pointRadius: 0,
				pointHoverRadius: 4,
			}],
		},
		options: {
			...baseLineOptions(),
			plugins: {
				...baseLineOptions().plugins,
				legend: { display: false },
				tooltip: { ...baseLineOptions().plugins?.tooltip, callbacks: { label: (ctx: any) => `${ctx.parsed.y} events` } },
			},
		},
	});

	let totalPageviews = $derived(topPages.reduce((s, p) => s + p.views, 0));

	let isEmpty = $derived(!loading && todayTotal === 0 && sessions.length === 0 && topPages.length === 0);

	function copySnippet() {
		if (!project) return;
		const snippet = `<script src="${window.location.origin}/sdk.js"\n  data-api-key="${project.api_key}"\n  data-host="${window.location.origin}"><\/script>`;
		navigator.clipboard.writeText(snippet);
		snippetCopied = true;
		setTimeout(() => { snippetCopied = false; }, 2000);
	}

	// Show AI chat panel only if enabled (LLM is configured)
	let showChat = $derived(chatEnabled || chatLoading);
</script>

<div class="p-6 max-w-6xl">
	<div class="mb-6">
		<h2 class="text-2xl font-bold tracking-tight">Overview</h2>
		<p class="text-sm text-muted-foreground mt-1">What's happening today</p>
	</div>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
		</div>
	{:else if isEmpty}
		<!-- Getting started / empty state -->
		<div class="max-w-xl">
			<div class="border border-border rounded-lg bg-card overflow-hidden">
				<div class="px-5 py-4 border-b border-border">
					<h3 class="text-sm font-semibold">Install the SDK</h3>
					<p class="text-xs text-muted-foreground mt-0.5">Add this snippet to your HTML <code class="font-mono">&lt;head&gt;</code> — events will appear automatically.</p>
				</div>
				<div class="p-5 space-y-4">
					{#if project}
						<div>
							<pre class="bg-muted rounded-md p-3 text-xs font-mono overflow-x-auto leading-relaxed">&lt;script src="{window.location.origin}/sdk.js"
  data-api-key="{project.api_key}"
  data-host="{window.location.origin}"&gt;&lt;/script&gt;</pre>
							<button
								onclick={copySnippet}
								class="mt-2 px-3 py-1.5 text-xs rounded border border-border hover:bg-accent transition-colors"
							>{snippetCopied ? '✓ Copied!' : 'Copy snippet'}</button>
						</div>
					{/if}
					<ol class="space-y-2 text-sm text-muted-foreground list-decimal list-inside">
						<li>Paste the snippet before <code class="font-mono text-xs">&lt;/head&gt;</code> on every page you want to track.</li>
						<li>Reload this page — pageviews and clicks will appear within seconds.</li>
						<li>Optionally call <code class="font-mono text-xs">ClickNest.identify('user-id')</code> to link events to users.</li>
					</ol>
					<p class="text-xs text-muted-foreground">
						Your API key and full docs are in <a href="/platform/settings" class="text-primary hover:underline">Settings</a>.
					</p>
				</div>
			</div>
		</div>
	{:else}
		<!-- Stats row -->
		<div class="grid grid-cols-4 gap-4 mb-6">
			<div class="border border-border rounded-lg p-4 bg-card">
				<p class="text-xs text-muted-foreground uppercase tracking-wide">Events Today</p>
				<p class="text-3xl font-bold mt-1">{todayTotal.toLocaleString()}</p>
				{#if changeVsYesterday !== null}
					<p class="text-xs mt-1 {changeVsYesterday >= 0 ? 'text-green-600' : 'text-red-500'}">
						{changeVsYesterday >= 0 ? '↑' : '↓'} {Math.abs(changeVsYesterday)}% vs yesterday
					</p>
				{/if}
			</div>
			<div class="border border-border rounded-lg p-4 bg-card">
				<p class="text-xs text-muted-foreground uppercase tracking-wide">Sessions (7d)</p>
				<p class="text-3xl font-bold mt-1">{totalSessions.toLocaleString()}</p>
			</div>
			<div class="border border-border rounded-lg p-4 bg-card">
				<p class="text-xs text-muted-foreground uppercase tracking-wide">Named Events</p>
				<p class="text-3xl font-bold mt-1">{eventNames.length.toLocaleString()}</p>
				<p class="text-xs text-muted-foreground mt-1">{namedPct}% of recent</p>
			</div>
			<div class="border border-border rounded-lg p-4 bg-card">
				<p class="text-xs text-muted-foreground uppercase tracking-wide">Pageviews (7d)</p>
				<p class="text-3xl font-bold mt-1">{totalPageviews.toLocaleString()}</p>
				<p class="text-xs text-muted-foreground mt-1">{topPages.length} unique pages</p>
			</div>
		</div>

		<!-- Main chart + right column -->
		<div class="grid grid-cols-3 gap-4 mb-6">
			<div class="col-span-2 border border-border rounded-lg p-4 bg-card">
				<div class="flex items-center justify-between mb-3">
					<h3 class="text-sm font-medium">Events Today (hourly)</h3>
					<a href="/analytics/trends" class="text-xs text-primary hover:underline">Full trends →</a>
				</div>
				{#if todayTrend.length === 0}
					<p class="text-sm text-muted-foreground py-8 text-center">No events today yet</p>
				{:else}
					<Chart config={areaConfig} class="h-48" />
				{/if}
			</div>

			<!-- Top pages mini -->
			<div class="border border-border rounded-lg bg-card overflow-hidden">
				<div class="px-4 py-3 border-b border-border flex items-center justify-between">
					<h3 class="text-sm font-medium">Top Pages (7d)</h3>
					<a href="/analytics/pages" class="text-xs text-primary hover:underline">All →</a>
				</div>
				{#if topPages.length === 0}
					<p class="text-sm text-muted-foreground p-4 text-center">No pageviews yet</p>
				{:else}
					<div class="divide-y divide-border">
						{#each topPages.slice(0, 5) as page}
							{@const maxViews = topPages[0].views}
							<div class="px-4 py-2.5">
								<div class="flex items-center justify-between mb-1">
									<span class="text-xs font-mono truncate flex-1 mr-2 text-muted-foreground">{page.path}</span>
									<span class="text-xs font-medium tabular-nums shrink-0">{page.views.toLocaleString()}</span>
								</div>
								<div class="w-full bg-muted rounded-full h-1 overflow-hidden">
									<div class="h-full bg-primary rounded-full" style="width: {Math.round((page.views/maxViews)*100)}%"></div>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>

		<!-- AI Insights chat panel (only shown when LLM is configured) -->
		{#if showChat}
			<div class="border border-border rounded-lg bg-card overflow-hidden mb-6">
				<div class="px-4 py-3 border-b border-border flex items-center gap-2">
					<div class="w-5 h-5 rounded-full bg-primary/10 flex items-center justify-center">
						<svg class="w-3 h-3 text-primary" fill="none" viewBox="0 0 24 24" stroke="currentColor">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.347.347a3.75 3.75 0 01-5.304 0l-.346-.347z" />
						</svg>
					</div>
					<h3 class="text-sm font-medium">AI Insights</h3>
					{#if chatLoading}
						<span class="text-xs text-muted-foreground animate-pulse ml-1">Analyzing...</span>
					{/if}
				</div>

				<!-- Chat messages -->
				<div bind:this={chatScrollEl} class="max-h-80 overflow-y-auto p-4 flex flex-col gap-3">
					{#each chatHistory as msg}
						{#if msg.role === 'assistant'}
							<div class="flex gap-2">
								<div class="w-6 h-6 rounded-full bg-primary/10 flex items-center justify-center shrink-0 mt-0.5">
									<svg class="w-3 h-3 text-primary" fill="none" viewBox="0 0 24 24" stroke="currentColor">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.347.347a3.75 3.75 0 01-5.304 0l-.346-.347z" />
									</svg>
								</div>
								<div class="flex-1">
									<p class="text-sm leading-relaxed whitespace-pre-wrap">{msg.content}</p>
								</div>
							</div>
						{:else if msg.content !== 'Analyze my analytics data'}
							<div class="flex justify-end">
								<div class="bg-primary text-primary-foreground rounded-lg px-3 py-2 max-w-xs">
									<p class="text-sm">{msg.content}</p>
								</div>
							</div>
						{/if}
					{/each}
					{#if chatLoading && chatHistory.length === 0}
						<div class="flex gap-2">
							<div class="w-6 h-6 rounded-full bg-primary/10 flex items-center justify-center shrink-0 mt-0.5">
								<svg class="w-3 h-3 text-primary" fill="none" viewBox="0 0 24 24" stroke="currentColor">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.347.347a3.75 3.75 0 01-5.304 0l-.346-.347z" />
								</svg>
							</div>
							<div class="flex gap-1 items-center py-1">
								<span class="w-1.5 h-1.5 bg-muted-foreground/50 rounded-full animate-bounce" style="animation-delay: 0ms"></span>
								<span class="w-1.5 h-1.5 bg-muted-foreground/50 rounded-full animate-bounce" style="animation-delay: 150ms"></span>
								<span class="w-1.5 h-1.5 bg-muted-foreground/50 rounded-full animate-bounce" style="animation-delay: 300ms"></span>
							</div>
						</div>
					{/if}
					{#if chatError}
						<p class="text-xs text-red-500">{chatError}</p>
					{/if}
				</div>

				<!-- Chat input -->
				<div class="px-4 py-3 border-t border-border flex gap-2">
					<input
						bind:value={chatInput}
						placeholder="Ask about your data..."
						class="flex-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background"
						disabled={chatLoading}
						onkeydown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(); } }}
					/>
					<button
						onclick={sendMessage}
						disabled={chatLoading || !chatInput.trim()}
						class="px-3 py-1.5 text-sm rounded-md bg-primary text-primary-foreground disabled:opacity-50"
					>
						Send
					</button>
				</div>
			</div>
		{/if}

		<!-- Bottom row -->
		<div class="grid grid-cols-2 gap-4">
			<!-- Recent events -->
			<div class="border border-border rounded-lg bg-card overflow-hidden">
				<div class="px-4 py-3 border-b border-border flex items-center justify-between">
					<h3 class="text-sm font-medium">Live Events</h3>
					<a href="/analytics/events" class="text-xs text-primary hover:underline">View all →</a>
				</div>
				{#if recentEvents.length === 0}
					<p class="text-sm text-muted-foreground p-8 text-center">No events yet. Add the SDK to start tracking.</p>
				{:else}
					<div class="divide-y divide-border">
						{#each recentEvents.slice(0, 8) as event}
							<div class="px-4 py-2 flex items-center gap-3 text-sm hover:bg-accent/50 transition-colors">
								<span class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium uppercase tracking-wider shrink-0
									{event.event_type === 'click' ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400' :
									 event.event_type === 'pageview' ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' :
									 event.event_type === 'submit' ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400' :
									 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400'}">
									{event.event_type}
								</span>
								<span class="flex-1 truncate text-xs {event.event_name ? 'font-medium' : 'text-muted-foreground'}">
									{eventDisplayName(event)}
								</span>
								<span class="text-xs text-muted-foreground shrink-0">{relativeTime(event.timestamp)}</span>
							</div>
						{/each}
					</div>
				{/if}
			</div>

			<!-- Recent sessions -->
			<div class="border border-border rounded-lg bg-card overflow-hidden">
				<div class="px-4 py-3 border-b border-border flex items-center justify-between">
					<h3 class="text-sm font-medium">Recent Sessions</h3>
					<a href="/people/sessions" class="text-xs text-primary hover:underline">View all →</a>
				</div>
				{#if sessions.length === 0}
					<p class="text-sm text-muted-foreground p-8 text-center">No sessions yet.</p>
				{:else}
					<div class="divide-y divide-border">
						{#each sessions as session}
							<a href="/people/sessions" class="px-4 py-2.5 flex items-center gap-3 hover:bg-accent/50 transition-colors block">
								<div class="w-7 h-7 rounded-full bg-muted flex items-center justify-center shrink-0">
									<svg class="w-3.5 h-3.5 text-muted-foreground" fill="none" viewBox="0 0 24 24" stroke="currentColor">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
									</svg>
								</div>
								<div class="flex-1 min-w-0">
									<p class="text-xs font-medium truncate">{session.entry_url.replace(/^https?:\/\/[^/]+/, '') || '/'}</p>
									<p class="text-xs text-muted-foreground">{session.event_count} events · {relativeTime(session.last_seen)}</p>
								</div>
								{#if session.distinct_id}
									<span class="text-xs text-muted-foreground truncate max-w-[80px]">{session.distinct_id}</span>
								{/if}
							</a>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>
