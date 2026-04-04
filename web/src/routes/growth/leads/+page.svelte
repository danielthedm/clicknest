<script lang="ts">
	import { onMount } from 'svelte';
	import { getLeads, listScoringRules, createScoringRule, updateScoringRule, deleteScoringRule, listCRMWebhooks, createCRMWebhook, updateCRMWebhook, deleteCRMWebhook, testCRMWebhook, listWebhookDeliveries, retryWebhookDelivery, getDeadLetters, getLeadAttribution, listSegments, createSegment, deleteSegment, getSegmentMembers } from '$lib/api';
	import { formatTime, relativeTime } from '$lib/utils';
	import type { ScoredLead, ScoringRule, CRMWebhook, WebhookDelivery, DeadLetter, LeadAttribution, Segment } from '$lib/types';
	import Select from '$lib/components/ui/Select.svelte';

	let leads = $state<ScoredLead[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let range = $state('30d');

	let rules = $state<ScoringRule[]>([]);
	let webhooks = $state<CRMWebhook[]>([]);

	let showRuleForm = $state(false);
	let showWebhookForm = $state(false);
	let activeTab = $state<'leads' | 'rules' | 'webhooks' | 'segments'>('leads');

	// Rule form
	let ruleName = $state('');
	let ruleType = $state('page_visit');
	let rulePoints = $state(10);
	let ruleConfigURL = $state('');
	let ruleConfigEvent = $state('');
	let ruleConfigMinCount = $state(1);
	let ruleConfigPropKey = $state('');
	let ruleConfigPropValue = $state('');
	let ruleConfigHalfLife = $state(14);
	let ruleConfigInactiveDays = $state(14);
	let ruleConfigTargetURL = $state('');
	let ruleConfigWithinDays = $state(7);

	// Webhook form
	let whName = $state('');
	let whURL = $state('');
	let whMinScore = $state(50);
	let whTemplate = $state('');
	let showTemplateHelp = $state(false);

	// Dead letters
	let deadLetters = $state<DeadLetter[]>([]);
	let deadLettersLoading = $state(false);
	let showDeadLetters = $state(false);

	// Webhook deliveries
	let expandedWebhookId = $state<string | null>(null);
	let deliveries = $state<WebhookDelivery[]>([]);
	let deliveriesLoading = $state(false);
	let retryingId = $state<string | null>(null);

	// Lead attribution (expandable rows)
	let expandedLeadId = $state<string | null>(null);
	let leadAttribution = $state<LeadAttribution[]>([]);
	let attributionLoading = $state(false);

	// Segments
	let segments = $state<Segment[]>([]);
	let showSegmentForm = $state(false);
	let segName = $state('');
	let segConditions = $state<{ rule_type: string; config: string; points: number; enabled: boolean; name: string; id: string }[]>([]);
	let segRuleType = $state('page_visit');
	let segConfigURL = $state('');
	let segConfigEvent = $state('');
	let segConfigMinCount = $state(1);
	let segConfigPropKey = $state('');
	let segConfigPropValue = $state('');
	let activeSegment = $state<string | null>(null);
	let segmentMembers = $state<ScoredLead[]>([]);
	let segmentMembersLoading = $state(false);

	const ruleTypes = [
		{ value: 'page_visit', label: 'Page Visit' },
		{ value: 'event_count', label: 'Event Count' },
		{ value: 'session_count', label: 'Session Count' },
		{ value: 'identified', label: 'Identified User' },
		{ value: 'property_match', label: 'Property Match' },
		{ value: 'negative', label: 'Negative (property)' },
		{ value: 'inactivity', label: 'Inactivity Penalty' },
		{ value: 'recency_decay', label: 'Recency Decay' },
		{ value: 'behavioral', label: 'Behavioral Pattern' },
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
			const [leadsRes, rulesRes, whRes, segsRes] = await Promise.all([
				getLeads({ start: start.toISOString(), end: end.toISOString() }),
				listScoringRules(),
				listCRMWebhooks(),
				listSegments(),
			]);
			leads = leadsRes.leads ?? [];
			total = leadsRes.total ?? 0;
			rules = rulesRes.rules ?? [];
			webhooks = whRes.webhooks ?? [];
			segments = segsRes.segments ?? [];
		} catch (e) {
			console.error('Failed to load leads:', e);
		}
		loading = false;
	}

	async function toggleLeadAttribution(distinctId: string) {
		if (expandedLeadId === distinctId) {
			expandedLeadId = null;
			leadAttribution = [];
			return;
		}
		expandedLeadId = distinctId;
		attributionLoading = true;
		try {
			const res = await getLeadAttribution(distinctId);
			leadAttribution = res.sources ?? [];
		} catch {
			leadAttribution = [];
		}
		attributionLoading = false;
	}

	function buildSegCondition(): { rule_type: string; config: string; points: number; enabled: boolean; name: string; id: string } {
		let config = '{}';
		if (segRuleType === 'page_visit') config = JSON.stringify({ url_path: segConfigURL });
		else if (segRuleType === 'event_count') config = JSON.stringify({ event_name: segConfigEvent, min_count: segConfigMinCount });
		else if (segRuleType === 'session_count') config = JSON.stringify({ min_count: segConfigMinCount });
		else if (segRuleType === 'property_match') config = JSON.stringify({ property_key: segConfigPropKey, property_value: segConfigPropValue });
		return { rule_type: segRuleType, config, points: 10, enabled: true, name: `Condition ${segConditions.length + 1}`, id: '' };
	}

	function addSegCondition() {
		segConditions = [...segConditions, buildSegCondition()];
		segConfigURL = '';
		segConfigEvent = '';
		segConfigPropKey = '';
		segConfigPropValue = '';
		segConfigMinCount = 1;
	}

	async function handleCreateSegment() {
		if (!segName || segConditions.length === 0) return;
		await createSegment({ name: segName, conditions: JSON.stringify(segConditions) });
		segName = '';
		segConditions = [];
		showSegmentForm = false;
		loadAll();
	}

	async function handleDeleteSegment(id: string) {
		await deleteSegment(id);
		if (activeSegment === id) { activeSegment = null; segmentMembers = []; }
		loadAll();
	}

	async function handleViewSegmentMembers(id: string) {
		if (activeSegment === id) { activeSegment = null; segmentMembers = []; return; }
		activeSegment = id;
		segmentMembersLoading = true;
		try {
			const res = await getSegmentMembers(id);
			segmentMembers = res.members ?? [];
		} catch {
			segmentMembers = [];
		}
		segmentMembersLoading = false;
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
			case 'negative':
				return JSON.stringify({ property_key: ruleConfigPropKey, property_value: ruleConfigPropValue });
			case 'inactivity':
				return JSON.stringify({ inactive_days: ruleConfigInactiveDays });
			case 'recency_decay':
				return JSON.stringify({ half_life_days: ruleConfigHalfLife });
			case 'behavioral':
				return JSON.stringify({ url_path: ruleConfigURL, target_url_path: ruleConfigTargetURL, within_days: ruleConfigWithinDays });
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
		await createCRMWebhook({ name: whName, webhook_url: whURL, min_score: whMinScore, payload_template: whTemplate || undefined });
		showWebhookForm = false;
		whName = '';
		whURL = '';
		whMinScore = 50;
		whTemplate = '';
		loadAll();
	}

	async function handleToggleWebhook(wh: CRMWebhook) {
		await updateCRMWebhook(wh.id, { name: wh.name, webhook_url: wh.webhook_url, min_score: wh.min_score, enabled: !wh.enabled, payload_template: wh.payload_template });
		loadAll();
	}

	async function handleLoadDeadLetters() {
		showDeadLetters = !showDeadLetters;
		if (!showDeadLetters) return;
		deadLettersLoading = true;
		try {
			const res = await getDeadLetters();
			deadLetters = res.dead_letters ?? [];
		} catch {
			deadLetters = [];
		}
		deadLettersLoading = false;
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
		if (expandedWebhookId === id) expandedWebhookId = null;
		loadAll();
	}

	async function toggleWebhookDeliveries(id: string) {
		if (expandedWebhookId === id) {
			expandedWebhookId = null;
			deliveries = [];
			return;
		}
		expandedWebhookId = id;
		deliveriesLoading = true;
		try {
			const res = await listWebhookDeliveries(id);
			deliveries = res.deliveries ?? [];
		} catch (e) {
			deliveries = [];
		}
		deliveriesLoading = false;
	}

	async function handleRetryDelivery(webhookId: string, deliveryId: string) {
		retryingId = deliveryId;
		try {
			const res = await retryWebhookDelivery(webhookId, deliveryId);
			alert(`Retry ${res.success ? 'succeeded' : 'failed'} (HTTP ${res.status_code})`);
			await toggleWebhookDeliveries(webhookId);
		} catch (e) {
			alert(`Retry failed: ${e}`);
		}
		retryingId = null;
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
			<Select
				bind:value={range}
				onchange={() => loadAll()}
				options={[
					{ value: '7d', label: 'Last 7 days' },
					{ value: '30d', label: 'Last 30 days' },
					{ value: '90d', label: 'Last 90 days' },
				]}
				size="sm"
				fullWidth={false}
			/>
		</div>
	</div>

	<!-- Sub-tabs -->
	<div class="flex gap-1 border-b border-border">
		{#each [['leads', 'Leads'], ['rules', 'Scoring Rules'], ['webhooks', 'CRM Webhooks'], ['segments', 'Segments']] as [key, label] (key)}
			<button
				onclick={() => activeTab = key as 'leads' | 'rules' | 'webhooks' | 'segments'}
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
							<th class="text-right px-4 py-2.5 font-medium">Recency</th>
						</tr>
					</thead>
					<tbody>
						{#each leads as lead}
							{@const isExpanded = expandedLeadId === lead.distinct_id}
							<tr
								class="border-b border-border hover:bg-muted/30 transition-colors cursor-pointer"
								onclick={() => toggleLeadAttribution(lead.distinct_id)}
							>
								<td class="px-4 py-2.5 font-mono text-xs">
									<span class="mr-1.5 text-muted-foreground">{isExpanded ? '▼' : '▶'}</span>{lead.distinct_id}
								</td>
								<td class="px-4 py-2.5 text-right font-bold {scoreColor(lead.score)}">
									{lead.score}
									{#if lead.raw_score && lead.raw_score !== lead.score}
										<span class="text-xs font-normal text-muted-foreground ml-1">({lead.raw_score})</span>
									{/if}
									{#if lead.score_delta !== undefined && lead.score_delta !== null}
										{#if lead.score_delta > 0}
											<span class="text-xs font-normal text-green-600 dark:text-green-400 ml-1">▲{lead.score_delta}</span>
										{:else if lead.score_delta < 0}
											<span class="text-xs font-normal text-red-500 ml-1">▼{Math.abs(lead.score_delta)}</span>
										{/if}
									{/if}
								</td>
								<td class="px-4 py-2.5 text-right">{lead.event_count}</td>
								<td class="px-4 py-2.5 text-right">{lead.session_count}</td>
								<td class="px-4 py-2.5 text-right">{lead.page_views}</td>
								<td class="px-4 py-2.5 text-right text-muted-foreground">{relativeTime(lead.last_seen)}</td>
								<td class="px-4 py-2.5 text-right">
									{#if lead.days_since_last_seen <= 1}
										<span class="text-xs px-1.5 py-0.5 rounded bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400">Active</span>
									{:else if lead.days_since_last_seen <= 7}
										<span class="text-xs px-1.5 py-0.5 rounded bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400">{lead.days_since_last_seen}d ago</span>
									{:else}
										<span class="text-xs px-1.5 py-0.5 rounded bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400">{lead.days_since_last_seen}d ago</span>
									{/if}
								</td>
							</tr>
							{#if isExpanded}
								<tr class="border-b border-border bg-muted/20">
									<td colspan="7" class="px-6 py-3">
										{#if attributionLoading}
											<p class="text-xs text-muted-foreground">Loading attribution...</p>
										{:else if leadAttribution.length === 0}
											<p class="text-xs text-muted-foreground">No attribution data — no sessions recorded in the last 90 days.</p>
										{:else}
											<div class="space-y-1">
												<p class="text-xs font-medium text-muted-foreground mb-2">Traffic sources (90 days)</p>
												{#each leadAttribution as src}
													<div class="flex items-center gap-3 text-xs">
														<span class="font-mono bg-muted px-1.5 py-0.5 rounded">{src.source}</span>
														<span class="text-muted-foreground">{src.channel}</span>
														{#if src.campaign}
															<span class="text-muted-foreground italic">{src.campaign}</span>
														{/if}
														<span class="ml-auto">{src.sessions} session{src.sessions !== 1 ? 's' : ''}</span>
														<span class="text-muted-foreground">first: {new Date(src.first_touch).toLocaleDateString()}</span>
													</div>
												{/each}
											</div>
										{/if}
									</td>
								</tr>
							{/if}
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
						<Select
							bind:value={ruleType}
							options={ruleTypes}
							label="Rule Type"
							size="sm"
						/>
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
				{:else if ruleType === 'property_match' || ruleType === 'negative'}
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
					{#if ruleType === 'negative'}
						<p class="text-xs text-muted-foreground">Points will be deducted when this property matches.</p>
					{/if}
				{:else if ruleType === 'inactivity'}
					<div>
						<label class="text-xs font-medium text-muted-foreground">Inactive Days Threshold</label>
						<input type="number" bind:value={ruleConfigInactiveDays} min={1} class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
						<p class="text-xs text-muted-foreground mt-1">Deduct points if user hasn't been active for this many days.</p>
					</div>
				{:else if ruleType === 'recency_decay'}
					<div>
						<label class="text-xs font-medium text-muted-foreground">Half-Life (days)</label>
						<input type="number" bind:value={ruleConfigHalfLife} min={1} class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
						<p class="text-xs text-muted-foreground mt-1">Score halves every N days of inactivity. E.g., 14 means a lead inactive for 14 days gets 50% of their score.</p>
					</div>
				{:else if ruleType === 'behavioral'}
					<div class="grid grid-cols-3 gap-3">
						<div>
							<label class="text-xs font-medium text-muted-foreground">First Page</label>
							<input bind:value={ruleConfigURL} placeholder="/signup" class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
						</div>
						<div>
							<label class="text-xs font-medium text-muted-foreground">Then Visited</label>
							<input bind:value={ruleConfigTargetURL} placeholder="/pricing" class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
						</div>
						<div>
							<label class="text-xs font-medium text-muted-foreground">Within (days)</label>
							<input type="number" bind:value={ruleConfigWithinDays} min={1} class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
						</div>
					</div>
					<p class="text-xs text-muted-foreground">Award points if user visited both pages within the time window.</p>
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
				<div>
					<div class="flex items-center gap-2 mb-1">
						<label class="text-xs font-medium text-muted-foreground">Payload Template</label>
						<button onclick={() => (showTemplateHelp = !showTemplateHelp)} class="text-xs text-muted-foreground hover:text-foreground underline">
							{showTemplateHelp ? 'hide help' : "what's this?"}
						</button>
					</div>
					{#if showTemplateHelp}
						<div class="text-xs text-muted-foreground bg-muted rounded p-2 mb-2 space-y-1">
							<p>Optional — leave blank for the default JSON envelope. Use Go template syntax to customize the payload:</p>
							<pre class="font-mono bg-background rounded p-1.5 overflow-x-auto">&#123;"contacts": &#123;&#123;.leads&#125;&#125;, "source": "clicknest", "ts": "&#123;&#123;.timestamp&#125;&#125;"&#125;</pre>
							<p>Variables: <code>.leads</code> (JSON array), <code>.lead_count</code>, <code>.project_id</code>, <code>.webhook_name</code>, <code>.timestamp</code></p>
						</div>
					{/if}
					<textarea
						bind:value={whTemplate}
						placeholder="Leave blank for default, or use Go template syntax"
						rows={3}
						class="w-full font-mono text-xs px-3 py-2 border border-border rounded-md bg-background resize-y"
					></textarea>
				</div>
				<button onclick={handleCreateWebhook} class="px-4 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90">Create Webhook</button>
			</div>
		{/if}

		<div class="space-y-2">
			{#each webhooks as wh}
				<div class="border border-border rounded-lg {wh.enabled ? 'bg-card' : 'bg-muted/30 opacity-60'}">
					<div class="flex items-center justify-between px-4 py-3">
						<div class="flex items-center gap-3">
							<button onclick={() => handleToggleWebhook(wh)} class="w-8 h-5 rounded-full transition-colors {wh.enabled ? 'bg-primary' : 'bg-muted-foreground/30'}">
								<div class="w-4 h-4 rounded-full bg-white shadow-sm transform transition-transform {wh.enabled ? 'translate-x-3.5' : 'translate-x-0.5'}"></div>
							</button>
							<div>
								<span class="font-medium text-sm">{wh.name}</span>
								<span class="ml-2 text-xs text-muted-foreground font-mono">{wh.webhook_url.slice(0, 40)}{wh.webhook_url.length > 40 ? '...' : ''}</span>
								<span class="ml-2 text-xs text-muted-foreground">min score: {wh.min_score}</span>
							</div>
						</div>
						<div class="flex items-center gap-2">
							{#if wh.last_pushed_at}
								<span class="text-xs text-muted-foreground">Last push: {relativeTime(wh.last_pushed_at)}</span>
							{/if}
							<button onclick={() => toggleWebhookDeliveries(wh.id)} class="px-2 py-1 text-xs border border-border rounded hover:bg-muted">
								{expandedWebhookId === wh.id ? 'Hide History' : 'History'}
							</button>
							<button onclick={() => handleTestWebhook(wh.id)} class="px-2 py-1 text-xs border border-border rounded hover:bg-muted">Test</button>
							<button onclick={() => handleDeleteWebhook(wh.id)} class="text-xs text-muted-foreground hover:text-red-500">Delete</button>
						</div>
					</div>

					{#if expandedWebhookId === wh.id}
						<div class="border-t border-border px-4 py-3">
							{#if deliveriesLoading}
								<p class="text-xs text-muted-foreground">Loading...</p>
							{:else if deliveries.length === 0}
								<p class="text-xs text-muted-foreground">No delivery history yet.</p>
							{:else}
								<div class="space-y-1.5">
									{#each deliveries as dlv}
										<div class="flex items-center justify-between text-xs border border-border rounded px-3 py-2 {dlv.success ? 'bg-green-50 dark:bg-green-900/10' : 'bg-red-50 dark:bg-red-900/10'}">
											<div class="flex items-center gap-3">
												<span class="font-mono {dlv.success ? 'text-green-700 dark:text-green-400' : 'text-red-700 dark:text-red-400'}">
													{dlv.success ? '✓' : '✗'} HTTP {dlv.status_code || '—'}
												</span>
												<span class="text-muted-foreground">{dlv.lead_count} leads · attempt {dlv.attempt}</span>
												{#if dlv.error}
													<span class="text-red-600 dark:text-red-400 truncate max-w-xs">{dlv.error}</span>
												{/if}
											</div>
											<div class="flex items-center gap-2 shrink-0">
												<span class="text-muted-foreground">{relativeTime(dlv.created_at)}</span>
												{#if !dlv.success}
													<button
														onclick={() => handleRetryDelivery(wh.id, dlv.id)}
														disabled={retryingId === dlv.id}
														class="px-2 py-0.5 border border-border rounded hover:bg-muted disabled:opacity-50"
													>Retry</button>
												{/if}
											</div>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
			{#if webhooks.length === 0}
				<p class="text-sm text-muted-foreground text-center py-6">No CRM webhooks configured. Add a webhook to auto-push qualified leads.</p>
			{/if}
		</div>

		<!-- Dead letter queue -->
		<div class="border-t border-border pt-4 mt-4">
			<div class="flex items-center justify-between mb-3">
				<div>
					<h4 class="text-sm font-medium">Dead Letter Queue</h4>
					<p class="text-xs text-muted-foreground">Deliveries that exhausted all retries without success</p>
				</div>
				<button
					onclick={handleLoadDeadLetters}
					class="text-xs text-primary hover:underline"
				>{showDeadLetters ? 'Hide' : 'Show'}</button>
			</div>
			{#if showDeadLetters}
				{#if deadLettersLoading}
					<p class="text-xs text-muted-foreground">Loading...</p>
				{:else if deadLetters.length === 0}
					<p class="text-xs text-muted-foreground py-3 text-center">No permanently failed deliveries.</p>
				{:else}
					<div class="space-y-1.5">
						{#each deadLetters as dl}
							<div class="flex items-start justify-between text-xs border border-red-200 dark:border-red-800 rounded px-3 py-2 bg-red-50 dark:bg-red-900/10">
								<div class="space-y-0.5">
									<div class="flex items-center gap-2">
										<span class="font-medium text-red-700 dark:text-red-400">{dl.webhook_name || dl.webhook_id}</span>
										<span class="text-muted-foreground">HTTP {dl.status_code || '—'} · {dl.lead_count} leads · attempt {dl.attempt}</span>
									</div>
									{#if dl.response_body}
										<p class="text-muted-foreground font-mono truncate max-w-lg">{dl.response_body.slice(0, 120)}</p>
									{/if}
									{#if dl.error}
										<p class="text-red-600 dark:text-red-400">{dl.error}</p>
									{/if}
								</div>
								<div class="flex items-center gap-2 shrink-0 ml-4">
									<span class="text-muted-foreground">{relativeTime(dl.created_at)}</span>
									<button
										onclick={() => handleRetryDelivery(dl.webhook_id, dl.id)}
										disabled={retryingId === dl.id}
										class="px-2 py-0.5 border border-border rounded hover:bg-muted disabled:opacity-50"
									>{retryingId === dl.id ? '...' : 'Retry'}</button>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			{/if}
		</div>
	{:else if activeTab === 'segments'}
		<div class="flex items-center justify-between">
			<div>
				<p class="text-sm text-muted-foreground">Named user groups defined by behavioral conditions — query members on demand</p>
			</div>
			<button onclick={() => showSegmentForm = !showSegmentForm} class="px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90">
				{showSegmentForm ? 'Cancel' : '+ New Segment'}
			</button>
		</div>

		{#if showSegmentForm}
			<div class="border border-border rounded-lg p-4 space-y-3 bg-card">
				<div>
					<label class="text-xs font-medium text-muted-foreground">Segment Name</label>
					<input bind:value={segName} placeholder="e.g. High-Intent Visitors" class="w-full mt-1 px-3 py-1.5 text-sm border border-border rounded-md bg-background" />
				</div>

				<div class="space-y-2">
					<p class="text-xs font-medium text-muted-foreground">Conditions (users matching any condition are included)</p>
					{#each segConditions as cond, i}
						{@const cfg = JSON.parse(cond.config)}
						<div class="flex items-center gap-2 text-xs bg-muted rounded px-3 py-2">
							<span class="font-medium">{ruleTypes.find(r => r.value === cond.rule_type)?.label}</span>
							{#if cfg.url_path}<span class="font-mono">{cfg.url_path}</span>{/if}
							{#if cfg.event_name}<span class="font-mono">{cfg.event_name}</span>{/if}
							{#if cfg.property_key}<span class="font-mono">{cfg.property_key}={cfg.property_value}</span>{/if}
							<button onclick={() => segConditions = segConditions.filter((_, j) => j !== i)} class="ml-auto text-muted-foreground hover:text-red-500">✕</button>
						</div>
					{/each}

					<div class="flex items-end gap-2 mt-2 flex-wrap">
						<div>
							<Select
								bind:value={segRuleType}
								options={ruleTypes.filter(r => !['inactivity','recency_decay','negative'].includes(r.value))}
								label="Type"
								size="sm"
								fullWidth={false}
							/>
						</div>
						{#if segRuleType === 'page_visit'}
							<div><label class="text-xs text-muted-foreground">URL Path</label><input bind:value={segConfigURL} placeholder="/pricing" class="mt-1 w-40 px-2 py-1.5 text-sm border border-border rounded-md bg-background" /></div>
						{:else if segRuleType === 'event_count'}
							<div><label class="text-xs text-muted-foreground">Event</label><input bind:value={segConfigEvent} class="mt-1 w-32 px-2 py-1.5 text-sm border border-border rounded-md bg-background" /></div>
							<div><label class="text-xs text-muted-foreground">Min</label><input type="number" bind:value={segConfigMinCount} class="mt-1 w-20 px-2 py-1.5 text-sm border border-border rounded-md bg-background" /></div>
						{:else if segRuleType === 'session_count'}
							<div><label class="text-xs text-muted-foreground">Min Sessions</label><input type="number" bind:value={segConfigMinCount} class="mt-1 w-24 px-2 py-1.5 text-sm border border-border rounded-md bg-background" /></div>
						{:else if segRuleType === 'property_match'}
							<div><label class="text-xs text-muted-foreground">Key</label><input bind:value={segConfigPropKey} class="mt-1 w-28 px-2 py-1.5 text-sm border border-border rounded-md bg-background" /></div>
							<div><label class="text-xs text-muted-foreground">Value</label><input bind:value={segConfigPropValue} class="mt-1 w-28 px-2 py-1.5 text-sm border border-border rounded-md bg-background" /></div>
						{/if}
						<button onclick={addSegCondition} class="mt-auto px-3 py-1.5 text-sm border border-primary text-primary rounded-md hover:bg-primary/10">+ Add Condition</button>
					</div>
				</div>

				<button
					onclick={handleCreateSegment}
					disabled={!segName || segConditions.length === 0}
					class="px-4 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90 disabled:opacity-50"
				>Save Segment</button>
			</div>
		{/if}

		<div class="space-y-2">
			{#each segments as seg}
				<div class="border border-border rounded-lg bg-card">
					<div class="flex items-center justify-between px-4 py-3">
						<div>
							<span class="font-medium text-sm">{seg.name}</span>
							<span class="ml-2 text-xs text-muted-foreground">
								{JSON.parse(seg.conditions).length} condition{JSON.parse(seg.conditions).length !== 1 ? 's' : ''}
							</span>
						</div>
						<div class="flex items-center gap-2">
							<button
								onclick={() => handleViewSegmentMembers(seg.id)}
								class="px-3 py-1 text-xs border border-border rounded hover:bg-muted"
							>{activeSegment === seg.id ? 'Hide Members' : 'View Members'}</button>
							<button onclick={() => handleDeleteSegment(seg.id)} class="text-xs text-muted-foreground hover:text-red-500">Delete</button>
						</div>
					</div>
					{#if activeSegment === seg.id}
						<div class="border-t border-border px-4 py-3">
							{#if segmentMembersLoading}
								<p class="text-xs text-muted-foreground">Querying segment members...</p>
							{:else if segmentMembers.length === 0}
								<p class="text-xs text-muted-foreground">No users match this segment in the last 30 days.</p>
							{:else}
								<p class="text-xs text-muted-foreground mb-2">{segmentMembers.length} users match this segment</p>
								<div class="space-y-1 max-h-48 overflow-y-auto">
									{#each segmentMembers.slice(0, 50) as m}
										<div class="flex items-center gap-3 text-xs py-1 border-b border-border last:border-0">
											<span class="font-mono">{m.distinct_id}</span>
											<span class="ml-auto font-bold {scoreColor(m.score)}">{m.score} pts</span>
											<span class="text-muted-foreground">{m.session_count} sessions</span>
											<span class="text-muted-foreground">{relativeTime(m.last_seen)}</span>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
			{#if segments.length === 0}
				<div class="border border-border rounded-lg p-8 bg-card text-center">
					<p class="text-sm text-muted-foreground">No segments yet. Create a segment to define and query a named user group.</p>
				</div>
			{/if}
		</div>
	{/if}
</div>
