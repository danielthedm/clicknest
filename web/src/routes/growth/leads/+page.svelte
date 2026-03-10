<script lang="ts">
	import { onMount } from 'svelte';
	import { getLeads, listScoringRules, createScoringRule, updateScoringRule, deleteScoringRule, listCRMWebhooks, createCRMWebhook, updateCRMWebhook, deleteCRMWebhook, testCRMWebhook } from '$lib/api';
	import { formatTime, relativeTime } from '$lib/utils';
	import type { ScoredLead, ScoringRule, CRMWebhook } from '$lib/types';

	let leads = $state<ScoredLead[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let range = $state('30d');

	let rules = $state<ScoringRule[]>([]);
	let webhooks = $state<CRMWebhook[]>([]);

	let showRuleForm = $state(false);
	let showWebhookForm = $state(false);
	let activeTab = $state<'leads' | 'rules' | 'webhooks'>('leads');

	// Rule form
	let ruleName = $state('');
	let ruleType = $state('page_visit');
	let rulePoints = $state(10);
	let ruleConfigURL = $state('');
	let ruleConfigEvent = $state('');
	let ruleConfigMinCount = $state(1);
	let ruleConfigPropKey = $state('');
	let ruleConfigPropValue = $state('');

	// Webhook form
	let whName = $state('');
	let whURL = $state('');
	let whMinScore = $state(50);

	const ruleTypes = [
		{ value: 'page_visit', label: 'Page Visit' },
		{ value: 'event_count', label: 'Event Count' },
		{ value: 'session_count', label: 'Session Count' },
		{ value: 'identified', label: 'Identified User' },
		{ value: 'property_match', label: 'Property Match' },
	];

	onMount(() => {
		loadAll();
	});

	function getDateRange() {
		const end = new Date();
		const days: Record<string, number> = { '7d': 7, '30d': 30, '90d': 90 };
		const d = days[range] ?? 30;
		return { start: new Date(end.getTime() - d * 86400000), end };
	}

	async function loadAll() {
		loading = true;
		try {
			const { start, end } = getDateRange();
			const [leadsRes, rulesRes, whRes] = await Promise.all([
				getLeads({ start: start.toISOString(), end: end.toISOString() }),
				listScoringRules(),
				listCRMWebhooks(),
			]);
			leads = leadsRes.leads ?? [];
			total = leadsRes.total ?? 0;
			rules = rulesRes.rules ?? [];
			webhooks = whRes.webhooks ?? [];
		} catch (e) {
			console.error('Failed to load leads:', e);
		}
		loading = false;
	}

	function buildConfig(): string {
		switch (ruleType) {
			case 'page_visit':
				return JSON.stringify({ url_path: ruleConfigURL });
			case 'event_count':
				return JSON.stringify({ event_name: ruleConfigEvent, min_count: ruleConfigMinCount });
			case 'session_count':
				return JSON.stringify({ min_count: ruleConfigMinCount });
			case 'property_match':
				return JSON.stringify({ property_key: ruleConfigPropKey, property_value: ruleConfigPropValue });
			default:
				return '{}';
		}
	}

	async function handleCreateRule() {
		if (!ruleName) return;
		await createScoringRule({ name: ruleName, rule_type: ruleType, config: buildConfig(), points: rulePoints });
		showRuleForm = false;
		ruleName = '';
		rulePoints = 10;
		ruleConfigURL = '';
		ruleConfigEvent = '';
		loadAll();
	}

	async function handleToggleRule(rule: ScoringRule) {
		await updateScoringRule(rule.id, { name: rule.name, rule_type: rule.rule_type, config: rule.config, points: rule.points, enabled: !rule.enabled });
		loadAll();
	}

	async function handleDeleteRule(id: string) {
		await deleteScoringRule(id);
		loadAll();
	}

	async function handleCreateWebhook() {
		if (!whName || !whURL) return;
		await createCRMWebhook({ name: whName, webhook_url: whURL, min_score: whMinScore });
		showWebhookForm = false;
		whName = '';
		whURL = '';
		whMinScore = 50;
		loadAll();
	}

	async function handleToggleWebhook(wh: CRMWebhook) {
		await updateCRMWebhook(wh.id, { name: wh.name, webhook_url: wh.webhook_url, min_score: wh.min_score, enabled: !wh.enabled });
		loadAll();
	}

	async function handleTestWebhook(id: string) {
		try {
			const res = await testCRMWebhook(id);
			alert(`Test sent! HTTP status: ${res.http_status}`);
		} catch (e) {
			alert(`Test failed: ${e}`);
		}
	}

	async function handleDeleteWebhook(id: string) {
		await deleteCRMWebhook(id);
		loadAll();
	}

	function scoreColor(score: number): string {
		if (score >= 80) return 'text-green-600 dark:text-green-400';
		if (score >= 40) return 'text-yellow-600 dark:text-yellow-400';
		return 'text-muted-foreground';
	}

	function parseRuleConfig(config: string): Record<string, unknown> {
		try { return JSON.parse(config); } catch { return {}; }
	}
</script>

<div class="p-6 space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-xl font-semibold">Lead Scoring</h2>
			<p class="text-sm text-muted-foreground">{total} identified users scored based on {rules.length} rule{rules.length !== 1 ? 's' : ''}</p>
		</div>
		<div class="flex items-center gap-2">
			<select bind:value={range} onchange={() => loadAll()} class="text-sm border border-border rounded-md px-2 py-1 bg-background">
				<option value="7d">Last 7 days</option>
				<option value="30d">Last 30 days</option>
				<option value="90d">Last 90 days</option>
			</select>
		</div>
	</div>

	<!-- Sub-tabs -->
	<div class="flex gap-1 border-b border-border">
		{#each [['leads', 'Leads'], ['rules', 'Scoring Rules'], ['webhooks', 'CRM Webhooks']] as [key, label]}
			<button
				onclick={() => activeTab = key as 'leads' | 'rules' | 'webhooks'}
				class="px-3 py-2 text-sm border-b-2 transition-colors {activeTab === key
					? 'border-primary text-primary font-medium'
					: 'border-transparent text-muted-foreground hover:text-foreground'}"
			>{label}</button>
		{/each}
	</div>

	{#if activeTab === 'leads'}
		{#if loading}
			<div class="text-sm text-muted-foreground py-8 text-center">Loading leads...</div>
		{:else if leads.length === 0}
			<div class="text-center py-12 text-muted-foreground">
				<p class="text-lg font-medium">No scored leads yet</p>
				<p class="text-sm mt-1">Add scoring rules and identify users with the SDK to see leads here.</p>
			</div>
		{:else}
			<div class="border border-border rounded-lg overflow-hidden">
				<table class="w-full text-sm">
					<thead>
						<tr class="bg-muted/50 border-b border-border">
							<th class="text-left px-4 py-2.5 font-medium">User</th>
							<th class="text-right px-4 py-2.5 font-medium">Score</th>
							<th class="text-right px-4 py-2.5 font-medium">Events</th>
							<th class="text-right px-4 py-2.5 font-medium">Sessions</th>
							<th class="text-right px-4 py-2.5 font-medium">Pages</th>
							<th class="text-right px-4 py-2.5 font-medium">Last Seen</th>
						</tr>
					</thead>
					<tbody>
						{#each leads as lead}
							<tr class="border-b border-border hover:bg-muted/30 transition-colors">
								<td class="px-4 py-2.5 font-mono text-xs">{lead.distinct_id}</td>
								<td class="px-4 py-2.5 text-right font-bold {scoreColor(lead.score)}">{lead.score}</td>
								<td class="px-4 py-2.5 text-right">{lead.event_count}</td>
								<td class="px-4 py-2.5 text-right">{lead.session_count}</td>
								<td class="px-4 py-2.5 text-right">{lead.page_views}</td>
								<td class="px-4 py-2.5 text-right text-muted-foreground">{relativeTime(lead.last_seen)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}

	{:else if activeTab === 'rules'}
		<div class="flex items-center justify-between">
			<p class="text-sm text-muted-foreground">{rules.length} scoring rule{rules.length !== 1 ? 's' : ''} configured</p>
			<button onclick={() => showRuleForm = !showRuleForm} class="px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90">
				{showRuleForm ? 'Cancel' : '+ Add Rule'}
			</button>
		</div>

		{#if showRuleForm}
			<div class="border border-border rounded-lg p-4 space-y-3 bg-card">
				<div class="grid grid-cols-3 gap-3">
					<div>
						<label class="text-xs font-medium text-muted-foreground">Name</label>
						<input bind:value={ruleName} placeholder="e.g. Visited pricing" class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
					</div>
					<div>
						<label class="text-xs font-medium text-muted-foreground">Rule Type</label>
						<select bind:value={ruleType} class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background">
							{#each ruleTypes as rt}
								<option value={rt.value}>{rt.label}</option>
							{/each}
						</select>
					</div>
					<div>
						<label class="text-xs font-medium text-muted-foreground">Points</label>
						<input type="number" bind:value={rulePoints} class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
					</div>
				</div>

				{#if ruleType === 'page_visit'}
					<div>
						<label class="text-xs font-medium text-muted-foreground">URL Path</label>
						<input bind:value={ruleConfigURL} placeholder="/pricing" class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
					</div>
				{:else if ruleType === 'event_count'}
					<div class="grid grid-cols-2 gap-3">
						<div>
							<label class="text-xs font-medium text-muted-foreground">Event Name (optional)</label>
							<input bind:value={ruleConfigEvent} placeholder="signup_click" class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
						</div>
						<div>
							<label class="text-xs font-medium text-muted-foreground">Min Count</label>
							<input type="number" bind:value={ruleConfigMinCount} class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
						</div>
					</div>
				{:else if ruleType === 'session_count'}
					<div>
						<label class="text-xs font-medium text-muted-foreground">Min Sessions</label>
						<input type="number" bind:value={ruleConfigMinCount} class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
					</div>
				{:else if ruleType === 'property_match'}
					<div class="grid grid-cols-2 gap-3">
						<div>
							<label class="text-xs font-medium text-muted-foreground">Property Key</label>
							<input bind:value={ruleConfigPropKey} placeholder="plan" class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
						</div>
						<div>
							<label class="text-xs font-medium text-muted-foreground">Property Value</label>
							<input bind:value={ruleConfigPropValue} placeholder="pro" class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
						</div>
					</div>
				{/if}

				<button onclick={handleCreateRule} class="px-4 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90">Create Rule</button>
			</div>
		{/if}

		<div class="space-y-2">
			{#each rules as rule}
				{@const cfg = parseRuleConfig(rule.config)}
				<div class="flex items-center justify-between border border-border rounded-lg px-4 py-3 {rule.enabled ? 'bg-card' : 'bg-muted/30 opacity-60'}">
					<div class="flex items-center gap-3">
						<button onclick={() => handleToggleRule(rule)} class="w-8 h-5 rounded-full transition-colors {rule.enabled ? 'bg-primary' : 'bg-muted-foreground/30'}">
							<div class="w-4 h-4 rounded-full bg-white shadow-sm transform transition-transform {rule.enabled ? 'translate-x-3.5' : 'translate-x-0.5'}"></div>
						</button>
						<div>
							<span class="font-medium text-sm">{rule.name}</span>
							<span class="ml-2 text-xs px-1.5 py-0.5 rounded bg-muted text-muted-foreground">{ruleTypes.find(r => r.value === rule.rule_type)?.label ?? rule.rule_type}</span>
							{#if cfg.url_path}<span class="ml-1 text-xs text-muted-foreground font-mono">{cfg.url_path}</span>{/if}
							{#if cfg.event_name}<span class="ml-1 text-xs text-muted-foreground font-mono">{cfg.event_name}</span>{/if}
						</div>
					</div>
					<div class="flex items-center gap-3">
						<span class="text-sm font-bold text-primary">+{rule.points} pts</span>
						<button onclick={() => handleDeleteRule(rule.id)} class="text-xs text-muted-foreground hover:text-red-500">Delete</button>
					</div>
				</div>
			{/each}
			{#if rules.length === 0}
				<p class="text-sm text-muted-foreground text-center py-6">No scoring rules yet. Add rules to score your leads.</p>
			{/if}
		</div>

	{:else if activeTab === 'webhooks'}
		<div class="flex items-center justify-between">
			<p class="text-sm text-muted-foreground">Push qualified leads to your CRM every 15 minutes</p>
			<button onclick={() => showWebhookForm = !showWebhookForm} class="px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90">
				{showWebhookForm ? 'Cancel' : '+ Add Webhook'}
			</button>
		</div>

		{#if showWebhookForm}
			<div class="border border-border rounded-lg p-4 space-y-3 bg-card">
				<div class="grid grid-cols-3 gap-3">
					<div>
						<label class="text-xs font-medium text-muted-foreground">Name</label>
						<input bind:value={whName} placeholder="My CRM" class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
					</div>
					<div>
						<label class="text-xs font-medium text-muted-foreground">Webhook URL</label>
						<input bind:value={whURL} placeholder="https://..." class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
					</div>
					<div>
						<label class="text-xs font-medium text-muted-foreground">Min Score</label>
						<input type="number" bind:value={whMinScore} class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
					</div>
				</div>
				<button onclick={handleCreateWebhook} class="px-4 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90">Create Webhook</button>
			</div>
		{/if}

		<div class="space-y-2">
			{#each webhooks as wh}
				<div class="flex items-center justify-between border border-border rounded-lg px-4 py-3 {wh.enabled ? 'bg-card' : 'bg-muted/30 opacity-60'}">
					<div class="flex items-center gap-3">
						<button onclick={() => handleToggleWebhook(wh)} class="w-8 h-5 rounded-full transition-colors {wh.enabled ? 'bg-primary' : 'bg-muted-foreground/30'}">
							<div class="w-4 h-4 rounded-full bg-white shadow-sm transform transition-transform {wh.enabled ? 'translate-x-3.5' : 'translate-x-0.5'}"></div>
						</button>
						<div>
							<span class="font-medium text-sm">{wh.name}</span>
							<span class="ml-2 text-xs text-muted-foreground font-mono">{wh.webhook_url.slice(0, 40)}...</span>
							<span class="ml-2 text-xs text-muted-foreground">min score: {wh.min_score}</span>
						</div>
					</div>
					<div class="flex items-center gap-2">
						{#if wh.last_pushed_at}
							<span class="text-xs text-muted-foreground">Last push: {relativeTime(wh.last_pushed_at)}</span>
						{/if}
						<button onclick={() => handleTestWebhook(wh.id)} class="px-2 py-1 text-xs border border-border rounded hover:bg-muted">Test</button>
						<button onclick={() => handleDeleteWebhook(wh.id)} class="text-xs text-muted-foreground hover:text-red-500">Delete</button>
					</div>
				</div>
			{/each}
			{#if webhooks.length === 0}
				<p class="text-sm text-muted-foreground text-center py-6">No CRM webhooks configured. Add a webhook to auto-push qualified leads.</p>
			{/if}
		</div>
	{/if}
</div>
