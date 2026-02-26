<script lang="ts">
	import '../app.css';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import Logo from '$lib/components/Logo.svelte';

	let { children } = $props();

	const authRoutes = ['/login', '/setup', '/onboarding'];
	let isAuthRoute = $derived(authRoutes.includes($page.url.pathname));

	const navItems = [
		{
			href: '/',
			label: 'Overview',
			exact: true,
			icon: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6',
		},
		{
			href: '/analytics',
			label: 'Analytics',
			icon: 'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z',
		},
		{
			href: '/behavior',
			label: 'Behavior',
			icon: 'M15 15l-2 5L9 9l11 4-5 2zm0 0l5 5',
		},
		{
			href: '/people',
			label: 'People',
			icon: 'M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.501 20.118a7.5 7.5 0 0114.998 0A17.933 17.933 0 0112 21.75c-2.676 0-5.216-.584-7.499-1.632z',
		},
		{
			href: '/monitoring',
			label: 'Monitoring',
			icon: 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z',
		},
		{
			href: '/platform',
			label: 'Platform',
			icon: 'M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zm0 8a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zm10 0a1 1 0 011-1h4a1 1 0 011 1v6a1 1 0 01-1 1h-4a1 1 0 01-1-1v-6z',
		},
	];

	onMount(async () => {
		if (isAuthRoute) return;
		try {
			const res = await fetch('/api/v1/auth/me');
			if (res.status === 401) {
				// Check if setup is needed first.
				const setupRes = await fetch('/api/v1/auth/setup-required');
				const setup = await setupRes.json();
				goto(setup.required ? '/setup' : '/login');
			}
		} catch {
			goto('/login');
		}
	});

	async function logout() {
		await fetch('/api/v1/auth/logout', { method: 'POST' });
		goto('/login');
	}
</script>

{#if isAuthRoute}
	{@render children()}
{:else}
	<div class="flex h-screen bg-background">
		<!-- Sidebar -->
		<aside class="w-48 border-r border-border bg-card flex flex-col">
			<div class="p-4 border-b border-border">
				<a href="/" class="flex items-center gap-1">
					<Logo class="w-10 h-10 text-primary" />
					<h1 class="text-lg font-bold tracking-tight">
						<span class="text-primary">Click</span>Nest
					</h1>
				</a>
				<p class="text-xs text-muted-foreground mt-0.5">AI-native analytics</p>
			</div>
			<nav class="flex-1 p-2 space-y-0.5">
				{#each navItems as item}
					{@const active = item.exact
						? $page.url.pathname === item.href
						: $page.url.pathname === item.href || $page.url.pathname.startsWith(item.href + '/')}
					<a
						href={item.href}
						class="flex items-center gap-2.5 px-3 py-2 rounded-md text-sm transition-colors {active
							? 'bg-primary/10 text-primary font-medium'
							: 'text-muted-foreground hover:text-foreground hover:bg-accent'}"
					>
						<svg class="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
							<path stroke-linecap="round" stroke-linejoin="round" d={item.icon} />
						</svg>
						{item.label}
					</a>
				{/each}
			</nav>
			<div class="p-3 border-t border-border space-y-2">
				<button
					onclick={logout}
					class="flex items-center gap-2 w-full px-2 py-1.5 rounded-md text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
				>
					<svg class="w-3.5 h-3.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
						<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6a2.25 2.25 0 00-2.25 2.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15M12 9l-3 3m0 0l3 3m-3-3h12.75" />
					</svg>
					Sign out
				</button>
				<p class="text-[10px] text-muted-foreground px-2">v0.1.0</p>
			</div>
		</aside>

		<!-- Main content -->
		<main class="flex-1 overflow-hidden flex flex-col">
			{@render children()}
		</main>
	</div>
{/if}
