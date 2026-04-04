<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listMentions,
		updateMention,
		draftMentionReply,
		publishMentionReply,
		listSources,
		listConnectors,
	} from '$lib/api';
	import { relativeTime } from '$lib/utils';
	import type { MentionRecord, SourceInfo, ConnectorInfo } from '$lib/types';
	import Select from '$lib/components/ui/Select.svelte';

	let mentions = $state<MentionRecord[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let sources = $state<SourceInfo[]>([]);
	let publishers = $state<ConnectorInfo[]>([]);

	let filterStatus = $state('');
	let filterSource = $state('');
	let expandedId = $state<string | null>(null);

	let draftingId = $state<string | null>(null);
	let replyText = $state('');
	let selectedPublisher = $state('');
	let publishing = $state(false);

	const statusColors: Record<string, string> = {
		new: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
		reviewed: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400',
		replied: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
		dismissed: 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400',
		lead: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
	};

	onMount(() => {
		loadAll();
	});

	async function loadAll() {
		loading = true;
		try {
			const params: Record<string, string> = {};
			if (filterStatus) params.status = filterStatus;
			if (filterSource) params.source = filterSource;

			const [mentionsRes, sourcesRes, pubRes] = await Promise.all([
				listMentions(params),
				listSources(),
				listConnectors(),
			]);
			mentions = mentionsRes.mentions ?? [];
			total = mentionsRes.total ?? 0;
			sources = sourcesRes.sources ?? [];
			publishers = pubRes.connectors ?? [];
			if (publishers.length > 0 && !selectedPublisher) {
				selectedPublisher = publishers[0].name;
			}
		} catch (e) {
			console.error('Failed to load mentions:', e);
		}
		loading = false;
	}

	function toggleExpand(id: string) {
		if (expandedId === id) {
			expandedId = null;
			replyText = '';
		} else {
			expandedId = id;
			const m = mentions.find((x) => x.id === id);
			replyText = m?.suggested_reply ?? '';
		}
	}

	async function handleDraft(id: string) {
		draftingId = id;
		try {
			const res = await draftMentionReply(id);
			replyText = res.reply;
			const idx = mentions.findIndex((m) => m.id === id);
			if (idx >= 0) {
				mentions[idx] = { ...mentions[idx], suggested_reply: res.reply, status: 'reviewed' };
			}
		} catch (e) {
			console.error('Draft failed:', e);
			alert(`Draft failed: ${e}`);
		}
		draftingId = null;
	}

	async function handlePublish(id: string) {
		if (!replyText.trim()) return;
		publishing = true;
		try {
			await publishMentionReply(id, {
				publisher_name: selectedPublisher,
				reply_text: replyText,
			});
			const idx = mentions.findIndex((m) => m.id === id);
			if (idx >= 0) {
				mentions[idx] = { ...mentions[idx], status: 'replied' };
			}
			expandedId = null;
			replyText = '';
		} catch (e) {
			console.error('Publish failed:', e);
			alert(`Publish failed: ${e}`);
		}
		publishing = false;
	}

	async function handleDismiss(id: string) {
		try {
			await updateMention(id, { status: 'dismissed' });
			const idx = mentions.findIndex((m) => m.id === id);
			if (idx >= 0) mentions[idx] = { ...mentions[idx], status: 'dismissed' };
		} catch (e) {
			console.error('Dismiss failed:', e);
		}
	}

	async function handleMarkLead(id: string) {
		try {
			await updateMention(id, { status: 'lead' });
			const idx = mentions.findIndex((m) => m.id === id);
			if (idx >= 0) mentions[idx] = { ...mentions[idx], status: 'lead' };
		} catch (e) {
			console.error('Mark lead failed:', e);
		}
	}
</script>

<div class="p-6 space-y-6">
	<div>
		<h2 class="text-lg font-semibold">Mentions Inbox</h2>
		<p class="text-sm text-muted-foreground">
			Conversations discovered by sources. Draft AI replies and publish through your connected publishers.
		</p>
	</div>

	<!-- Filters -->
	<div class="flex gap-3 items-center">
		<Select
			bind:value={filterStatus}
			onchange={() => loadAll()}
			options={[
				{ value: '', label: 'All statuses' },
				{ value: 'new', label: 'New' },
				{ value: 'reviewed', label: 'Reviewed' },
				{ value: 'replied', label: 'Replied' },
				{ value: 'dismissed', label: 'Dismissed' },
				{ value: 'lead', label: 'Lead' },
			]}
			size="sm"
			fullWidth={false}
		/>

		<Select
			bind:value={filterSource}
			onchange={() => loadAll()}
			options={[
				{ value: '', label: 'All sources' },
				...sources.map(src => ({ value: src.name, label: src.display_name })),
			]}
			size="sm"
			fullWidth={false}
		/>

		<span class="text-sm text-muted-foreground ml-auto">{total} mentions</span>
	</div>

	<!-- List -->
	{#if loading}
		<p class="text-sm text-muted-foreground">Loading...</p>
	{:else if mentions.length === 0}
		<div class="border border-border rounded-lg p-8 text-center">
			<p class="text-muted-foreground">No mentions yet.</p>
			<p class="text-sm text-muted-foreground mt-2">
				{#if sources.length === 0}
					Register a <code class="text-xs bg-muted px-1 py-0.5 rounded">Source</code> implementation to discover conversations where your product is relevant.
				{:else}
					Configure keywords in the <a href="/growth/connectors" class="underline">Connectors</a> page to start monitoring.
				{/if}
			</p>
		</div>
	{:else}
		<div class="space-y-2">
			{#each mentions as mention}
				<div class="border border-border rounded-lg bg-card">
					<!-- Header row -->
					<button
						onclick={() => toggleExpand(mention.id)}
						class="w-full text-left p-4 hover:bg-muted/50 transition-colors"
					>
						<div class="flex items-start gap-3">
							<div class="flex-1 min-w-0">
								<div class="flex items-center gap-2 mb-1">
									<span class="text-xs font-medium px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
										{mention.source_name}
									</span>
									<span class="text-xs px-1.5 py-0.5 rounded {statusColors[mention.status] ?? statusColors.new}">
										{mention.status}
									</span>
									{#if mention.author}
										<span class="text-xs text-muted-foreground">by {mention.author}</span>
									{/if}
								</div>
								{#if mention.title}
									<p class="text-sm font-medium truncate">{mention.title}</p>
								{/if}
								<p class="text-sm text-muted-foreground truncate">{mention.content.slice(0, 150)}{mention.content.length > 150 ? '...' : ''}</p>
							</div>
							<span class="text-xs text-muted-foreground whitespace-nowrap">
								{mention.posted_at ? relativeTime(mention.posted_at) : relativeTime(mention.created_at)}
							</span>
						</div>
					</button>

					<!-- Expanded detail -->
					{#if expandedId === mention.id}
						<div class="border-t border-border p-4 space-y-4">
							<!-- Full content -->
							<div>
								<p class="text-sm whitespace-pre-wrap">{mention.content}</p>
								{#if mention.external_url}
									<a
										href={mention.external_url}
										target="_blank"
										rel="noopener"
										class="text-xs text-primary hover:underline mt-2 inline-block"
									>View original</a>
								{/if}
							</div>

							<!-- Reply area -->
							<div class="space-y-2">
								<div class="flex items-center gap-2">
									<button
										onclick={() => handleDraft(mention.id)}
										disabled={draftingId === mention.id}
										class="text-sm px-3 py-1.5 bg-primary text-primary-foreground rounded-md hover:opacity-90 disabled:opacity-50"
									>
										{draftingId === mention.id ? 'Drafting...' : 'Draft Reply'}
									</button>

									{#if publishers.length > 0}
										<Select
											bind:value={selectedPublisher}
											options={publishers.map(pub => ({ value: pub.name, label: pub.display_name }))}
											size="sm"
											fullWidth={false}
										/>
									{/if}
								</div>

								{#if replyText}
									<textarea
										bind:value={replyText}
										rows={4}
										class="w-full text-sm border border-border rounded-md p-3 bg-background resize-y"
									></textarea>
									<div class="flex gap-2">
										{#if publishers.length > 0}
											<button
												onclick={() => handlePublish(mention.id)}
												disabled={publishing}
												class="text-sm px-3 py-1.5 bg-primary text-primary-foreground rounded-md hover:opacity-90 disabled:opacity-50"
											>
												{publishing ? 'Publishing...' : 'Publish Reply'}
											</button>
										{/if}
									</div>
								{/if}
							</div>

							<!-- Actions -->
							<div class="flex gap-2 pt-2 border-t border-border">
								<button
									onclick={() => handleMarkLead(mention.id)}
									class="text-xs px-2 py-1 border border-border rounded hover:bg-muted transition-colors"
								>Mark as Lead</button>
								<button
									onclick={() => handleDismiss(mention.id)}
									class="text-xs px-2 py-1 border border-border rounded hover:bg-muted transition-colors"
								>Dismiss</button>
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
