<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import Logo from '$lib/components/Logo.svelte';

	let email = $state('');
	let password = $state('');
	let confirm = $state('');
	let error = $state('');
	let loading = $state(false);

	onMount(async () => {
		const res = await fetch('/api/v1/auth/setup-required');
		const data = await res.json();
		if (!data.required) goto('/login');
	});

	async function handleSetup(e: Event) {
		e.preventDefault();
		if (password !== confirm) {
			error = 'Passwords do not match';
			return;
		}
		loading = true;
		error = '';
		try {
			const res = await fetch('/api/v1/auth/setup', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email, password }),
			});
			if (!res.ok) {
				const body = await res.json();
				error = body.error || 'Setup failed';
				return;
			}
			goto('/onboarding');
		} catch {
			error = 'Something went wrong. Please try again.';
		} finally {
			loading = false;
		}
	}
</script>

<div class="min-h-screen bg-background flex items-center justify-center p-4">
	<div class="w-full max-w-sm">
		<div class="flex items-center justify-center gap-2 mb-8">
			<Logo class="w-10 h-10 text-primary" />
			<h1 class="text-xl font-bold tracking-tight">
				<span class="text-primary">Click</span>Nest
			</h1>
		</div>

		<div class="border border-border rounded-xl bg-card p-6">
			<h2 class="text-base font-semibold mb-1">Create your account</h2>
			<p class="text-xs text-muted-foreground mb-5">Set up your admin account to get started. This screen only appears once.</p>

			<form onsubmit={handleSetup} class="space-y-4">
				<div>
					<label for="email" class="text-xs text-muted-foreground block mb-1">Email</label>
					<input
						id="email"
						type="email"
						bind:value={email}
						required
						autocomplete="email"
						placeholder="you@example.com"
						class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-primary/30"
					/>
				</div>
				<div>
					<label for="password" class="text-xs text-muted-foreground block mb-1">Password <span class="text-muted-foreground/60">(min 8 characters)</span></label>
					<input
						id="password"
						type="password"
						bind:value={password}
						required
						minlength="8"
						autocomplete="new-password"
						placeholder="••••••••"
						class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-primary/30"
					/>
				</div>
				<div>
					<label for="confirm" class="text-xs text-muted-foreground block mb-1">Confirm password</label>
					<input
						id="confirm"
						type="password"
						bind:value={confirm}
						required
						autocomplete="new-password"
						placeholder="••••••••"
						class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-primary/30"
					/>
				</div>

				{#if error}
					<p class="text-xs text-destructive">{error}</p>
				{/if}

				<button
					type="submit"
					disabled={loading}
					class="w-full py-2 text-sm rounded-md bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50 font-medium"
				>
					{loading ? 'Creating account…' : 'Create account'}
				</button>
			</form>
		</div>
	</div>
</div>
