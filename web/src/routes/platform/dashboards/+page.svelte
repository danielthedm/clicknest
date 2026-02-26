<script lang="ts">
	import { onMount } from 'svelte';
	import { listDashboards, createDashboard, deleteDashboard } from '$lib/api';
	import { relativeTime } from '$lib/utils';
	import type { Dashboard } from '$lib/types';

	let dashboards = $state<Dashboard[]>([]);
	let loading = $state(true);

	// Create form
	let showCreate = $state(false);
	let newName = $state('');
	let newView = $state('events');
	let creating = $state(false);

	onMount(() => {
		loadDashboards();
	});

	async function loadDashboards() {
		loading = true;
		try {
			const res = await listDashboards();
			dashboards = res.dashboards ?? [];
		} catch (e) {
			console.error('Failed to load dashboards:', e);
		}
		loading = false;
	}

	async function handleDelete(id: string) {
		if (!confirm('Delete this dashboard?')) return;
		try {
			await deleteDashboard(id);
			await loadDashboards();
		} catch (e) {
			console.error('Failed to delete dashboard:', e);
		}
	}

	async function handleCreate() {
		if (!newName) return;
		creating = true;
		try {
			await createDashboard(newName, { view: newView, filters: {} });
			newName = '';
			newView = 'events';
			showCreate = false;
			await loadDashboards();
		} catch (e) {
			console.error('Failed to create dashboard:', e);
		}
		creating = false;
	}

	function parseConfig(config: string): { view: string; filters: Record<string, string> } {
		try {
			return JSON.parse(config);
		} catch {
			return { view: 'events', filters: {} };
		}
	}

	function dashboardUrl(dashboard: Dashboard): string {
		const cfg = parseConfig(dashboard.config);
		const base = `/${cfg.view || 'events'}`;
		const filters = cfg.filters || {};
		const params = new URLSearchParams(filters);
		const qs = params.toString();
		return qs ? `${base}?${qs}` : base;
	}

	function viewLabel(config: string): string {
		const cfg = parseConfig(config);
		return (cfg.view || 'events').charAt(0).toUpperCase() + (cfg.view || 'events').slice(1);
	}
</script>

<div class="p-6 max-w-6xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Dashboards</h2>
			<p class="text-sm text-muted-foreground mt-1">Saved views and filters</p>
		</div>
		<button
			onclick={() => showCreate = !showCreate}
			class="px-3 py-1.5 text-sm rounded-md border border-border hover:bg-accent"
		>
			{showCreate ? 'Cancel' : '+ New Dashboard'}
		</button>
	</div>

	{#if showCreate}
		<div class="border border-border rounded-lg p-4 bg-card mb-6">
			<h3 class="text-sm font-medium mb-3">Create Dashboard</h3>
			<div class="space-y-3">
				<input
					bind:value={newName}
					placeholder="Dashboard name"
					class="w-full px-3 py-1.5 text-sm border border-border rounded-md bg-background"
				/>
				<div class="flex gap-2 items-center">
					<span class="text-xs text-muted-foreground">View type</span>
					<select
						bind:value={newView}
						class="px-3 py-1.5 text-sm border border-border rounded-md bg-background"
					>
						<option value="events">Events</option>
						<option value="trends">Trends</option>
						<option value="funnels">Funnels</option>
						<option value="sessions">Sessions</option>
						<option value="retention">Retention</option>
					</select>
				</div>
				<button
					onclick={handleCreate}
					disabled={creating || !newName}
					class="px-3 py-1.5 text-sm rounded-md bg-primary text-primary-foreground disabled:opacity-50"
				>
					{creating ? 'Creating...' : 'Create Dashboard'}
				</button>
			</div>
		</div>
	{/if}

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
		</div>
	{:else if dashboards.length === 0}
		<div class="border border-border rounded-lg p-12 bg-card text-center">
			<p class="text-muted-foreground text-sm mb-4">No saved dashboards yet.</p>
			<p class="text-muted-foreground text-xs">Use the "Save View" button on the Events or Trends pages to save filters as a dashboard.</p>
		</div>
	{:else}
		<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
			{#each dashboards as dashboard}
				<div class="border border-border rounded-lg bg-card p-4 hover:border-primary/50 transition-colors">
					<div class="flex items-start justify-between mb-2">
						<a href={dashboardUrl(dashboard)} class="text-sm font-medium hover:text-primary transition-colors">
							{dashboard.name}
						</a>
						<button
							onclick={() => handleDelete(dashboard.id)}
							class="text-xs text-red-500 hover:text-red-700"
						>Delete</button>
					</div>
					<div class="flex items-center gap-2">
						<span class="inline-flex items-center px-2 py-0.5 rounded text-[10px] font-medium bg-muted text-muted-foreground">
							{viewLabel(dashboard.config)}
						</span>
						<span class="text-xs text-muted-foreground">{relativeTime(dashboard.updated_at)}</span>
					</div>
					<a href={dashboardUrl(dashboard)} class="mt-3 inline-block text-xs text-primary hover:underline">
						Open view &rarr;
					</a>
				</div>
			{/each}
		</div>
	{/if}
</div>
