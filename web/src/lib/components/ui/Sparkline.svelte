<script lang="ts">
	import { onMount } from 'svelte';
	import type { TrendPoint } from '$lib/types';

	let { data, width = 120, height = 32, color = '#6366f1' }: {
		data: TrendPoint[];
		width?: number;
		height?: number;
		color?: string;
	} = $props();

	let canvas: HTMLCanvasElement;

	function draw() {
		if (!canvas || !data || data.length === 0) return;
		const ctx = canvas.getContext('2d');
		if (!ctx) return;

		const dpr = window.devicePixelRatio || 1;
		canvas.width = width * dpr;
		canvas.height = height * dpr;
		ctx.scale(dpr, dpr);
		ctx.clearRect(0, 0, width, height);

		const counts = data.map(d => d.count);
		const max = Math.max(...counts, 1);
		const pad = 2;
		const w = width - pad * 2;
		const h = height - pad * 2;

		// Draw area fill.
		ctx.beginPath();
		ctx.moveTo(pad, height - pad);
		for (let i = 0; i < counts.length; i++) {
			const x = pad + (i / Math.max(counts.length - 1, 1)) * w;
			const y = height - pad - (counts[i] / max) * h;
			ctx.lineTo(x, y);
		}
		ctx.lineTo(pad + w, height - pad);
		ctx.closePath();
		ctx.fillStyle = color + '20';
		ctx.fill();

		// Draw line.
		ctx.beginPath();
		for (let i = 0; i < counts.length; i++) {
			const x = pad + (i / Math.max(counts.length - 1, 1)) * w;
			const y = height - pad - (counts[i] / max) * h;
			if (i === 0) ctx.moveTo(x, y);
			else ctx.lineTo(x, y);
		}
		ctx.strokeStyle = color;
		ctx.lineWidth = 1.5;
		ctx.stroke();
	}

	onMount(() => draw());

	$effect(() => {
		data;
		draw();
	});
</script>

<canvas
	bind:this={canvas}
	style="width: {width}px; height: {height}px;"
	class="block"
></canvas>
