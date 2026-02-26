<script lang="ts">
	import { onMount } from 'svelte';
	import { getUsers, getUserEvents } from '$lib/api';
	import { eventDisplayName, formatTime, relativeTime } from '$lib/utils';
	import { exportCSV } from '$lib/csv';
	import type { UserProfile, Event } from '$lib/types';

	let users = $state<UserProfile[]>([]);
	let total = $state(0);
	let selectedUser = $state<string | null>(null);
	let userEvents = $state<Event[]>([]);
	let loading = $state(true);
	let loadingDetail = $state(false);
	let range = $state('30d');
	let search = $state('');

	onMount(() => {
		loadUsers();
	});

	async function loadUsers() {
		loading = true;
		try {
			const end = new Date();
			let start: Date;
			switch (range) {
				case '7d': start = new Date(end.getTime() - 7 * 24 * 60 * 60 * 1000); break;
				case '30d': start = new Date(end.getTime() - 30 * 24 * 60 * 60 * 1000); break;
				case '90d': start = new Date(end.getTime() - 90 * 24 * 60 * 60 * 1000); break;
				default: start = new Date(end.getTime() - 30 * 24 * 60 * 60 * 1000);
			}
			const res = await getUsers({
				limit: '100',
				start: start.toISOString(),
				end: end.toISOString(),
			});
			users = res.users ?? [];
			total = res.total ?? 0;
		} catch (e) {
			console.error('Failed to load users:', e);
		}
		loading = false;
	}

	async function selectUser(id: string) {
		selectedUser = id;
		loadingDetail = true;
		try {
			const res = await getUserEvents(id, { limit: '200' });
			userEvents = res.events ?? [];
		} catch (e) {
			console.error('Failed to load user events:', e);
		}
		loadingDetail = false;
	}

	function duration(first: string, last: string): string {
		const diff = new Date(last).getTime() - new Date(first).getTime();
		const seconds = Math.floor(diff / 1000);
		if (seconds < 60) return `${seconds}s`;
		const minutes = Math.floor(seconds / 60);
		if (minutes < 60) return `${minutes}m`;
		const hours = Math.floor(minutes / 60);
		if (hours < 24) return `${hours}h`;
		const days = Math.floor(hours / 24);
		return `${days}d`;
	}

	let filteredUsers = $derived(() => {
		const q = search.trim().toLowerCase();
		if (!q) return users;
		return users.filter(u => u.distinct_id.toLowerCase().includes(q));
	});

	// Compute top pages and event breakdown from user events
	let userTopPages = $derived(() => {
		if (!userEvents.length) return [];
		const counts: Record<string, number> = {};
		for (const e of userEvents) {
			if (e.url_path) counts[e.url_path] = (counts[e.url_path] || 0) + 1;
		}
		return Object.entries(counts)
			.sort((a, b) => b[1] - a[1])
			.slice(0, 5);
	});

	let userEventTypes = $derived(() => {
		if (!userEvents.length) return {};
		const counts: Record<string, number> = {};
		for (const e of userEvents) {
			counts[e.event_type] = (counts[e.event_type] || 0) + 1;
		}
		return counts;
	});

	let userTopEventNames = $derived(() => {
		if (!userEvents.length) return [];
		const counts: Record<string, number> = {};
		for (const e of userEvents) {
			const name = e.event_name || null;
			if (name) counts[name] = (counts[name] || 0) + 1;
		}
		return Object.entries(counts)
			.sort((a, b) => b[1] - a[1])
			.slice(0, 5);
	});
</script>

<div class="p-6 max-w-6xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Users</h2>
			<p class="text-sm text-muted-foreground mt-1">{total} identified users</p>
		</div>
		<div class="flex gap-2 items-center">
			<button
				onclick={() => exportCSV(users as any, 'users.csv')}
				disabled={users.length === 0}
				class="px-2 py-1 text-xs rounded border border-border hover:bg-accent disabled:opacity-40 transition-colors"
			>Export CSV</button>
			<div class="flex gap-1">
				{#each [['7d', '7D'], ['30d', '30D'], ['90d', '90D']] as [value, label]}
					<button
						onclick={() => { range = value; loadUsers(); }}
						class="px-3 py-1.5 text-sm rounded-md border transition-colors {range === value
							? 'bg-primary text-primary-foreground border-primary'
							: 'border-border hover:bg-accent'}"
					>
						{label}
					</button>
				{/each}
			</div>
		</div>
	</div>

	<div class="grid grid-cols-[1fr_1.5fr] gap-4">
		<!-- User list -->
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<div class="px-4 py-3 border-b border-border flex items-center gap-2">
				<h3 class="text-sm font-medium">Users ({filteredUsers().length}{filteredUsers().length !== users.length ? `/${users.length}` : ''})</h3>
				<input
					bind:value={search}
					placeholder="Search users..."
					class="ml-auto px-2 py-1 text-xs border border-border rounded bg-background w-36"
				/>
			</div>
			{#if loading}
				<div class="p-8 text-center text-muted-foreground text-sm">Loading...</div>
			{:else if users.length === 0}
				<div class="p-8 text-center">
					<p class="text-muted-foreground text-sm">No identified users found</p>
					<p class="text-xs text-muted-foreground mt-1">Users appear when your SDK calls <code class="font-mono">identify(userId)</code></p>
				</div>
			{:else if filteredUsers().length === 0}
				<div class="p-6 text-center text-muted-foreground text-sm">No users match "{search}"</div>
			{:else}
				<div class="divide-y divide-border max-h-[600px] overflow-y-auto">
					{#each filteredUsers() as user}
						<button
							onclick={() => selectUser(user.distinct_id)}
							class="w-full px-4 py-3 text-left hover:bg-accent/50 transition-colors text-sm {selectedUser === user.distinct_id ? 'bg-accent' : ''}"
						>
							<div class="flex items-center justify-between">
								<span class="font-medium text-sm truncate max-w-[160px]">{user.distinct_id}</span>
								<span class="text-[10px] text-muted-foreground">{relativeTime(user.last_seen)}</span>
							</div>
							<div class="flex items-center gap-3 mt-1.5">
								<span class="text-xs text-muted-foreground">{user.event_count} events</span>
								<span class="text-xs text-muted-foreground">active {duration(user.first_seen, user.last_seen)}</span>
							</div>
						</button>
					{/each}
				</div>
			{/if}
		</div>

		<!-- User detail panel -->
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<div class="px-4 py-3 border-b border-border">
				<h3 class="text-sm font-medium">
					{#if selectedUser}
						{selectedUser}
					{:else}
						Select a user
					{/if}
				</h3>
			</div>
			{#if !selectedUser}
				<div class="p-8 text-center text-muted-foreground text-sm">Click a user to view their activity</div>
			{:else if loadingDetail}
				<div class="p-8 text-center text-muted-foreground text-sm">Loading...</div>
			{:else}
				<!-- Activity summary -->
				<div class="px-4 py-3 border-b border-border bg-muted/30 grid grid-cols-3 gap-4">
					<!-- Event type breakdown -->
					<div>
						<p class="text-xs text-muted-foreground font-medium mb-1.5">Event Types</p>
						<div class="flex flex-col gap-1">
							{#each Object.entries(userEventTypes()).sort((a, b) => b[1] - a[1]) as [type, count]}
								<div class="flex items-center gap-1.5">
									<span class="w-1.5 h-1.5 rounded-full flex-shrink-0 {type === 'click' ? 'bg-blue-500' :
										type === 'pageview' ? 'bg-green-500' :
										type === 'submit' ? 'bg-purple-500' :
										'bg-gray-400'}"></span>
									<span class="text-xs text-muted-foreground capitalize">{type}</span>
									<span class="text-xs font-medium ml-auto">{count}</span>
								</div>
							{/each}
						</div>
					</div>
					<!-- Top pages -->
					<div>
						<p class="text-xs text-muted-foreground font-medium mb-1.5">Top Pages</p>
						<div class="flex flex-col gap-1">
							{#each userTopPages() as [path, count]}
								<div class="flex items-center gap-1.5">
									<span class="text-xs text-muted-foreground truncate flex-1" title={path}>{path}</span>
									<span class="text-xs font-medium">{count}</span>
								</div>
							{:else}
								<span class="text-xs text-muted-foreground">No pageviews</span>
							{/each}
						</div>
					</div>
					<!-- Top events -->
					<div>
						<p class="text-xs text-muted-foreground font-medium mb-1.5">Top Events</p>
						<div class="flex flex-col gap-1">
							{#each userTopEventNames() as [name, count]}
								<div class="flex items-center gap-1.5">
									<span class="text-xs text-muted-foreground truncate flex-1" title={name}>{name}</span>
									<span class="text-xs font-medium">{count}</span>
								</div>
							{:else}
								<span class="text-xs text-muted-foreground">No named events</span>
							{/each}
						</div>
					</div>
				</div>

				<!-- Event timeline -->
				<div class="divide-y divide-border max-h-[480px] overflow-y-auto">
					{#each userEvents as event, i}
						<div class="px-4 py-3 flex gap-3">
							<div class="flex flex-col items-center">
								<div class="w-2.5 h-2.5 rounded-full mt-1 flex-shrink-0 {event.event_type === 'click' ? 'bg-blue-500' :
									event.event_type === 'pageview' ? 'bg-green-500' :
									event.event_type === 'submit' ? 'bg-purple-500' :
									'bg-gray-400'}"></div>
								{#if i < userEvents.length - 1}
									<div class="w-px flex-1 bg-border mt-1"></div>
								{/if}
							</div>
							<div class="flex-1 min-w-0">
								<div class="flex items-center gap-2">
									<span class="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">{event.event_type}</span>
									<span class="text-xs text-muted-foreground">{formatTime(event.timestamp)}</span>
								</div>
								<p class="text-sm mt-0.5 {event.event_name ? 'font-medium' : 'text-muted-foreground'}">
									{eventDisplayName(event)}
								</p>
								<p class="text-xs text-muted-foreground mt-0.5 truncate">{event.url_path}</p>
								{#if event.properties && Object.keys(event.properties).length > 0}
									<details class="mt-1">
										<summary class="text-xs text-primary cursor-pointer">Properties</summary>
										<pre class="text-xs font-mono text-muted-foreground mt-1 whitespace-pre-wrap">{JSON.stringify(event.properties, null, 2)}</pre>
									</details>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>
</div>
