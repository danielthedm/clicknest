<script lang="ts">
	import { onMount } from 'svelte';
	import { listConversionGoals, createConversionGoal, deleteConversionGoal } from '$lib/api';
	import type { ConversionGoal } from '$lib/types';

	let goals: ConversionGoal[] = $state([]);
	let loading = $state(true);
	let showCreate = $state(false);
	let form = $state({ name: '', event_type: 'custom', event_name: '', url_pattern: '', value_property: '$value' });

	const presetTypes = [
		{ label: 'Custom Event', value: 'custom' },
		{ label: 'Signup', value: 'identify' },
		{ label: 'Pageview', value: 'pageview' },
	];

	onMount(load);

	async function load() {
		loading = true;
		try {
			const res = await listConversionGoals();
			goals = res.goals ?? [];
		} catch {}
		loading = false;
	}

	async function handleCreate() {
		if (!form.name) return;
		try {
			await createConversionGoal(form);
			form = { name: '', event_type: 'custom', event_name: '', url_pattern: '', value_property: '$value' };
			showCreate = false;
			await load();
		} catch {}
	}

	async function handleDelete(id: string) {
		try {
			await deleteConversionGoal(id);
			await load();
		} catch {}
	}
</script>

<div class="p-6 max-w-4xl mx-auto space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold text-foreground">Conversion Goals</h1>
			<p class="text-sm text-muted-foreground mt-1">Define named conversion events to track revenue and attribution.</p>
		</div>
		<button onclick={() => showCreate = !showCreate} class="px-4 py-2 bg-primary text-primary-foreground rounded-md text-sm font-medium hover:bg-primary/90">
			{showCreate ? 'Cancel' : 'New Goal'}
		</button>
	</div>

	{#if showCreate}
		<div class="border border-border rounded-lg p-4 space-y-4 bg-card">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label class="block text-sm font-medium text-foreground mb-1">Name</label>
					<input bind:value={form.name} placeholder="e.g. Purchase, Signup" class="w-full px-3 py-2 rounded-md border border-input bg-background text-sm" />
				</div>
				<div>
					<label class="block text-sm font-medium text-foreground mb-1">Event Type</label>
					<select bind:value={form.event_type} class="w-full px-3 py-2 rounded-md border border-input bg-background text-sm">
						{#each presetTypes as t}
							<option value={t.value}>{t.label}</option>
						{/each}
					</select>
				</div>
				<div>
					<label class="block text-sm font-medium text-foreground mb-1">Event Name (optional)</label>
					<input bind:value={form.event_name} placeholder="e.g. purchase, upgrade" class="w-full px-3 py-2 rounded-md border border-input bg-background text-sm" />
				</div>
				<div>
					<label class="block text-sm font-medium text-foreground mb-1">URL Pattern (optional)</label>
					<input bind:value={form.url_pattern} placeholder="e.g. /thank-you%" class="w-full px-3 py-2 rounded-md border border-input bg-background text-sm" />
				</div>
				<div>
					<label class="block text-sm font-medium text-foreground mb-1">Revenue Property</label>
					<input bind:value={form.value_property} class="w-full px-3 py-2 rounded-md border border-input bg-background text-sm" />
				</div>
			</div>
			<button onclick={handleCreate} class="px-4 py-2 bg-primary text-primary-foreground rounded-md text-sm font-medium hover:bg-primary/90">Create Goal</button>
		</div>
	{/if}

	{#if loading}
		<div class="text-sm text-muted-foreground">Loading...</div>
	{:else if goals.length === 0}
		<div class="text-center py-12 text-muted-foreground">
			<p class="text-lg font-medium">No conversion goals yet</p>
			<p class="text-sm mt-1">Create a goal to start tracking conversions and revenue attribution.</p>
		</div>
	{:else}
		<div class="border border-border rounded-lg overflow-hidden">
			<table class="w-full text-sm">
				<thead class="bg-muted/50">
					<tr>
						<th class="text-left px-4 py-3 font-medium text-muted-foreground">Name</th>
						<th class="text-left px-4 py-3 font-medium text-muted-foreground">Event</th>
						<th class="text-left px-4 py-3 font-medium text-muted-foreground">Revenue Key</th>
						<th class="text-left px-4 py-3 font-medium text-muted-foreground">Created</th>
						<th class="text-right px-4 py-3 font-medium text-muted-foreground">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each goals as goal}
						<tr class="border-t border-border hover:bg-muted/30">
							<td class="px-4 py-3">
								<a href="/growth/conversions/{goal.id}" class="text-primary hover:underline font-medium">{goal.name}</a>
							</td>
							<td class="px-4 py-3 text-muted-foreground">
								{goal.event_type}{goal.event_name ? `: ${goal.event_name}` : ''}
							</td>
							<td class="px-4 py-3 text-muted-foreground font-mono text-xs">{goal.value_property}</td>
							<td class="px-4 py-3 text-muted-foreground">{new Date(goal.created_at).toLocaleDateString()}</td>
							<td class="px-4 py-3 text-right">
								<button onclick={() => handleDelete(goal.id)} class="text-xs text-destructive hover:underline">Delete</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
