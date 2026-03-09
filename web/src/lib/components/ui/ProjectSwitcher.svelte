<script lang="ts">
	import { switchProject, createNewProject } from '$lib/api';

	interface ProjectInfo {
		id: string;
		name: string;
	}

	let { projects = $bindable(), activeProject = $bindable() }: { projects: ProjectInfo[]; activeProject?: ProjectInfo } = $props();
	let open = $state(false);
	let creating = $state(false);
	let newName = $state('');

	async function select(id: string) {
		if (id === activeProject?.id) {
			open = false;
			return;
		}
		await switchProject(id);
		window.location.reload();
	}

	async function handleCreate() {
		if (!newName.trim()) return;
		const project = await createNewProject(newName.trim());
		projects = [...projects, { id: project.id, name: project.name }];
		newName = '';
		creating = false;
		await switchProject(project.id);
		window.location.reload();
	}
</script>

<div class="relative">
	<button
		onclick={() => (open = !open)}
		class="flex items-center gap-1.5 w-full px-2 py-1 rounded text-[11px] text-left border border-border hover:bg-accent transition-colors"
	>
		<span class="truncate flex-1 text-muted-foreground">{activeProject?.name ?? 'Select project'}</span>
		<svg class="w-3 h-3 shrink-0 text-muted-foreground/60" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
			<path stroke-linecap="round" stroke-linejoin="round" d="M8.25 15L12 18.75 15.75 15m-7.5-6L12 5.25 15.75 9" />
		</svg>
	</button>

	{#if open}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="fixed inset-0 z-[100]" onclick={() => { open = false; creating = false; }} onkeydown={() => {}}></div>
		<div class="absolute left-0 w-full mt-1 z-[101] border border-border rounded shadow-lg py-1 max-h-60 overflow-y-auto" style="background:#fff">
			{#each projects as project}
				<button
					onclick={() => select(project.id)}
					class="flex items-center gap-1.5 w-full px-2 py-1.5 text-[11px] text-left hover:bg-accent transition-colors {project.id === activeProject?.id ? 'text-primary font-medium' : 'text-foreground'}"
				>
					{#if project.id === activeProject?.id}
						<svg class="w-3 h-3 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
						</svg>
					{:else}
						<span class="w-3 shrink-0"></span>
					{/if}
					<span class="truncate">{project.name}</span>
				</button>
			{/each}

			<div class="border-t border-border mt-1 pt-1">
				{#if creating}
					<form onsubmit={(e) => { e.preventDefault(); handleCreate(); }} class="px-2 py-1 flex gap-1">
						<input
							bind:value={newName}
							placeholder="Name"
							class="flex-1 min-w-0 px-1.5 py-0.5 text-[11px] rounded border border-border bg-background text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:ring-1 focus:ring-primary"
						/>
						<button type="submit" class="px-1.5 py-0.5 text-[11px] rounded bg-primary text-primary-foreground hover:bg-primary/90">
							Add
						</button>
					</form>
				{:else}
					<button
						onclick={() => (creating = true)}
						class="flex items-center gap-1.5 w-full px-2 py-1.5 text-[11px] text-left text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
					>
						<svg class="w-3 h-3 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
						</svg>
						New project
					</button>
				{/if}
			</div>
		</div>
	{/if}
</div>
