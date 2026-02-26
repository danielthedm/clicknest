<script lang="ts">
	import { onMount } from 'svelte';
	import { listAlerts, createAlert, updateAlert, deleteAlert } from '$lib/api';
	import { relativeTime } from '$lib/utils';
	import type { Alert } from '$lib/types';

	let alerts = $state<Alert[]>([]);
	let loading = $state(true);
	let showForm = $state(false);
	let error = $state('');

	// New alert form
	let newName = $state('');
	let newMetric = $state('error_count');
	let newEventName = $state('');
	let newThreshold = $state(10);
	let newWindowMinutes = $state(60);
	let newWebhookURL = $state('');
	let creating = $state(false);

	const windowOptions = [
		{ value: 15, label: '15 min' },
		{ value: 30, label: '30 min' },
		{ value: 60, label: '1 hour' },
		{ value: 1440, label: '24 hours' },
	];

	onMount(() => load());

	async function load() {
		loading = true;
		try {
			const res = await listAlerts();
			alerts = res.alerts ?? [];
		} catch (e) {
			console.error('Failed to load alerts:', e);
		}
		loading = false;
	}

	async function handleCreate() {
		if (!newName.trim() || !newWebhookURL.trim()) return;
		creating = true;
		error = '';
		try {
			await createAlert({
				name: newName,
				metric: newMetric,
				event_name: newEventName || undefined,
				threshold: newThreshold,
				window_minutes: newWindowMinutes,
				webhook_url: newWebhookURL,
				enabled: true,
			});
			newName = '';
			newMetric = 'error_count';
			newEventName = '';
			newThreshold = 10;
			newWindowMinutes = 60;
			newWebhookURL = '';
			showForm = false;
			await load();
		} catch (e: any) {
			error = e.message ?? 'Create failed';
		}
		creating = false;
	}

	async function toggleEnabled(alert: Alert) {
		try {
			await updateAlert(alert.id, !alert.enabled, alert.threshold, alert.webhook_url);
			alert.enabled = !alert.enabled;
			alerts = [...alerts];
		} catch (e) {
			console.error('Failed to update alert:', e);
		}
	}

	async function handleDelete(id: string) {
		if (!confirm('Delete this alert?')) return;
		try {
			await deleteAlert(id);
			alerts = alerts.filter(a => a.id !== id);
		} catch (e) {
			console.error('Failed to delete alert:', e);
		}
	}

	function metricLabel(m: string): string {
		const labels: Record<string, string> = {
			error_count: 'Error count',
			event_count: 'Event count',
			pageview_count: 'Pageview count',
		};
		return labels[m] ?? m;
	}
</script>

<div class="p-6 max-w-4xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Alerts</h2>
			<p class="text-sm text-muted-foreground mt-1">Fire webhooks when metrics cross thresholds</p>
		</div>
		<button
			onclick={() => { showForm = !showForm; error = ''; }}
			class="px-3 py-1.5 text-sm rounded-md bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
		>
			+ New Alert
		</button>
	</div>

	{#if showForm}
		<div class="border border-border rounded-lg p-4 bg-card mb-6">
			<h3 class="text-sm font-medium mb-3">Create Alert</h3>
			<div class="grid grid-cols-2 gap-3 mb-3">
				<div>
					<label class="text-xs text-muted-foreground block mb-1">Name</label>
					<input bind:value={newName} placeholder="High error rate" class="w-full px-2 py-1.5 text-sm border border-border rounded bg-background" />
				</div>
				<div>
					<label class="text-xs text-muted-foreground block mb-1">Metric</label>
					<select bind:value={newMetric} class="w-full px-2 py-1.5 text-sm border border-border rounded bg-background">
						<option value="error_count">Error count</option>
						<option value="event_count">Event count</option>
						<option value="pageview_count">Pageview count</option>
					</select>
				</div>
				{#if newMetric === 'event_count'}
					<div>
						<label class="text-xs text-muted-foreground block mb-1">Event name</label>
						<input bind:value={newEventName} placeholder="e.g. signup" class="w-full px-2 py-1.5 text-sm border border-border rounded bg-background" />
					</div>
				{/if}
				<div>
					<label class="text-xs text-muted-foreground block mb-1">Threshold (fire when count &gt; this)</label>
					<input type="number" bind:value={newThreshold} min="0" class="w-full px-2 py-1.5 text-sm border border-border rounded bg-background" />
				</div>
				<div>
					<label class="text-xs text-muted-foreground block mb-1">Window</label>
					<select bind:value={newWindowMinutes} class="w-full px-2 py-1.5 text-sm border border-border rounded bg-background">
						{#each windowOptions as opt}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				</div>
				<div class="col-span-2">
					<label class="text-xs text-muted-foreground block mb-1">Webhook URL</label>
					<input bind:value={newWebhookURL} placeholder="https://hooks.slack.com/..." class="w-full px-2 py-1.5 text-sm border border-border rounded bg-background" />
				</div>
			</div>
			{#if error}
				<p class="text-xs text-destructive mb-2">{error}</p>
			{/if}
			<div class="flex gap-2">
				<button
					onclick={handleCreate}
					disabled={creating || !newName.trim() || !newWebhookURL.trim()}
					class="px-3 py-1.5 text-sm rounded-md bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50 transition-colors"
				>
					{creating ? 'Creating…' : 'Create'}
				</button>
				<button
					onclick={() => { showForm = false; error = ''; }}
					class="px-3 py-1.5 text-sm rounded-md border border-border hover:bg-accent transition-colors"
				>
					Cancel
				</button>
			</div>
		</div>
	{/if}

	{#if loading}
		<div class="flex items-center justify-center h-48">
			<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
		</div>
	{:else if alerts.length === 0}
		<div class="border border-border rounded-lg p-12 bg-card text-center">
			<p class="text-muted-foreground">No alerts configured. Create one to get notified via webhook.</p>
		</div>
	{:else}
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b border-border bg-muted/50">
						<th class="text-left px-4 py-2 text-xs font-medium text-muted-foreground">Name</th>
						<th class="text-left px-4 py-2 text-xs font-medium text-muted-foreground">Condition</th>
						<th class="text-left px-4 py-2 text-xs font-medium text-muted-foreground">Webhook</th>
						<th class="text-left px-4 py-2 text-xs font-medium text-muted-foreground">Last triggered</th>
						<th class="text-left px-4 py-2 text-xs font-medium text-muted-foreground">Enabled</th>
						<th class="px-4 py-2"></th>
					</tr>
				</thead>
				<tbody class="divide-y divide-border">
					{#each alerts as alert}
						<tr class="hover:bg-accent/30 transition-colors">
							<td class="px-4 py-3 font-medium">{alert.name}</td>
							<td class="px-4 py-3 text-xs text-muted-foreground">
								{metricLabel(alert.metric)}{alert.event_name ? ` (${alert.event_name})` : ''} &gt; {alert.threshold}
								in {alert.window_minutes >= 60 ? `${alert.window_minutes / 60}h` : `${alert.window_minutes}m`}
							</td>
							<td class="px-4 py-3">
								<span class="text-xs font-mono text-muted-foreground truncate max-w-[180px] block">{alert.webhook_url}</span>
							</td>
							<td class="px-4 py-3 text-xs text-muted-foreground">
								{alert.last_triggered_at ? relativeTime(alert.last_triggered_at) : 'Never'}
							</td>
							<td class="px-4 py-3">
								<button
									onclick={() => toggleEnabled(alert)}
									class="relative inline-flex h-5 w-9 items-center rounded-full transition-colors {alert.enabled ? 'bg-primary' : 'bg-muted-foreground/30'}"
									aria-label="Toggle alert"
								>
									<span class="inline-block h-3.5 w-3.5 rounded-full bg-white shadow transition-transform {alert.enabled ? 'translate-x-4.5' : 'translate-x-0.5'}"></span>
								</button>
							</td>
							<td class="px-4 py-3">
								<button onclick={() => handleDelete(alert.id)} class="text-xs text-destructive hover:underline">Delete</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
