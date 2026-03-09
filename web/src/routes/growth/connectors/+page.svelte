<script lang="ts">
	import { onMount } from 'svelte';
	import { listConnectors } from '$lib/api';
	import type { ConnectorInfo } from '$lib/types';

	let connectors = $state<ConnectorInfo[]>([]);
	let loading = $state(true);

	onMount(async () => {
		try {
			const res = await listConnectors();
			connectors = res.connectors ?? [];
		} catch (e) {
			console.error('Failed to load connectors:', e);
		}
		loading = false;
	});
</script>

<div class="p-6 space-y-6">
	<div>
		<h2 class="text-xl font-semibold">Connectors</h2>
		<p class="text-sm text-muted-foreground">Social and content platform integrations for publishing campaigns</p>
	</div>

	{#if loading}
		<div class="text-sm text-muted-foreground py-8 text-center">Loading connectors...</div>
	{:else if connectors.length === 0}
		<div class="border border-border rounded-lg p-8 bg-card text-center space-y-4">
			<div class="w-12 h-12 mx-auto rounded-full bg-muted flex items-center justify-center">
				<svg class="w-6 h-6 text-muted-foreground" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
					<path stroke-linecap="round" stroke-linejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.86-2.556a4.5 4.5 0 00-6.364-6.364L4.5 8.257l4.5 4.5a4.5 4.5 0 006.364 0l4.243-4.243z" />
				</svg>
			</div>
			<div>
				<h3 class="text-lg font-medium">No connectors registered</h3>
				<p class="text-sm text-muted-foreground mt-1">
					Connectors enable publishing campaigns directly to social platforms like Reddit, LinkedIn, and Twitter.
				</p>
			</div>
			<div class="border border-border rounded-lg p-4 bg-muted/50 text-left max-w-lg mx-auto">
				<p class="text-xs font-medium text-muted-foreground mb-2">To add a connector, implement the Connector interface:</p>
				<pre class="text-xs font-mono bg-background rounded p-3 overflow-x-auto">type Connector interface &#123;
    Name() string
    DisplayName() string
    Post(ctx, content) (*PostResult, error)
    FetchEngagement(ctx, id) (*Metrics, error)
    Validate(ctx) error
&#125;</pre>
				<p class="text-xs text-muted-foreground mt-2">
					Register your connector in main.go with <code class="bg-background px-1 rounded">registry.Register(myConnector)</code>
				</p>
			</div>
		</div>
	{:else}
		<div class="grid grid-cols-2 gap-4">
			{#each connectors as connector}
				<div class="border border-border rounded-lg p-4 bg-card flex items-center justify-between">
					<div>
						<h3 class="font-medium text-sm">{connector.display_name}</h3>
						<p class="text-xs text-muted-foreground font-mono">{connector.name}</p>
					</div>
					<span class="text-xs px-2 py-1 rounded-full bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400">Active</span>
				</div>
			{/each}
		</div>
	{/if}
</div>
