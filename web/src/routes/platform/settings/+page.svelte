<script lang="ts">
	import { onMount } from 'svelte';
	import { getProject, getLLMConfig, updateLLMConfig, getGitHub, connectGitHub, getGitHubOAuthURL, exportBackupURL, importBackup, getStorage } from '$lib/api';
	import type { Project, GitHubConnection, StorageInfo } from '$lib/types';

	let project = $state<Project | null>(null);
	let loading = $state(true);
	let copied = $state(false);

	let llmProvider = $state('openai');
	let llmApiKey = $state('');
	let llmModel = $state('gpt-4o-mini');
	let llmBaseUrl = $state('');
	let llmApiKeySet = $state(false); // true if a key is already saved
	let llmApiKeyHint = $state(''); // masked key like "sk-ant-...a1b2"
	let saving = $state(false);
	let saved = $state(false);

	let github = $state<GitHubConnection | null>(null);
	let ghOwner = $state('');
	let ghRepo = $state('');
	let ghToken = $state('');
	let ghBranch = $state('main');
	let ghSaving = $state(false);
	let ghSaved = $state(false);
	let ghError = $state('');
	let oauthConnecting = $state(false);
	let oauthJustConnected = $state(false);

	let storage = $state<StorageInfo | null>(null);

	let importFile = $state<File | null>(null);
	let importing = $state(false);
	let importMessage = $state('');
	let importError = $state('');

	async function handleImport() {
		if (!importFile) return;
		importing = true;
		importMessage = '';
		importError = '';
		try {
			const result = await importBackup(importFile);
			importMessage = result.message;
		} catch (e: unknown) {
			importError = e instanceof Error ? e.message : 'Import failed';
		} finally {
			importing = false;
		}
	}

	function fmtBytes(n: number): string {
		if (n >= 1_073_741_824) return (n / 1_073_741_824).toFixed(2) + ' GB';
		if (n >= 1_048_576) return (n / 1_048_576).toFixed(1) + ' MB';
		if (n >= 1_024) return (n / 1_024).toFixed(0) + ' KB';
		return n + ' B';
	}

	onMount(async () => {
		try {
			const [proj, gh, llm, stor] = await Promise.all([getProject(), getGitHub(), getLLMConfig(), getStorage()]);
			storage = stor;
			project = proj;
			github = gh;
			if (gh.connected) {
				ghOwner = gh.repo_owner ?? '';
				ghRepo = gh.repo_name ?? '';
				ghBranch = gh.default_branch ?? 'main';
			}
			// Populate LLM form with saved values
			if (llm.provider) {
				llmProvider = llm.provider;
				llmModel = llm.model || (models[llm.provider]?.[0] ?? '');
				llmBaseUrl = llm.base_url || '';
				llmApiKeySet = llm.api_key_set;
				llmApiKeyHint = llm.api_key_hint || '';
			}

			// Detect OAuth callback redirect.
			const params = new URLSearchParams(window.location.search);
			if (params.get('github') === 'connected') {
				oauthJustConnected = true;
				// Refresh connection status.
				const freshGh = await getGitHub();
				github = freshGh;
				if (freshGh.connected) {
					ghOwner = freshGh.repo_owner ?? '';
					ghRepo = freshGh.repo_name ?? '';
					ghBranch = freshGh.default_branch ?? 'main';
				}
				// Clean up URL.
				window.history.replaceState({}, '', '/settings');
			}
		} catch (e) {
			console.error('Failed to load settings:', e);
		}
		loading = false;
	});

	function copyApiKey() {
		if (!project) return;
		navigator.clipboard.writeText(project.api_key);
		copied = true;
		setTimeout(() => { copied = false; }, 2000);
	}

	async function saveLLMConfig() {
		saving = true;
		try {
			await updateLLMConfig({
				provider: llmProvider,
				api_key: llmApiKey.trim() || undefined,
				model: llmModel,
				base_url: llmBaseUrl || undefined,
			});
			if (llmApiKey.trim()) llmApiKeySet = true;
			llmApiKey = '';
			saved = true;
			setTimeout(() => { saved = false; }, 2000);
		} catch (e) {
			console.error('Failed to save LLM config:', e);
		}
		saving = false;
	}

	async function startOAuth() {
		oauthConnecting = true;
		ghError = '';
		try {
			const { url } = await getGitHubOAuthURL();
			window.location.href = url;
		} catch (e: any) {
			ghError = e.message || 'Failed to start OAuth';
			oauthConnecting = false;
		}
	}

	async function saveGitHub() {
		ghSaving = true;
		ghError = '';
		try {
			await connectGitHub({
				repo_owner: ghOwner,
				repo_name: ghRepo,
				access_token: ghToken || undefined,
				default_branch: ghBranch || 'main',
			});
			github = { connected: true, repo_owner: ghOwner, repo_name: ghRepo, default_branch: ghBranch, oauth_enabled: github?.oauth_enabled };
			ghToken = '';
			oauthJustConnected = false;
			ghSaved = true;
			setTimeout(() => { ghSaved = false; }, 3000);
		} catch (e: any) {
			ghError = e.message || 'Failed to connect';
		}
		ghSaving = false;
	}

	const models: Record<string, string[]> = {
		openai: ['gpt-4o-mini', 'gpt-4o', 'gpt-4-turbo'],
		anthropic: ['claude-sonnet-4-6', 'claude-haiku-4-5-20251001'],
		ollama: ['llama3', 'mistral', 'codellama'],
	};
</script>

<div class="p-6 max-w-2xl">
	<div class="mb-6">
		<h2 class="text-2xl font-bold tracking-tight">Settings</h2>
		<p class="text-sm text-muted-foreground mt-1">Project configuration</p>
	</div>

	{#if loading}
		<div class="flex items-center justify-center h-32">
			<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
		</div>
	{:else if project}
		<!-- Project info -->
		<div class="border border-border rounded-lg p-5 bg-card mb-6">
			<h3 class="text-sm font-medium mb-4">Project</h3>
			<div class="space-y-3">
				<div>
					<p class="text-xs text-muted-foreground block mb-1">Name</p>
					<p class="text-sm font-medium">{project.name}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground block mb-1">Project ID</p>
					<p class="text-sm font-mono">{project.id}</p>
				</div>
				<div>
					<p class="text-xs text-muted-foreground block mb-1">API Key</p>
					<div class="flex items-center gap-2">
						<code class="text-sm font-mono bg-muted px-2 py-1 rounded flex-1 truncate">{project.api_key}</code>
						<button
							onclick={copyApiKey}
							class="px-3 py-1.5 text-xs border border-border rounded-md hover:bg-accent transition-colors"
						>
							{copied ? 'Copied!' : 'Copy'}
						</button>
					</div>
				</div>
			</div>
		</div>

		<!-- SDK snippet -->
		<div class="border border-border rounded-lg p-5 bg-card mb-6">
			<h3 class="text-sm font-medium mb-4">SDK Installation</h3>
			<p class="text-xs text-muted-foreground mb-2">Add this snippet to your HTML:</p>
			<pre class="bg-muted p-3 rounded text-xs font-mono overflow-x-auto">&lt;script src="{window.location.origin}/sdk.js"
  data-api-key="{project.api_key}"
  data-host="{window.location.origin}"&gt;&lt;/script&gt;</pre>
		</div>

		<!-- GitHub Integration -->
		<div class="border border-border rounded-lg p-5 bg-card mb-6">
			<h3 class="text-sm font-medium mb-1">GitHub Integration</h3>
			<p class="text-xs text-muted-foreground mb-3">Connect your repo so ClickNest can read your source code when naming events. Instead of <code class="font-mono bg-muted px-1 rounded">button.click #checkout</code> you'll see <strong>"User clicked 'Place Order'"</strong>.</p>

			{#if !github?.connected}
			<div class="bg-muted/50 border border-border rounded-md p-3 mb-4 space-y-1.5 text-xs text-muted-foreground">
				<p class="font-medium text-foreground">How to connect:</p>
				<ol class="list-decimal list-inside space-y-1">
					<li>Go to <a href="https://github.com/settings/tokens?type=beta" target="_blank" rel="noopener" class="underline text-foreground hover:opacity-70">github.com/settings/tokens</a> → <strong>Generate new token (fine-grained)</strong></li>
					<li>Under <strong>Repository access</strong>, select your app's repo</li>
					<li>Under <strong>Permissions → Repository → Contents</strong>, set to <strong>Read-only</strong></li>
					<li>Generate the token and paste it below</li>
				</ol>
			</div>
			{/if}

			{#if github?.connected && github.repo_owner && github.repo_name}
				<div class="flex items-center gap-3 bg-muted/60 border border-border rounded-md px-3 py-2.5 mb-4">
					<span class="w-2 h-2 rounded-full bg-green-500 shrink-0"></span>
					<div class="flex-1 min-w-0">
						<p class="text-sm font-medium font-mono">{github.repo_owner}/{github.repo_name}</p>
						<p class="text-xs text-muted-foreground mt-0.5">
							Branch: {github.default_branch}
							{#if github.last_synced_at}
								 · Last synced {new Date(github.last_synced_at).toLocaleString()}
							{/if}
						</p>
					</div>
					<span class="text-xs text-green-600 font-medium shrink-0">Connected</span>
				</div>
			{:else if oauthJustConnected}
				<div class="flex items-center gap-3 bg-muted/60 border border-border rounded-md px-3 py-2.5 mb-4">
					<span class="w-2 h-2 rounded-full bg-green-500 shrink-0"></span>
					<p class="text-sm flex-1">GitHub account connected! Now select the repository to index.</p>
				</div>
			{/if}

			{#if github?.oauth_enabled && !github?.connected && !oauthJustConnected}
				<!-- OAuth mode: show Connect with GitHub button -->
				<div class="space-y-4">
					{#if ghError}
						<p class="text-sm text-red-600">{ghError}</p>
					{/if}
					<button
						onclick={startOAuth}
						disabled={oauthConnecting}
						class="px-4 py-2 text-sm rounded-md bg-[#24292f] text-white hover:bg-[#24292f]/90 transition-colors disabled:opacity-50 flex items-center gap-2"
					>
						<svg class="w-4 h-4" viewBox="0 0 16 16" fill="currentColor"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"></path></svg>
						{#if oauthConnecting}
							Redirecting to GitHub...
						{:else}
							Connect with GitHub
						{/if}
					</button>
				</div>
			{:else}
				<!-- PAT mode or post-OAuth repo selection -->
				<div class="space-y-4">
					<div class="grid grid-cols-2 gap-3">
						<div>
							<label for="gh-owner" class="text-xs text-muted-foreground block mb-1">Owner</label>
							<input
								id="gh-owner"
								type="text"
								bind:value={ghOwner}
								placeholder="your-username"
								class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background"
							/>
						</div>
						<div>
							<label for="gh-repo" class="text-xs text-muted-foreground block mb-1">Repository</label>
							<input
								id="gh-repo"
								type="text"
								bind:value={ghRepo}
								placeholder="your-app"
								class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background"
							/>
						</div>
					</div>

					{#if !github?.oauth_enabled}
						<div>
							<label for="gh-token" class="text-xs text-muted-foreground block mb-1">
								Personal Access Token
								{#if github?.connected}<span class="text-green-600">(saved — enter new to update)</span>{/if}
							</label>
							<input
								id="gh-token"
								type="password"
								bind:value={ghToken}
								placeholder="ghp_..."
								class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background"
							/>
							<p class="text-xs text-muted-foreground mt-1">Fine-grained token with <code class="font-mono bg-muted px-0.5 rounded">Contents: Read-only</code>, or a classic token with <code class="font-mono bg-muted px-0.5 rounded">repo</code> scope.</p>
						</div>
					{/if}

					<div>
						<label for="gh-branch" class="text-xs text-muted-foreground block mb-1">Default Branch</label>
						<input
							id="gh-branch"
							type="text"
							bind:value={ghBranch}
							placeholder="main"
							class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background"
						/>
					</div>

					{#if ghError}
						<p class="text-sm text-red-600">{ghError}</p>
					{/if}

					<button
						onclick={saveGitHub}
						disabled={ghSaving || !ghOwner || !ghRepo || (!ghToken && !github?.connected && !github?.oauth_enabled)}
						class="px-4 py-2 text-sm rounded-md bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
					>
						{#if ghSaving}
							Connecting...
						{:else if ghSaved}
							Connected! Syncing repo...
						{:else if github?.connected && github.repo_owner && github.repo_name}
							Update Connection
						{:else}
							Connect Repository
						{/if}
					</button>
				</div>
			{/if}
		</div>

		<!-- LLM Configuration -->
		<div class="border border-border rounded-lg p-5 bg-card">
			<h3 class="text-sm font-medium mb-1">AI Event Naming</h3>
			<p class="text-xs text-muted-foreground mb-4">Configure the LLM provider for auto-generating human-readable event names.</p>

			<!-- Current config status -->
			{#if llmApiKeySet || (llmProvider === 'ollama' && llmBaseUrl)}
				<div class="flex items-center gap-3 bg-muted/60 border border-border rounded-md px-3 py-2.5 mb-4">
					<span class="w-2 h-2 rounded-full bg-green-500 shrink-0"></span>
					<div class="flex-1 min-w-0">
						<p class="text-sm font-medium capitalize">{llmProvider} · {llmModel}</p>
						{#if llmApiKeyHint}
							<p class="text-xs text-muted-foreground font-mono mt-0.5">{llmApiKeyHint}</p>
						{:else if llmProvider === 'ollama' && llmBaseUrl}
							<p class="text-xs text-muted-foreground font-mono mt-0.5">{llmBaseUrl}</p>
						{/if}
					</div>
					<span class="text-xs text-green-600 font-medium shrink-0">Active</span>
				</div>
			{/if}

			<div class="space-y-4">
				<div>
					<label for="llm-provider" class="text-xs text-muted-foreground block mb-1">Provider</label>
					<select
						id="llm-provider"
						bind:value={llmProvider}
						onchange={() => { llmModel = models[llmProvider]?.[0] ?? ''; }}
						class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background"
					>
						<option value="openai">OpenAI</option>
						<option value="anthropic">Anthropic</option>
						<option value="ollama">Ollama (self-hosted)</option>
					</select>
				</div>

				{#if llmProvider !== 'ollama'}
					<div>
						<label for="llm-key" class="text-xs text-muted-foreground block mb-1">
							API Key
							{#if llmApiKeySet && !llmApiKey}
								<span class="text-green-600 font-normal ml-1">(saved — leave blank to keep)</span>
							{/if}
						</label>
						<input
							id="llm-key"
							type="password"
							bind:value={llmApiKey}
							placeholder={llmApiKeySet ? '••••••••' : (llmProvider === 'openai' ? 'sk-...' : 'sk-ant-...')}
							class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background"
						/>
					</div>
				{/if}

				<div>
					<label for="llm-model" class="text-xs text-muted-foreground block mb-1">Model</label>
					<select
						id="llm-model"
						bind:value={llmModel}
						class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background"
					>
						{#each models[llmProvider] ?? [] as model}
							<option value={model}>{model}</option>
						{/each}
					</select>
				</div>

				{#if llmProvider === 'ollama'}
					<div>
						<label for="llm-url" class="text-xs text-muted-foreground block mb-1">Base URL</label>
						<input
							id="llm-url"
							type="text"
							bind:value={llmBaseUrl}
							placeholder="http://localhost:11434"
							class="w-full px-3 py-2 text-sm border border-border rounded-md bg-background"
						/>
					</div>
				{/if}

				<button
					onclick={saveLLMConfig}
					disabled={saving}
					class="px-4 py-2 text-sm rounded-md bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
				>
					{#if saving}
						Saving...
					{:else if saved}
						Saved!
					{:else}
						Save Configuration
					{/if}
				</button>
			</div>
		</div>
	{/if}

	<!-- Storage -->
	{#if storage}
	{@const usedPct = storage.volume_bytes > 0 ? (storage.total_bytes / storage.volume_bytes) * 100 : 0}
	{@const barColor = usedPct > 85 ? 'bg-red-500' : usedPct > 70 ? 'bg-yellow-500' : 'bg-primary'}
	<div class="border border-border rounded-lg p-5 bg-card mb-6">
		<h3 class="text-sm font-medium mb-3">Storage</h3>
		{#if storage.volume_bytes > 0}
			<div class="mb-3">
				<div class="flex justify-between text-xs text-muted-foreground mb-1.5">
					<span>{fmtBytes(storage.total_bytes)} used</span>
					<span>{fmtBytes(storage.free_bytes)} free of {fmtBytes(storage.volume_bytes)}</span>
				</div>
				<div class="h-2 rounded-full bg-muted overflow-hidden">
					<div class="h-full rounded-full transition-all {barColor}" style="width: {Math.min(usedPct, 100).toFixed(1)}%"></div>
				</div>
				{#if usedPct > 85}
					<p class="text-xs text-red-600 mt-1.5">Volume is nearly full. Export a backup and consider upgrading.</p>
				{:else if usedPct > 70}
					<p class="text-xs text-yellow-600 mt-1.5">Volume is {usedPct.toFixed(0)}% full.</p>
				{/if}
			</div>
		{/if}
		<div class="grid grid-cols-2 gap-3 text-xs">
			<div class="bg-muted/50 rounded-md px-3 py-2">
				<p class="text-muted-foreground mb-0.5">Events database</p>
				<p class="font-mono font-medium">{fmtBytes(storage.events_bytes)}</p>
			</div>
			<div class="bg-muted/50 rounded-md px-3 py-2">
				<p class="text-muted-foreground mb-0.5">Metadata database</p>
				<p class="font-mono font-medium">{fmtBytes(storage.meta_bytes)}</p>
			</div>
		</div>
	</div>
	{/if}

	<!-- Backup & Restore -->
	<div class="border border-border rounded-lg p-6 space-y-4">
		<div>
			<h2 class="text-base font-semibold">Backup & Restore</h2>
			<p class="text-sm text-muted-foreground mt-1">
				Export your data before migrating to a new server, upgrading your volume, or as a routine backup.
				The archive includes all events, settings, and the encryption key.
			</p>
		</div>

		<div class="flex flex-col sm:flex-row gap-6">
			<!-- Export -->
			<div class="flex-1 space-y-2">
				<p class="text-sm font-medium">Export</p>
				<p class="text-xs text-muted-foreground">Downloads a <code class="font-mono">.tar.gz</code> of your databases and encryption key.</p>
				<a
					href={exportBackupURL()}
					download
					class="inline-block px-4 py-2 text-sm rounded-md bg-primary text-primary-foreground hover:bg-primary/90 transition-colors"
				>
					Download Backup
				</a>
			</div>

			<!-- Import -->
			<div class="flex-1 space-y-2">
				<p class="text-sm font-medium">Restore</p>
				<p class="text-xs text-muted-foreground">Upload a backup archive. The server will restart automatically after restore.</p>
				<div class="space-y-2">
					<input
						type="file"
						accept=".tar.gz,.tgz"
						onchange={(e) => { importFile = (e.target as HTMLInputElement).files?.[0] ?? null; importMessage = ''; importError = ''; }}
						class="block w-full text-sm text-muted-foreground file:mr-3 file:py-1.5 file:px-3 file:rounded-md file:border-0 file:text-sm file:bg-muted file:text-foreground hover:file:bg-muted/80 cursor-pointer"
					/>
					<button
						onclick={handleImport}
						disabled={!importFile || importing}
						class="px-4 py-2 text-sm rounded-md bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors disabled:opacity-50"
					>
						{importing ? 'Restoring...' : 'Restore Backup'}
					</button>
					{#if importMessage}
						<p class="text-xs text-green-600">{importMessage}</p>
					{/if}
					{#if importError}
						<p class="text-xs text-destructive">{importError}</p>
					{/if}
				</div>
			</div>
		</div>
	</div>
</div>
