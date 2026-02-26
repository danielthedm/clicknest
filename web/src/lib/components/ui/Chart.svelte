<script lang="ts">
	import { Chart, type ChartConfiguration } from '$lib/chart-config';

	interface Props {
		config: ChartConfiguration;
		class?: string;
	}

	let { config, class: className = '' }: Props = $props();

	let canvas: HTMLCanvasElement;
	let chart: Chart | null = null;

	$effect(() => {
		if (!canvas) return;

		if (chart) {
			// Update data in-place for efficiency
			chart.data = config.data;
			if (config.options) {
				chart.options = config.options as any;
			}
			chart.update('none');
		} else {
			chart = new Chart(canvas, {
				type: config.type,
				data: config.data,
				options: config.options,
			} as ChartConfiguration);
		}

		return () => {
			chart?.destroy();
			chart = null;
		};
	});
</script>

<div class={className}>
	<canvas bind:this={canvas}></canvas>
</div>
