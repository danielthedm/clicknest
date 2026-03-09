<script lang="ts">
	import { tick } from 'svelte';
	import { aiChat } from '$lib/api';

	let { cacheKey, prompt, ready = true }: { cacheKey: string; prompt: string; ready?: boolean } = $props();

	const TTL = 10 * 60 * 1000; // 10 minutes

	let insight = $state('');
	let loading = $state(false);
	let error = $state('');
	let enabled = $state(false);
	let hasLoaded = $state(false);

	function getCached(): string | null {
		try {
			const raw = sessionStorage.getItem(`cn_insight_${cacheKey}`);
			if (!raw) return null;
			const { reply, ts } = JSON.parse(raw);
			if (Date.now() - ts > TTL) return null;
			return reply;
		} catch {
			return null;
		}
	}

	function setCache(reply: string) {
		try {
			sessionStorage.setItem(`cn_insight_${cacheKey}`, JSON.stringify({ reply, ts: Date.now() }));
		} catch { /* ignore */ }
	}

	async function load(force = false) {
		if (!prompt) return;

		if (!force) {
			const cached = getCached();
			if (cached) {
				enabled = true;
				insight = cached;
				hasLoaded = true;
				return;
			}
		}

		loading = true;
		enabled = true;
		error = '';
		try {
			const res = await aiChat(prompt, []);
			insight = res.reply;
			setCache(res.reply);
		} catch (e: any) {
			const msg = e.message || '';
			if (msg.includes('LLM not configured')) {
				enabled = false;
			} else {
				error = msg.replace('API error 500: ', '').replace(/^{"error":"/, '').replace(/"}$/, '');
			}
		}
		loading = false;
		hasLoaded = true;
	}

	$effect(() => {
		if (ready && !hasLoaded) {
			load();
		}
	});
</script>

{#if enabled || loading}
	<div class="border border-border rounded-lg bg-card overflow-hidden mb-6">
		<div class="px-4 py-3 border-b border-border flex items-center gap-2">
			<div class="w-5 h-5 rounded-full bg-primary/10 flex items-center justify-center">
				<svg class="w-3 h-3 text-primary" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.347.347a3.75 3.75 0 01-5.304 0l-.346-.347z" />
				</svg>
			</div>
			<h3 class="text-sm font-medium">AI Insights</h3>
			{#if loading}
				<span class="text-xs text-muted-foreground animate-pulse ml-1">Analyzing...</span>
			{/if}
			<div class="ml-auto">
				<button
					onclick={() => load(true)}
					disabled={loading}
					class="p-1 rounded hover:bg-accent transition-colors disabled:opacity-50"
					title="Refresh insights"
				>
					<svg class="w-3.5 h-3.5 text-muted-foreground {loading ? 'animate-spin' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
					</svg>
				</button>
			</div>
		</div>
		<div class="px-4 py-3">
			{#if loading && !insight}
				<div class="flex gap-1 items-center py-1">
					<span class="w-1.5 h-1.5 bg-muted-foreground/50 rounded-full animate-bounce" style="animation-delay: 0ms"></span>
					<span class="w-1.5 h-1.5 bg-muted-foreground/50 rounded-full animate-bounce" style="animation-delay: 150ms"></span>
					<span class="w-1.5 h-1.5 bg-muted-foreground/50 rounded-full animate-bounce" style="animation-delay: 300ms"></span>
				</div>
			{:else if error}
				<p class="text-xs text-red-500">{error}</p>
			{:else}
				<p class="text-sm leading-relaxed whitespace-pre-wrap">{insight}</p>
			{/if}
		</div>
	</div>
{/if}
