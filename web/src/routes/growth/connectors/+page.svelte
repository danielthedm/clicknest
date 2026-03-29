<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import {
		listConnectors,
		listSources,
		listSourceConfigs,
		upsertSourceConfig,
		getSourceCredentials,
		saveSourceCredentials,
		deleteSourceCredentials,
		getSourceOAuthUrl,
	} from '$lib/api';
	import type { ConnectorInfo, SourceInfo, SourceConfig, SourceCredentialStatus } from '$lib/types';

	let publishers = $state<ConnectorInfo[]>([]);
	let sources = $state<SourceInfo[]>([]);
	let configs = $state<SourceConfig[]>([]);
	let credentials = $state<Record<string, SourceCredentialStatus>>({});
	let loading = $state(true);
	let toast = $state<{ type: 'success' | 'error'; message: string } | null>(null);

	// Source config modal state
	let configuring = $state<string | null>(null);
	let cfgKeywords = $state('');
	let cfgSchedule = $state(60);
	let cfgEnabled = $state(true);
	let saving = $state(false);

	// Credential modal state
	let connectingSource = $state<string | null>(null);
	let manualAccessToken = $state('');
	let manualRefreshToken = $state('');
	let connectSaving = $state(false);
	let connectError = $state('');
	let oauthUrl = $state<string | null>(null);
	let checkingOAuth = $state(false);

	onMount(async () => {
		// Handle OAuth callback redirects.
		const connected = $page.url.searchParams.get('connected');
		const error = $page.url.searchParams.get('error');
		if (connected) {
			showToast('success', `Connected to ${connected} successfully.`);
			goto('/growth/connectors', { replaceState: true });
		} else if (error) {
			const messages: Record<string, string> = {
				oauth_denied: 'Authorization was denied.',
				oauth_invalid: 'Invalid OAuth response.',
				oauth_state_invalid: 'OAuth state expired — please try again.',
				oauth_exchange_failed: 'Token exchange failed — check your app settings.',
				save_failed: 'Credentials received but could not be saved.',
				unknown_source: 'Unknown source.',
			};
			showToast('error', messages[error] ?? `OAuth error: ${error}`);
			goto('/growth/connectors', { replaceState: true });
		}

		try {
			const [pubRes, srcRes, cfgRes] = await Promise.all([
				listConnectors(),
				listSources(),
				listSourceConfigs(),
			]);
			publishers = pubRes.connectors ?? [];
			sources = srcRes.sources ?? [];
			configs = cfgRes.configs ?? [];

			// Load credential status for each unique source name.
			const names = new Set([
				...publishers.map((p) => p.name),
				...sources.map((s) => s.name),
			]);
			const credResults = await Promise.all(
				[...names].map(async (name) => {
					const cred = await getSourceCredentials(name).catch(() => ({ connected: false }));
					return [name, cred] as const;
				}),
			);
			credentials = Object.fromEntries(credResults);
		} catch (e) {
			console.error('Failed to load connectors:', e);
		}
		loading = false;
	});

	function showToast(type: 'success' | 'error', message: string) {
		toast = { type, message };
		setTimeout(() => (toast = null), 4000);
	}

	function getConfig(sourceName: string): SourceConfig | undefined {
		return configs.find((c) => c.source_name === sourceName);
	}

	function startConfigure(sourceName: string) {
		const existing = getConfig(sourceName);
		if (existing) {
			const kw = JSON.parse(existing.keywords || '[]') as string[];
			cfgKeywords = kw.join(', ');
			cfgSchedule = existing.schedule_minutes;
			cfgEnabled = existing.enabled;
		} else {
			cfgKeywords = '';
			cfgSchedule = 60;
			cfgEnabled = true;
		}
		configuring = sourceName;
	}

	async function handleSaveConfig() {
		if (!configuring) return;
		saving = true;
		try {
			const keywords = cfgKeywords
				.split(',')
				.map((k) => k.trim())
				.filter(Boolean);
			await upsertSourceConfig({
				source_name: configuring,
				keywords,
				schedule_minutes: cfgSchedule,
				enabled: cfgEnabled,
			});
			const cfgRes = await listSourceConfigs();
			configs = cfgRes.configs ?? [];
			configuring = null;
		} catch (e) {
			console.error('Save failed:', e);
			alert(`Save failed: ${e}`);
		}
		saving = false;
	}

	async function startConnect(name: string) {
		connectingSource = name;
		manualAccessToken = '';
		manualRefreshToken = '';
		connectError = '';
		oauthUrl = null;
		checkingOAuth = true;

		try {
			const res = await getSourceOAuthUrl(name);
			if (res.oauth_available && res.url) {
				oauthUrl = res.url;
			} else {
				oauthUrl = null;
			}
		} catch {
			oauthUrl = null;
		}
		checkingOAuth = false;
	}

	function launchOAuth() {
		if (!oauthUrl) return;
		window.location.href = oauthUrl;
	}

	async function handleSaveCredentials() {
		if (!connectingSource) return;
		if (!manualAccessToken && !manualRefreshToken) {
			connectError = 'Enter at least one token.';
			return;
		}
		connectSaving = true;
		connectError = '';
		try {
			const result = await saveSourceCredentials(connectingSource, {
				access_token: manualAccessToken || undefined,
				refresh_token: manualRefreshToken || undefined,
			});
			credentials = {
				...credentials,
				[connectingSource]: {
					connected: true,
					username: result.username,
					connected_at: new Date().toISOString(),
				},
			};
			showToast('success', `Connected as @${result.username}`);
			connectingSource = null;
		} catch (e: any) {
			connectError = e?.message ?? 'Failed to save credentials.';
		}
		connectSaving = false;
	}

	async function handleDisconnect(name: string) {
		try {
			await deleteSourceCredentials(name);
			credentials = { ...credentials, [name]: { connected: false } };
			showToast('success', `Disconnected from ${name}.`);
		} catch {
			showToast('error', 'Failed to disconnect.');
		}
	}
</script>

<div class="p-6 space-y-8">
	<!-- Toast notification -->
	{#if toast}
		<div class="fixed top-4 right-4 z-50 px-4 py-3 rounded-lg shadow-lg text-sm font-medium
			{toast.type === 'success' ? 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300' : 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300'}">
			{toast.message}
		</div>
	{/if}

	<div>
		<h2 class="text-lg font-semibold">Publishers & Sources</h2>
		<p class="text-sm text-muted-foreground">
			Publishers post content to platforms. Sources discover conversations and leads.
		</p>
	</div>

	<!-- Publishers -->
	<div class="space-y-3">
		<h3 class="text-sm font-medium">Publishers (outbound)</h3>
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading...</p>
		{:else if publishers.length === 0}
			<div class="border border-border rounded-lg p-6 bg-card text-center">
				<p class="text-sm text-muted-foreground">No publishers registered.</p>
				<div class="border border-border rounded-lg p-3 bg-muted/50 text-left max-w-md mx-auto mt-3">
					<p class="text-xs font-medium text-muted-foreground mb-1">Implement the Publisher interface:</p>
					<pre class="text-xs font-mono bg-background rounded p-2 overflow-x-auto">type Publisher interface &#123;
    Name() string
    DisplayName() string
    Post(ctx, content) (*PostResult, error)
    FetchEngagement(ctx, id) (*Metrics, error)
    Validate(ctx) error
&#125;</pre>
					<p class="text-xs text-muted-foreground mt-1">
						Register with <code class="bg-background px-1 rounded">registry.RegisterPublisher(p)</code>
					</p>
				</div>
			</div>
		{:else}
			<div class="grid grid-cols-2 gap-3">
				{#each publishers as pub}
					{@const cred = credentials[pub.name]}
					<div class="border border-border rounded-lg p-4 bg-card">
						<div class="flex items-start justify-between">
							<div>
								<h4 class="font-medium text-sm">{pub.display_name}</h4>
								<p class="text-xs text-muted-foreground font-mono">{pub.name}</p>
								{#if cred?.connected}
									<p class="text-xs text-green-600 dark:text-green-400 mt-1">@{cred.username}</p>
								{/if}
							</div>
							{#if cred?.connected}
								<span class="text-xs px-2 py-1 rounded-full bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400">Connected</span>
							{:else}
								<span class="text-xs px-2 py-1 rounded-full bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400">Not connected</span>
							{/if}
						</div>
						<div class="flex gap-2 mt-3">
							{#if cred?.connected}
								<button
									onclick={() => handleDisconnect(pub.name)}
									class="text-xs text-red-500 hover:underline"
								>Disconnect</button>
							{:else}
								<button
									onclick={() => startConnect(pub.name)}
									class="text-xs px-2 py-1 bg-primary text-primary-foreground rounded hover:opacity-90"
								>Connect</button>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Sources -->
	<div class="space-y-3">
		<h3 class="text-sm font-medium">Sources (inbound)</h3>
		{#if loading}
			<p class="text-sm text-muted-foreground">Loading...</p>
		{:else if sources.length === 0}
			<div class="border border-border rounded-lg p-6 bg-card text-center">
				<p class="text-sm text-muted-foreground">No sources registered.</p>
				<div class="border border-border rounded-lg p-3 bg-muted/50 text-left max-w-md mx-auto mt-3">
					<p class="text-xs font-medium text-muted-foreground mb-1">Implement the Source interface:</p>
					<pre class="text-xs font-mono bg-background rounded p-2 overflow-x-auto">type Source interface &#123;
    Name() string
    DisplayName() string
    Search(ctx, query) ([]Mention, error)
    Validate(ctx) error
&#125;</pre>
					<p class="text-xs text-muted-foreground mt-1">
						Register with <code class="bg-background px-1 rounded">registry.RegisterSource(s)</code>
					</p>
				</div>
			</div>
		{:else}
			<div class="grid grid-cols-2 gap-3">
				{#each sources as src}
					{@const cfg = getConfig(src.name)}
					{@const cred = credentials[src.name]}
					<div class="border border-border rounded-lg p-4 bg-card">
						<div class="flex items-start justify-between mb-2">
							<div>
								<h4 class="font-medium text-sm">{src.display_name}</h4>
								<p class="text-xs text-muted-foreground font-mono">{src.name}</p>
								{#if cred?.connected}
									<p class="text-xs text-green-600 dark:text-green-400 mt-0.5">@{cred.username}</p>
								{/if}
							</div>
							<div class="flex flex-col items-end gap-1">
								<span class="text-xs px-2 py-1 rounded-full {cfg?.enabled ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' : 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'}">
									{cfg ? (cfg.enabled ? 'Active' : 'Paused') : 'Not configured'}
								</span>
								{#if cred?.connected}
									<span class="text-xs px-2 py-1 rounded-full bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">Auth'd</span>
								{/if}
							</div>
						</div>
						{#if cfg}
							{@const kw = JSON.parse(cfg.keywords || '[]')}
							<div class="text-xs text-muted-foreground space-y-0.5 mb-2">
								<p>Keywords: {kw.length > 0 ? kw.join(', ') : 'none'}</p>
								<p>Schedule: every {cfg.schedule_minutes}m</p>
							</div>
						{/if}
						<div class="flex gap-3">
							<button
								onclick={() => startConfigure(src.name)}
								class="text-xs text-primary hover:underline"
							>Configure</button>
							{#if cred?.connected}
								<button
									onclick={() => handleDisconnect(src.name)}
									class="text-xs text-red-500 hover:underline"
								>Disconnect</button>
							{:else}
								<button
									onclick={() => startConnect(src.name)}
									class="text-xs text-primary hover:underline"
								>Connect account</button>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Source config modal -->
	{#if configuring}
		<div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
			<div class="bg-background border border-border rounded-lg p-6 w-full max-w-md space-y-4">
				<h3 class="font-medium">Configure {configuring}</h3>

				<div>
					<label class="text-sm font-medium block mb-1">Keywords (comma-separated)</label>
					<input
						type="text"
						bind:value={cfgKeywords}
						placeholder="analytics, product analytics, self-hosted"
						class="w-full text-sm border border-border rounded-md px-3 py-2 bg-background"
					/>
				</div>

				<div>
					<label class="text-sm font-medium block mb-1">Schedule (minutes)</label>
					<input
						type="number"
						bind:value={cfgSchedule}
						min={10}
						class="w-full text-sm border border-border rounded-md px-3 py-2 bg-background"
					/>
				</div>

				<label class="flex items-center gap-2 text-sm">
					<input type="checkbox" bind:checked={cfgEnabled} />
					Enabled
				</label>

				<div class="flex gap-2 justify-end">
					<button
						onclick={() => (configuring = null)}
						class="text-sm px-3 py-1.5 border border-border rounded-md hover:bg-muted"
					>Cancel</button>
					<button
						onclick={handleSaveConfig}
						disabled={saving}
						class="text-sm px-3 py-1.5 bg-primary text-primary-foreground rounded-md hover:opacity-90 disabled:opacity-50"
					>{saving ? 'Saving...' : 'Save'}</button>
				</div>
			</div>
		</div>
	{/if}

	<!-- Connect credentials modal -->
	{#if connectingSource}
		<div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
			<div class="bg-background border border-border rounded-lg p-6 w-full max-w-md space-y-4">
				<h3 class="font-medium capitalize">Connect {connectingSource}</h3>

				{#if checkingOAuth}
					<p class="text-sm text-muted-foreground">Checking OAuth availability...</p>
				{:else if oauthUrl}
					<div class="space-y-3">
						<p class="text-sm text-muted-foreground">
							Click the button below to authorize ClickNest with your {connectingSource} account.
							You'll be redirected back automatically.
						</p>
						<button
							onclick={launchOAuth}
							class="w-full text-sm px-3 py-2 bg-primary text-primary-foreground rounded-md hover:opacity-90"
						>Authorize with {connectingSource}</button>
						<p class="text-xs text-center text-muted-foreground">— or enter tokens manually —</p>
					</div>
				{/if}

				<div class="space-y-3 {oauthUrl && !checkingOAuth ? 'border-t border-border pt-3' : ''}">
					{#if !oauthUrl && !checkingOAuth}
						<p class="text-sm text-muted-foreground">
							Enter your {connectingSource} API credentials manually.
						</p>
					{/if}
					<div>
						<label class="text-xs font-medium block mb-1 text-muted-foreground">Access Token</label>
						<input
							type="password"
							bind:value={manualAccessToken}
							placeholder="Access token"
							class="w-full text-sm border border-border rounded-md px-3 py-2 bg-background font-mono"
						/>
					</div>
					<div>
						<label class="text-xs font-medium block mb-1 text-muted-foreground">Refresh Token</label>
						<input
							type="password"
							bind:value={manualRefreshToken}
							placeholder="Refresh token (recommended)"
							class="w-full text-sm border border-border rounded-md px-3 py-2 bg-background font-mono"
						/>
					</div>
					{#if connectError}
						<p class="text-xs text-red-500">{connectError}</p>
					{/if}
				</div>

				<div class="flex gap-2 justify-end">
					<button
						onclick={() => (connectingSource = null)}
						class="text-sm px-3 py-1.5 border border-border rounded-md hover:bg-muted"
					>Cancel</button>
					<button
						onclick={handleSaveCredentials}
						disabled={connectSaving || (!manualAccessToken && !manualRefreshToken)}
						class="text-sm px-3 py-1.5 bg-primary text-primary-foreground rounded-md hover:opacity-90 disabled:opacity-50"
					>{connectSaving ? 'Saving...' : 'Save tokens'}</button>
				</div>
			</div>
		</div>
	{/if}
</div>
