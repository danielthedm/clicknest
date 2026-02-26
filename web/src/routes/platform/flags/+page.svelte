<script lang="ts">
	import { onMount } from 'svelte';
	import { listFlags, createFlag, updateFlag, deleteFlag } from '$lib/api';
	import { relativeTime } from '$lib/utils';
	import type { FeatureFlag } from '$lib/types';

	let flags = $state<FeatureFlag[]>([]);
	let loading = $state(true);
	let showForm = $state(false);
	let error = $state('');

	// New flag form
	let newName = $state('');
	let newKey = $state('');
	let newRollout = $state(100);
	let creating = $state(false);

	onMount(() => load());

	async function load() {
		loading = true;
		try {
			const res = await listFlags();
			flags = res.flags ?? [];
		} catch (e) {
			console.error('Failed to load flags:', e);
		}
		loading = false;
	}

	function slugify(s: string): string {
		return s.toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9-_]/g, '');
	}

	function onNameInput() {
		if (!newKey || newKey === slugify(newName.slice(0, -1))) {
			newKey = slugify(newName);
		}
	}

	async function handleCreate() {
		if (!newName.trim() || !newKey.trim()) return;
		creating = true;
		error = '';
		try {
			await createFlag(newKey, newName, newRollout);
			newName = '';
			newKey = '';
			newRollout = 100;
			showForm = false;
			await load();
		} catch (e: any) {
			error = e.message ?? 'Create failed';
		}
		creating = false;
	}

	async function toggleEnabled(flag: FeatureFlag) {
		try {
			await updateFlag(flag.id, !flag.enabled, flag.rollout_percentage);
			flag.enabled = !flag.enabled;
			flags = [...flags];
		} catch (e) {
			console.error('Failed to update flag:', e);
		}
	}

	async function updateRollout(flag: FeatureFlag, pct: number) {
		try {
			await updateFlag(flag.id, flag.enabled, pct);
			flag.rollout_percentage = pct;
			flags = [...flags];
		} catch (e) {
			console.error('Failed to update rollout:', e);
		}
	}

	async function handleDelete(id: string) {
		if (!confirm('Delete this feature flag?')) return;
		try {
			await deleteFlag(id);
			flags = flags.filter(f => f.id !== id);
		} catch (e) {
			console.error('Failed to delete flag:', e);
		}
	}
</script>

<div class="p-6 max-w-4xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Feature Flags</h2>
			<p class="text-sm text-muted-foreground mt-1">Control feature rollout with percentage-based targeting</p>
		</div>
		<button
			onclick={() => { showForm = !showForm; error = ''; }}
			class="px-3 py-1.5 text-sm rounded-md bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
		>
			+ New Flag
		</button>
	</div>

	{#if showForm}
		<div class="border border-border rounded-lg p-4 bg-card mb-6">
			<h3 class="text-sm font-medium mb-3">Create Flag</h3>
			<div class="grid grid-cols-2 gap-3 mb-3">
				<div>
					<label class="text-xs text-muted-foreground block mb-1">Name</label>
					<input
						bind:value={newName}
						oninput={onNameInput}
						placeholder="My Feature"
						class="w-full px-2 py-1.5 text-sm border border-border rounded bg-background"
					/>
				</div>
				<div>
					<label class="text-xs text-muted-foreground block mb-1">Key (slug)</label>
					<input
						bind:value={newKey}
						placeholder="my-feature"
						class="w-full px-2 py-1.5 text-sm font-mono border border-border rounded bg-background"
					/>
				</div>
			</div>
			<div class="mb-3">
				<label class="text-xs text-muted-foreground block mb-1">Rollout: {newRollout}%</label>
				<input type="range" min="0" max="100" bind:value={newRollout} class="w-full" />
			</div>
			{#if error}
				<p class="text-xs text-destructive mb-2">{error}</p>
			{/if}
			<div class="flex gap-2">
				<button
					onclick={handleCreate}
					disabled={creating || !newName.trim() || !newKey.trim()}
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
	{:else if flags.length === 0}
		<div class="border border-border rounded-lg p-12 bg-card text-center">
			<p class="text-muted-foreground">No feature flags yet. Create one to get started.</p>
			<p class="text-xs text-muted-foreground mt-2">Check flags in your app with <code class="font-mono">ClickNest.isEnabled('flag-key')</code></p>
		</div>
	{:else}
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b border-border bg-muted/50">
						<th class="text-left px-4 py-2 text-xs font-medium text-muted-foreground">Name / Key</th>
						<th class="text-left px-4 py-2 text-xs font-medium text-muted-foreground">Rollout</th>
						<th class="text-left px-4 py-2 text-xs font-medium text-muted-foreground">Enabled</th>
						<th class="text-left px-4 py-2 text-xs font-medium text-muted-foreground">Created</th>
						<th class="px-4 py-2"></th>
					</tr>
				</thead>
				<tbody class="divide-y divide-border">
					{#each flags as flag}
						<tr class="hover:bg-accent/30 transition-colors">
							<td class="px-4 py-3">
								<p class="font-medium">{flag.name}</p>
								<p class="text-xs font-mono text-muted-foreground mt-0.5">{flag.key}</p>
							</td>
							<td class="px-4 py-3">
								<div class="flex items-center gap-2">
									<input
										type="range" min="0" max="100"
										value={flag.rollout_percentage}
										onchange={(e) => updateRollout(flag, parseInt((e.target as HTMLInputElement).value))}
										class="w-24"
									/>
									<span class="text-xs tabular-nums w-8">{flag.rollout_percentage}%</span>
								</div>
							</td>
							<td class="px-4 py-3">
								<button
									onclick={() => toggleEnabled(flag)}
									class="relative inline-flex h-5 w-9 items-center rounded-full transition-colors {flag.enabled ? 'bg-primary' : 'bg-muted-foreground/30'}"
									aria-label="Toggle flag"
								>
									<span class="inline-block h-3.5 w-3.5 rounded-full bg-white shadow transition-transform {flag.enabled ? 'translate-x-4.5' : 'translate-x-0.5'}"></span>
								</button>
							</td>
							<td class="px-4 py-3 text-xs text-muted-foreground">{relativeTime(flag.created_at)}</td>
							<td class="px-4 py-3">
								<button
									onclick={() => handleDelete(flag.id)}
									class="text-xs text-destructive hover:underline"
								>Delete</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
