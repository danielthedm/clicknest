<script lang="ts">
	import { onMount } from 'svelte';
	import { listExperiments, createExperiment, deleteExperiment, listFlags, listConversionGoals } from '$lib/api';
	import type { Experiment, FeatureFlag, ConversionGoal } from '$lib/types';
	import Select from '$lib/components/ui/Select.svelte';

	let experiments: Experiment[] = $state([]);
	let flags: FeatureFlag[] = $state([]);
	let goals: ConversionGoal[] = $state([]);
	let loading = $state(true);
	let showCreate = $state(false);

	let form = $state({
		name: '',
		flag_key: '',
		variants: 'control,variant_a',
		conversion_goal_id: '',
		auto_stop: false,
	});

	onMount(load);

	async function load() {
		loading = true;
		try {
			const [expRes, flagRes, goalRes] = await Promise.all([
				listExperiments(),
				listFlags(),
				listConversionGoals(),
			]);
			experiments = expRes.experiments ?? [];
			flags = flagRes.flags ?? [];
			goals = goalRes.goals ?? [];
		} catch {}
		loading = false;
	}

	async function handleCreate() {
		if (!form.name || !form.flag_key) return;
		const variants = form.variants.split(',').map((v) => v.trim()).filter(Boolean);
		if (variants.length < 2) return;
		try {
			await createExperiment({
				name: form.name,
				flag_key: form.flag_key,
				variants,
				conversion_goal_id: form.conversion_goal_id || undefined,
				auto_stop: form.auto_stop,
			});
			form = { name: '', flag_key: '', variants: 'control,variant_a', conversion_goal_id: '', auto_stop: false };
			showCreate = false;
			await load();
		} catch {}
	}

	async function handleDelete(id: string) {
		try {
			await deleteExperiment(id);
			await load();
		} catch {}
	}

	function statusBadge(status: string): string {
		switch (status) {
			case 'running': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
			case 'paused': return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400';
			case 'completed': return 'bg-muted text-muted-foreground';
			default: return 'bg-muted text-muted-foreground';
		}
	}
</script>

<div class="p-6 max-w-5xl mx-auto space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold text-foreground">Experiments</h1>
			<p class="text-sm text-muted-foreground mt-1">A/B tests with statistical significance, confidence intervals, and auto-stop.</p>
		</div>
		<button onclick={() => showCreate = !showCreate} class="px-4 py-2 bg-primary text-primary-foreground rounded-md text-sm font-medium hover:bg-primary/90">
			{showCreate ? 'Cancel' : 'New Experiment'}
		</button>
	</div>

	{#if showCreate}
		<div class="border border-border rounded-lg p-4 space-y-4 bg-card">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label class="block text-sm font-medium text-foreground mb-1">Name</label>
					<input bind:value={form.name} placeholder="e.g. CTA Button Color Test" class="w-full px-3 py-2 rounded-md border border-input bg-background text-sm" />
				</div>
				<div>
					<Select
						bind:value={form.flag_key}
						options={[
							{ value: '', label: 'Select a flag...' },
							...flags.map(flag => ({ value: flag.key, label: `${flag.name} (${flag.key})` })),
						]}
						label="Feature Flag Key"
						size="md"
					/>
				</div>
				<div>
					<label class="block text-sm font-medium text-foreground mb-1">Variants (comma-separated)</label>
					<input bind:value={form.variants} placeholder="control,variant_a,variant_b" class="w-full px-3 py-2 rounded-md border border-input bg-background text-sm" />
				</div>
				<div>
					<Select
						bind:value={form.conversion_goal_id}
						options={[
							{ value: '', label: 'None (use exposures only)' },
							...goals.map(goal => ({ value: goal.id, label: goal.name })),
						]}
						label="Conversion Goal (optional)"
						size="md"
					/>
				</div>
			</div>
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={form.auto_stop} class="rounded border-input" />
				<span class="text-foreground">Auto-stop when statistical significance is reached</span>
			</label>
			<button onclick={handleCreate} class="px-4 py-2 bg-primary text-primary-foreground rounded-md text-sm font-medium hover:bg-primary/90">Create Experiment</button>
		</div>
	{/if}

	{#if loading}
		<div class="text-sm text-muted-foreground">Loading...</div>
	{:else if experiments.length === 0}
		<div class="text-center py-12 text-muted-foreground">
			<p class="text-lg font-medium">No experiments yet</p>
			<p class="text-sm mt-1">Create an experiment to start A/B testing with feature flags.</p>
		</div>
	{:else}
		<div class="border border-border rounded-lg overflow-hidden">
			<table class="w-full text-sm">
				<thead class="bg-muted/50">
					<tr>
						<th class="text-left px-4 py-3 font-medium text-muted-foreground">Name</th>
						<th class="text-left px-4 py-3 font-medium text-muted-foreground">Flag Key</th>
						<th class="text-left px-4 py-3 font-medium text-muted-foreground">Variants</th>
						<th class="text-left px-4 py-3 font-medium text-muted-foreground">Status</th>
						<th class="text-left px-4 py-3 font-medium text-muted-foreground">Winner</th>
						<th class="text-right px-4 py-3 font-medium text-muted-foreground">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each experiments as exp}
						{@const variants = (() => { try { return JSON.parse(exp.variants); } catch { return []; } })()}
						<tr class="border-t border-border hover:bg-muted/30">
							<td class="px-4 py-3">
								<a href="/platform/experiments/{exp.id}" class="text-primary hover:underline font-medium">{exp.name}</a>
							</td>
							<td class="px-4 py-3 font-mono text-xs text-muted-foreground">{exp.flag_key}</td>
							<td class="px-4 py-3 text-muted-foreground">{variants.length} variants</td>
							<td class="px-4 py-3">
								<span class="inline-block px-2 py-0.5 rounded text-xs font-medium {statusBadge(exp.status)}">{exp.status}</span>
								{#if exp.auto_stop}
									<span class="inline-block px-1.5 py-0.5 rounded text-xs bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400 ml-1">auto</span>
								{/if}
							</td>
							<td class="px-4 py-3 text-muted-foreground">{exp.winner_variant || '-'}</td>
							<td class="px-4 py-3 text-right">
								<button onclick={() => handleDelete(exp.id)} class="text-xs text-destructive hover:underline">Delete</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
