<script lang="ts">
	import { onMount } from 'svelte';
	import { getBillingUsage, createBillingPortal, createCheckout } from '$lib/api';

	type BillingData = {
		tier: string;
		period_start: string;
		period_end: string;
		usage: { events: number; leads: number; campaigns: number; icp_analyses: number };
		limits: { free_events: number; free_leads: number; free_campaigns: number; free_icp: number };
	};

	let billing = $state<BillingData | null>(null);
	let loading = $state(true);
	let portalLoading = $state(false);
	let upgradeLoading = $state(false);

	onMount(async () => {
		billing = await getBillingUsage();
		loading = false;
	});

	async function openPortal() {
		portalLoading = true;
		try {
			const res = await createBillingPortal(window.location.href);
			if (res.url) window.location.href = res.url;
		} catch {
			// Portal not available
		} finally {
			portalLoading = false;
		}
	}

	async function handleUpgrade() {
		upgradeLoading = true;
		try {
			const res = await createCheckout(window.location.href, window.location.href);
			if (res.url) window.location.href = res.url;
		} catch {
			// Checkout not available
		} finally {
			upgradeLoading = false;
		}
	}

	function tierLabel(tier: string): string {
		if (tier === 'payg') return 'Pay-as-you-go';
		if (tier === 'enterprise') return 'Enterprise';
		return 'Free';
	}

	function tierBadgeColor(tier: string): string {
		if (tier === 'payg') return 'bg-blue-500/10 text-blue-600';
		if (tier === 'enterprise') return 'bg-purple-500/10 text-purple-600';
		return 'bg-primary/10 text-primary';
	}

	function fmtNumber(n: number): string {
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
		return n.toLocaleString();
	}

	function usagePct(used: number, limit: number): number {
		if (limit <= 0) return 0;
		return Math.min((used / limit) * 100, 100);
	}

	function barColor(pct: number): string {
		if (pct >= 90) return 'bg-red-500';
		if (pct >= 75) return 'bg-yellow-500';
		return 'bg-primary';
	}

	function periodLabel(start: string, end: string): string {
		const s = new Date(start);
		return s.toLocaleDateString('en-US', { month: 'long', year: 'numeric' });
	}
</script>

<div class="p-6 max-w-3xl">
	{#if loading}
		<div class="space-y-4">
			<div class="h-8 w-48 rounded bg-muted animate-pulse"></div>
			<div class="h-32 rounded-lg bg-muted animate-pulse"></div>
			<div class="h-48 rounded-lg bg-muted animate-pulse"></div>
		</div>
	{:else if billing}
		<!-- Plan header -->
		<div class="flex items-start justify-between mb-6">
			<div>
				<h2 class="text-lg font-semibold mb-1">Billing & Usage</h2>
				<p class="text-sm text-muted-foreground">{periodLabel(billing.period_start, billing.period_end)} billing period</p>
			</div>
			<div class="flex items-center gap-2">
				<span class="inline-flex items-center px-2.5 py-1 rounded-full text-xs font-medium {tierBadgeColor(billing.tier)}">
					{tierLabel(billing.tier)}
				</span>
				{#if billing.tier === 'free'}
					<button
						onclick={handleUpgrade}
						disabled={upgradeLoading}
						class="px-3 py-1.5 rounded-md text-xs font-medium bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
					>
						{upgradeLoading ? 'Loading...' : 'Upgrade'}
					</button>
				{/if}
				{#if billing.tier === 'payg' || billing.tier === 'enterprise'}
					<button
						onclick={openPortal}
						disabled={portalLoading}
						class="px-3 py-1.5 rounded-md text-xs font-medium border border-border hover:bg-accent transition-colors disabled:opacity-50"
					>
						{portalLoading ? 'Loading...' : 'Manage Billing'}
					</button>
				{/if}
			</div>
		</div>

		<!-- Usage meters -->
		{@const eventsPct = billing.tier === 'free' ? usagePct(billing.usage.events, billing.limits.free_events) : 0}
		{@const leadsPct = billing.tier === 'free' ? usagePct(billing.usage.leads, billing.limits.free_leads) : 0}
		{@const campaignsPct = billing.tier === 'free' ? usagePct(billing.usage.campaigns, billing.limits.free_campaigns) : 0}
		{@const icpPct = billing.tier === 'free' ? usagePct(billing.usage.icp_analyses, billing.limits.free_icp) : 0}
		<div class="border border-border rounded-lg bg-card divide-y divide-border">
			<!-- Events -->
			<div class="p-4">
				<div class="flex items-center justify-between mb-2">
					<div>
						<p class="text-sm font-medium">Events</p>
						<p class="text-xs text-muted-foreground">Pageviews, clicks, and custom events</p>
					</div>
					<div class="text-right">
						<p class="text-sm font-mono font-medium">{fmtNumber(billing.usage.events)}</p>
						{#if billing.tier === 'free'}
							<p class="text-xs text-muted-foreground">of {fmtNumber(billing.limits.free_events)}</p>
						{:else}
							<p class="text-xs text-muted-foreground">unlimited</p>
						{/if}
					</div>
				</div>
				{#if billing.tier === 'free'}
					<div class="h-1.5 rounded-full bg-muted overflow-hidden">
						<div class="h-full rounded-full transition-all {barColor(eventsPct)}" style="width: {eventsPct.toFixed(1)}%"></div>
					</div>
				{/if}
			</div>

			<!-- Leads -->
			<div class="p-4">
				<div class="flex items-center justify-between mb-2">
					<div>
						<p class="text-sm font-medium">Leads</p>
						<p class="text-xs text-muted-foreground">Scored and identified visitors</p>
					</div>
					<div class="text-right">
						<p class="text-sm font-mono font-medium">{fmtNumber(billing.usage.leads)}</p>
						{#if billing.tier === 'free'}
							<p class="text-xs text-muted-foreground">of {fmtNumber(billing.limits.free_leads)}</p>
						{:else}
							<p class="text-xs text-muted-foreground">unlimited</p>
						{/if}
					</div>
				</div>
				{#if billing.tier === 'free'}
					<div class="h-1.5 rounded-full bg-muted overflow-hidden">
						<div class="h-full rounded-full transition-all {barColor(leadsPct)}" style="width: {leadsPct.toFixed(1)}%"></div>
					</div>
				{/if}
			</div>

			<!-- Campaigns -->
			<div class="p-4">
				<div class="flex items-center justify-between mb-2">
					<div>
						<p class="text-sm font-medium">Campaigns</p>
						<p class="text-xs text-muted-foreground">Active growth campaigns</p>
					</div>
					<div class="text-right">
						<p class="text-sm font-mono font-medium">{billing.usage.campaigns}</p>
						{#if billing.tier === 'free'}
							<p class="text-xs text-muted-foreground">of {billing.limits.free_campaigns}</p>
						{:else}
							<p class="text-xs text-muted-foreground">unlimited</p>
						{/if}
					</div>
				</div>
				{#if billing.tier === 'free'}
					<div class="h-1.5 rounded-full bg-muted overflow-hidden">
						<div class="h-full rounded-full transition-all {barColor(campaignsPct)}" style="width: {campaignsPct.toFixed(1)}%"></div>
					</div>
				{/if}
			</div>

			<!-- ICP Analyses -->
			<div class="p-4">
				<div class="flex items-center justify-between mb-2">
					<div>
						<p class="text-sm font-medium">ICP Analyses</p>
						<p class="text-xs text-muted-foreground">Ideal customer profile discoveries</p>
					</div>
					<div class="text-right">
						<p class="text-sm font-mono font-medium">{billing.usage.icp_analyses}</p>
						{#if billing.tier === 'free'}
							<p class="text-xs text-muted-foreground">of {billing.limits.free_icp}</p>
						{:else}
							<p class="text-xs text-muted-foreground">unlimited</p>
						{/if}
					</div>
				</div>
				{#if billing.tier === 'free'}
					<div class="h-1.5 rounded-full bg-muted overflow-hidden">
						<div class="h-full rounded-full transition-all {barColor(icpPct)}" style="width: {icpPct.toFixed(1)}%"></div>
					</div>
				{/if}
			</div>
		</div>

		<!-- Pricing note for free tier -->
		{#if billing.tier === 'free'}
			<div class="mt-4 p-4 rounded-lg border border-border bg-card">
				<p class="text-xs text-muted-foreground">
					<strong class="text-foreground">Need more?</strong>
					Upgrade to Pay-as-you-go for unlimited events, leads, campaigns, and connectors. Only pay for what you use beyond the free tier.
				</p>
			</div>
		{/if}

		<!-- PAYG pricing reference -->
		{#if billing.tier === 'payg'}
			<div class="mt-4 p-4 rounded-lg border border-border bg-card">
				<p class="text-xs font-medium mb-2">Usage-based rates</p>
				<div class="grid grid-cols-2 gap-x-6 gap-y-1 text-xs text-muted-foreground">
					<span>Events over 1M free</span><span class="text-right">$0.10 / 10K</span>
					<span>Leads</span><span class="text-right">$5 / 500</span>
					<span>Campaigns</span><span class="text-right">$3 each</span>
					<span>ICP analyses</span><span class="text-right">$2 each</span>
				</div>
			</div>
		{/if}
	{:else}
		<div class="text-center py-12">
			<p class="text-sm text-muted-foreground">Billing is not available for self-hosted instances.</p>
		</div>
	{/if}
</div>
