<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import Logo from '$lib/components/Logo.svelte';
	import { getProject, updateLLMConfig, connectGitHub } from '$lib/api';

	let step = $state(1);
	let completedSteps = $state(new Set<number>());

	let project = $state<{ id: string; name: string; api_key: string } | null>(null);
	let origin = $state('');

	// AI config
	let provider = $state<'openai' | 'anthropic' | 'ollama'>('openai');
	let aiApiKey = $state('');
	let model = $state('gpt-4o-mini');
	let baseURL = $state('http://localhost:11434');

	// GitHub config
	let repoOwner = $state('');
	let repoName = $state('');
	let accessToken = $state('');
	let defaultBranch = $state('main');

	let loading = $state(false);
	let error = $state('');
	let copied = $state(false);

	const defaultModels: Record<string, string> = {
		openai: 'gpt-4o-mini',
		anthropic: 'claude-haiku-4-5-20251001',
		ollama: 'llama3',
	};

	function setProvider(p: 'openai' | 'anthropic' | 'ollama') {
		provider = p;
		model = defaultModels[p];
		error = '';
	}

	const snippet = $derived(
		project
			? `<script src="${origin}/sdk.js"\n  data-api-key="${project.api_key}"\n  data-host="${origin}"><\/script>`
			: `<script src="${origin}/sdk.js"\n  data-api-key="loading..."\n  data-host="${origin}"><\/script>`
	);

	onMount(async () => {
		origin = window.location.origin;
		try {
			project = await getProject();
		} catch {}
	});

	async function copySnippet() {
		try {
			await navigator.clipboard.writeText(snippet);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {}
	}

	function advanceTo(n: number) {
		completedSteps = new Set([...completedSteps, step]);
		step = n;
		error = '';
	}

	async function saveAI() {
		if (provider !== 'ollama' && !aiApiKey.trim()) {
			error = 'API key is required';
			return;
		}
		loading = true;
		error = '';
		try {
			await updateLLMConfig({
				provider,
				api_key: aiApiKey,
				model: model || defaultModels[provider],
				base_url: provider === 'ollama' ? baseURL : undefined,
			});
			advanceTo(3);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to save configuration';
		} finally {
			loading = false;
		}
	}

	async function saveGitHub() {
		if (!repoOwner.trim() || !repoName.trim() || !accessToken.trim()) {
			error = 'Owner, repository, and access token are required';
			return;
		}
		loading = true;
		error = '';
		try {
			await connectGitHub({
				repo_owner: repoOwner,
				repo_name: repoName,
				access_token: accessToken,
				default_branch: defaultBranch || 'main',
			});
			goto('/');
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to connect repository';
		} finally {
			loading = false;
		}
	}

	const stepDefs = [
		{ n: 1, label: 'Install SDK' },
		{ n: 2, label: 'AI Naming' },
		{ n: 3, label: 'GitHub' },
	];
</script>

<div class="min-h-screen bg-background flex items-center justify-center p-4">
	<div class="w-full max-w-lg">

		<!-- Logo -->
		<div class="flex items-center justify-center gap-2 mb-8">
			<Logo class="w-10 h-10 text-primary" />
			<h1 class="text-xl font-bold tracking-tight">
				<span class="text-primary">Click</span>Nest
			</h1>
		</div>

		<!-- Step indicator -->
		<div class="flex items-center justify-center mb-6 gap-0">
			{#each stepDefs as s, i}
				{@const done = completedSteps.has(s.n)}
				{@const current = step === s.n}
				<div class="flex items-center gap-0">
					<div class="flex items-center gap-2">
						<div class="w-6 h-6 rounded-full flex items-center justify-center text-xs font-semibold shrink-0 transition-colors
							{done ? 'bg-primary text-primary-foreground' : current ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground border border-border'}">
							{#if done}
								<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
									<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
								</svg>
							{:else}
								{s.n}
							{/if}
						</div>
						<span class="text-xs font-medium {current ? 'text-foreground' : 'text-muted-foreground'}">{s.label}</span>
					</div>
					{#if i < stepDefs.length - 1}
						<div class="w-8 h-px bg-border mx-3"></div>
					{/if}
				</div>
			{/each}
		</div>

		<!-- Card -->
		<div class="border border-border rounded-xl bg-card p-6">

			<!-- Step 1: Install SDK -->
			{#if step === 1}
				<h2 class="text-base font-semibold mb-1">Install the ClickNest SDK</h2>
				<p class="text-xs text-muted-foreground mb-5">
					Paste this snippet in the <code class="font-mono text-foreground bg-accent px-1 py-0.5 rounded">&lt;head&gt;</code> of your HTML. It auto-tracks pageviews and clicks.
				</p>

				<div class="relative rounded-lg border border-border bg-accent/50 mb-5">
					<div class="flex items-center justify-between px-3 pt-2.5 pb-1">
						<span class="text-[10px] font-medium text-muted-foreground uppercase tracking-wide">HTML</span>
						<button
							onclick={copySnippet}
							class="text-xs px-2 py-0.5 rounded border border-border bg-background text-muted-foreground hover:text-foreground transition-colors"
						>
							{copied ? '✓ Copied' : 'Copy'}
						</button>
					</div>
					<pre class="px-3 pb-3 text-xs font-mono text-foreground overflow-x-auto whitespace-pre leading-relaxed">{snippet}</pre>
				</div>

				<button
					onclick={() => advanceTo(2)}
					class="w-full py-2 text-sm rounded-md bg-primary text-primary-foreground hover:bg-primary/90 transition-colors font-medium mb-3"
				>
					Looks good, continue →
				</button>
				<div class="text-center">
					<button onclick={() => advanceTo(2)} class="text-xs text-muted-foreground hover:text-foreground transition-colors">
						I'll set this up later
					</button>
				</div>

			<!-- Step 2: AI Event Naming -->
			{:else if step === 2}
				<h2 class="text-base font-semibold mb-1">Enable AI event naming</h2>
				<p class="text-xs text-muted-foreground mb-5">
					ClickNest uses an LLM to automatically name raw click events. You can configure this any time in Settings.
				</p>

				<!-- Provider selector -->
				<div class="mb-4">
					<label class="text-xs text-muted-foreground block mb-1.5">Provider</label>
					<div class="flex gap-2">
						{#each [['openai', 'OpenAI'], ['anthropic', 'Anthropic'], ['ollama', 'Ollama']] as [val, label]}
							<button
								onclick={() => setProvider(val as 'openai' | 'anthropic' | 'ollama')}
								class="flex-1 py-1.5 text-xs rounded-md border transition-colors font-medium
									{provider === val ? 'border-primary bg-primary/5 text-primary' : 'border-border text-muted-foreground hover:text-foreground hover:border-foreground/30'}"
							>
								{label}
							</button>
						{/each}
					</div>
				</div>

				<!-- API key (not for Ollama) -->
				{#if provider !== 'ollama'}
					<div class="mb-4">
						<label for="ai-key" class="text-xs text-muted-foreground block mb-1">API Key</label>
						<input
							id="ai-key"
							type="password"
							bind:value={aiApiKey}
							oninput={() => (error = '')}
							placeholder={provider === 'openai' ? 'sk-...' : 'sk-ant-...'}
							autocomplete="off"
							class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-primary/30"
						/>
					</div>
				{/if}

				<!-- Model -->
				<div class="mb-4">
					<label for="ai-model" class="text-xs text-muted-foreground block mb-1">Model</label>
					<input
						id="ai-model"
						type="text"
						bind:value={model}
						oninput={() => (error = '')}
						class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-primary/30"
					/>
				</div>

				<!-- Base URL (Ollama only) -->
				{#if provider === 'ollama'}
					<div class="mb-4">
						<label for="ai-baseurl" class="text-xs text-muted-foreground block mb-1">Base URL</label>
						<input
							id="ai-baseurl"
							type="text"
							bind:value={baseURL}
							oninput={() => (error = '')}
							class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-primary/30"
						/>
					</div>
				{/if}

				{#if error}
					<p class="text-xs text-destructive mb-3">{error}</p>
				{/if}

				<button
					onclick={saveAI}
					disabled={loading}
					class="w-full py-2 text-sm rounded-md bg-primary text-primary-foreground hover:bg-primary/90 transition-colors font-medium disabled:opacity-50 mb-2"
				>
					{loading ? 'Saving…' : 'Save & continue →'}
				</button>
				<button
					onclick={() => advanceTo(3)}
					class="w-full py-2 text-sm rounded-md border border-border text-muted-foreground hover:text-foreground hover:border-foreground/30 transition-colors"
				>
					Skip for now
				</button>

			<!-- Step 3: GitHub -->
			{:else if step === 3}
				<h2 class="text-base font-semibold mb-1">Connect your GitHub repo</h2>
				<p class="text-xs text-muted-foreground mb-5">
					ClickNest maps click events to the source files that render them, for richer event names.
				</p>

				<!-- Owner + Repo row -->
				<div class="flex gap-3 mb-4">
					<div class="flex-1">
						<label for="gh-owner" class="text-xs text-muted-foreground block mb-1">Owner</label>
						<input
							id="gh-owner"
							type="text"
							bind:value={repoOwner}
							oninput={() => (error = '')}
							placeholder="acme"
							class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-primary/30"
						/>
					</div>
					<div class="flex-1">
						<label for="gh-repo" class="text-xs text-muted-foreground block mb-1">Repository</label>
						<input
							id="gh-repo"
							type="text"
							bind:value={repoName}
							oninput={() => (error = '')}
							placeholder="my-app"
							class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-primary/30"
						/>
					</div>
				</div>

				<div class="mb-4">
					<label for="gh-token" class="text-xs text-muted-foreground block mb-1">Personal access token</label>
					<input
						id="gh-token"
						type="password"
						bind:value={accessToken}
						oninput={() => (error = '')}
						placeholder="github_pat_..."
						autocomplete="off"
						class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-primary/30"
					/>
					<p class="text-[11px] text-muted-foreground mt-1.5">
						Token needs read access to code. Create one at
						<a href="https://github.com/settings/tokens" target="_blank" rel="noopener" class="text-primary hover:underline">
							github.com/settings/tokens
						</a>
					</p>
				</div>

				<div class="mb-4">
					<label for="gh-branch" class="text-xs text-muted-foreground block mb-1">Default branch</label>
					<input
						id="gh-branch"
						type="text"
						bind:value={defaultBranch}
						oninput={() => (error = '')}
						class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-primary/30"
					/>
				</div>

				{#if error}
					<p class="text-xs text-destructive mb-3">{error}</p>
				{/if}

				<button
					onclick={saveGitHub}
					disabled={loading}
					class="w-full py-2 text-sm rounded-md bg-primary text-primary-foreground hover:bg-primary/90 transition-colors font-medium disabled:opacity-50 mb-2"
				>
					{loading ? 'Connecting…' : 'Connect & finish →'}
				</button>
				<button
					onclick={() => goto('/')}
					class="w-full py-2 text-sm rounded-md border border-border text-muted-foreground hover:text-foreground hover:border-foreground/30 transition-colors"
				>
					Skip for now
				</button>
			{/if}

		</div>

		<!-- Skip all link -->
		{#if step === 1}
			<p class="text-center mt-4 text-xs text-muted-foreground">
				Already set up? <a href="/" class="text-primary hover:underline">Go to dashboard</a>
			</p>
		{/if}

	</div>
</div>
