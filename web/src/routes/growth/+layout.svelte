<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { getBillingUsage } from '$lib/api';
	let { children } = $props();

	const tabs = [
		{ href: '/growth/leads', label: 'Leads' },
		{ href: '/growth/mentions', label: 'Mentions' },
		{ href: '/growth/campaigns', label: 'Campaigns' },
		{ href: '/growth/conversions', label: 'Conversions' },
		{ href: '/growth/icp', label: 'ICP' },
		{ href: '/growth/connectors', label: 'Connectors' },
	];

	let usage = $state<Awaited<ReturnType<typeof getBillingUsage>>>(null);

	onMount(async () => {
		usage = await getBillingUsage();
	});

	function usagePct(used: number, limit: number): number {
		if (limit <= 0) return 0;
		return Math.min(Math.round((used / limit) * 100), 100);
	}

	function nearLimit(used: number, limit: number): boolean {
		return limit > 0 && used / limit >= 0.8;
	}
</script>

<div class="flex flex-col h-full">
	<div class="border-b border-border px-6 pt-4">
		<div class="flex items-end justify-between">
			<div class="flex gap-1">
				{#each tabs as tab}
					{@const active = $page.url.pathname === tab.href || $page.url.pathname.startsWith(tab.href + '/')}
					<a
						href={tab.href}
						class="px-3 py-2 text-sm rounded-t-md border-b-2 transition-colors {active
							? 'border-primary text-primary font-medium'
							: 'border-transparent text-muted-foreground hover:text-foreground'}"
					>{tab.label}</a>
				{/each}
			</div>
			{#if usage}
				<div class="flex items-center gap-3 pb-2 text-xs text-muted-foreground">
					<span class="px-1.5 py-0.5 rounded bg-muted font-medium capitalize">{usage.tier}</span>
					<div class="flex items-center gap-2">
						<span class:text-orange-500={nearLimit(usage.usage.campaigns, usage.limits.free_campaigns)}>
							{usage.usage.campaigns}/{usage.limits.free_campaigns} campaigns
						</span>
						<span class:text-orange-500={nearLimit(usage.usage.events, usage.limits.free_events)}>
							{(usage.usage.events / 1000).toFixed(0)}K/{(usage.limits.free_events / 1000).toFixed(0)}K events
						</span>
					</div>
					{#if nearLimit(usage.usage.campaigns, usage.limits.free_campaigns) || nearLimit(usage.usage.events, usage.limits.free_events)}
						<a href="/settings/billing" class="text-primary hover:underline">Upgrade</a>
					{/if}
				</div>
			{/if}
		</div>
	</div>
	<div class="flex-1 overflow-y-auto">
		{@render children()}
	</div>
</div>
