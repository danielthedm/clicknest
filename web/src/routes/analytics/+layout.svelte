<script lang="ts">
	import { page } from '$app/stores';
	let { children } = $props();

	const tabs = [
		{ href: '/analytics/events', label: 'Events' },
		{ href: '/analytics/pages', label: 'Pages' },
		{ href: '/analytics/trends', label: 'Trends' },
		{ href: '/analytics/retention', label: 'Retention' },
	];
</script>

<div class="flex flex-col h-full">
	<div class="border-b border-border px-6 pt-4">
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
	</div>
	<div class="flex-1 overflow-y-auto">
		{@render children()}
	</div>
</div>
