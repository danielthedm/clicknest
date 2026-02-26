<script lang="ts">
	import { onMount } from 'svelte';
	import { getHeatmap, getPages } from '$lib/api';
	import type { HeatmapPoint, PageStat } from '$lib/types';

	let points = $state<HeatmapPoint[]>([]);
	let pages = $state<PageStat[]>([]);
	let loading = $state(true);
	let range = $state('7d');
	let selectedPath = $state('');
	let canvas = $state<HTMLCanvasElement | null>(null);
	let canvasWidth = $state(800);
	let canvasHeight = $state(450);

	onMount(async () => {
		await loadTopPages();
		if (selectedPath) {
			await loadHeatmap();
		} else {
			loading = false;
		}
	});

	function getDateRange() {
		const end = new Date();
		const days: Record<string, number> = { '7d': 7, '30d': 30, '90d': 90 };
		const d = days[range] ?? 7;
		return { start: new Date(end.getTime() - d * 86400000), end };
	}

	async function loadTopPages() {
		try {
			const { start, end } = getDateRange();
			const res = await getPages({ start: start.toISOString(), end: end.toISOString(), limit: '50' });
			pages = res.pages ?? [];
			if (!selectedPath && pages.length > 0) {
				selectedPath = pages[0].path;
			}
		} catch (e) {
			console.error('Failed to load pages:', e);
		}
	}

	async function loadHeatmap() {
		if (!selectedPath) return;
		loading = true;
		points = [];
		try {
			const { start, end } = getDateRange();
			const res = await getHeatmap({
				url_path: selectedPath,
				start: start.toISOString(),
				end: end.toISOString(),
			});
			points = res.points ?? [];
		} catch (e) {
			console.error('Failed to load heatmap:', e);
		}
		loading = false;
		// Draw on next tick after canvas is in DOM.
		setTimeout(draw, 10);
	}

	function draw() {
		if (!canvas || !points.length) return;
		const ctx = canvas.getContext('2d');
		if (!ctx) return;

		const w = canvas.width;
		const h = canvas.height;
		ctx.clearRect(0, 0, w, h);

		const maxCount = Math.max(...points.map(p => p.count));

		for (const p of points) {
			const x = p.x * w;
			const y = p.y * h;
			const intensity = p.count / maxCount;
			const radius = Math.max(6, 20 * intensity);

			// Radial gradient: hot red → cool blue → transparent.
			const grad = ctx.createRadialGradient(x, y, 0, x, y, radius);
			const alpha = Math.min(0.9, 0.3 + 0.6 * intensity);
			if (intensity > 0.6) {
				grad.addColorStop(0, `rgba(255, 50, 50, ${alpha})`);
				grad.addColorStop(0.5, `rgba(255, 140, 0, ${alpha * 0.7})`);
			} else if (intensity > 0.3) {
				grad.addColorStop(0, `rgba(255, 165, 0, ${alpha})`);
				grad.addColorStop(0.5, `rgba(255, 255, 0, ${alpha * 0.6})`);
			} else {
				grad.addColorStop(0, `rgba(0, 100, 255, ${alpha})`);
				grad.addColorStop(0.5, `rgba(100, 180, 255, ${alpha * 0.5})`);
			}
			grad.addColorStop(1, 'rgba(0,0,0,0)');

			ctx.beginPath();
			ctx.arc(x, y, radius, 0, Math.PI * 2);
			ctx.fillStyle = grad;
			ctx.fill();
		}
	}

	$effect(() => {
		if (canvas && points.length > 0) {
			draw();
		}
	});
</script>

<div class="p-6 max-w-5xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h2 class="text-2xl font-bold tracking-tight">Heatmap</h2>
			<p class="text-sm text-muted-foreground mt-1">Click density — where users interact most</p>
		</div>
		<div class="flex gap-1">
			{#each [['7d', '7D'], ['30d', '30D'], ['90d', '90D']] as [value, label]}
				<button
					onclick={() => { range = value; loadTopPages().then(loadHeatmap); }}
					class="px-2 py-1 text-xs rounded border transition-colors {range === value
						? 'bg-primary text-primary-foreground border-primary'
						: 'border-border hover:bg-accent'}"
				>{label}</button>
			{/each}
		</div>
	</div>

	<div class="flex gap-3 mb-4">
		<select
			bind:value={selectedPath}
			onchange={() => loadHeatmap()}
			class="flex-1 px-3 py-1.5 text-sm border border-border rounded bg-background"
		>
			{#if pages.length === 0}
				<option value="">No pages found</option>
			{:else}
				{#each pages as p}
					<option value={p.path}>{p.path} ({p.views.toLocaleString()} views)</option>
				{/each}
			{/if}
		</select>
		<button
			onclick={() => loadHeatmap()}
			class="px-3 py-1.5 text-sm rounded border border-border hover:bg-accent transition-colors"
		>Reload</button>
	</div>

	{#if loading}
		<div class="flex items-center justify-center h-64">
			<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
		</div>
	{:else if !selectedPath}
		<div class="border border-border rounded-lg p-12 bg-card text-center">
			<p class="text-muted-foreground">Select a page to view its heatmap.</p>
		</div>
	{:else if points.length === 0}
		<div class="border border-border rounded-lg p-12 bg-card text-center">
			<svg class="w-12 h-12 mx-auto text-muted-foreground mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
			</svg>
			<p class="text-muted-foreground font-medium">No heatmap data for <span class="font-mono">{selectedPath}</span></p>
			<p class="text-xs text-muted-foreground mt-2">Make sure the SDK is loaded and users are clicking on this page.</p>
		</div>
	{:else}
		<div class="border border-border rounded-lg bg-card overflow-hidden">
			<div class="px-4 py-2 border-b border-border flex items-center justify-between">
				<span class="text-sm font-medium">{points.length} click clusters on <span class="font-mono">{selectedPath}</span></span>
				<div class="flex items-center gap-2 text-xs text-muted-foreground">
					<span class="inline-block w-3 h-3 rounded-full bg-blue-400"></span>Low
					<span class="inline-block w-3 h-3 rounded-full bg-yellow-400"></span>Med
					<span class="inline-block w-3 h-3 rounded-full bg-red-500"></span>High
				</div>
			</div>
			<div class="relative bg-muted/30 flex items-center justify-center" style="min-height: 460px;">
				<div class="text-xs text-muted-foreground absolute top-2 left-2 bg-background/80 rounded px-2 py-1">
					Normalized coordinates (0,0 = top-left, 1,1 = bottom-right)
				</div>
				<canvas
					bind:this={canvas}
					width={canvasWidth}
					height={canvasHeight}
					class="max-w-full rounded"
					style="background: hsl(var(--muted)/0.3);"
				></canvas>
			</div>
		</div>
	{/if}
</div>
