import {
	Chart,
	LineController,
	BarController,
	DoughnutController,
	LineElement,
	BarElement,
	ArcElement,
	PointElement,
	Filler,
	Tooltip,
	Legend,
	CategoryScale,
	LinearScale,
	type ChartOptions,
	type ChartConfiguration,
} from 'chart.js';

export type { ChartConfiguration };

Chart.register(
	LineController,
	BarController,
	DoughnutController,
	LineElement,
	BarElement,
	ArcElement,
	PointElement,
	Filler,
	Tooltip,
	Legend,
	CategoryScale,
	LinearScale,
);

export { Chart };

export function getCssColor(varName: string, alpha = 1): string {
	if (typeof document === 'undefined') return `hsl(0 0% 50% / ${alpha})`;
	const root = getComputedStyle(document.documentElement);
	const hsl = root.getPropertyValue(`--${varName}`).trim();
	if (!hsl) return `hsl(0 0% 50% / ${alpha})`;
	return `hsl(${hsl} / ${alpha})`;
}

export const EVENT_TYPE_COLORS: Record<string, string> = {
	click: 'hsl(217 91% 60%)',
	pageview: 'hsl(142 71% 45%)',
	submit: 'hsl(263 70% 50%)',
	input: 'hsl(25 95% 53%)',
	custom: 'hsl(220 9% 46%)',
};

export function baseLineOptions(): ChartOptions<'line'> {
	return {
		responsive: true,
		maintainAspectRatio: false,
		animation: false,
		interaction: { mode: 'index', intersect: false },
		plugins: {
			legend: { display: false },
			tooltip: {
				backgroundColor: 'hsl(224 71% 4% / 0.9)',
				titleFont: { size: 11 },
				bodyFont: { size: 11 },
				padding: 8,
				cornerRadius: 4,
			},
		},
		scales: {
			x: {
				grid: { display: false },
				ticks: {
					color: getCssColor('muted-foreground', 0.7),
					font: { size: 10 },
					maxRotation: 0,
					autoSkip: true,
					maxTicksLimit: 8,
				},
				border: { display: false },
			},
			y: {
				beginAtZero: true,
				grid: {
					color: getCssColor('border', 0.5),
				},
				ticks: {
					color: getCssColor('muted-foreground', 0.7),
					font: { size: 10 },
					precision: 0,
				},
				border: { display: false },
			},
		},
	};
}

export function baseDoughnutOptions(): ChartOptions<'doughnut'> {
	return {
		responsive: true,
		maintainAspectRatio: false,
		animation: false,
		plugins: {
			legend: {
				position: 'bottom',
				labels: {
					color: getCssColor('foreground', 0.8),
					font: { size: 11 },
					padding: 12,
					usePointStyle: true,
					pointStyleWidth: 8,
				},
			},
			tooltip: {
				backgroundColor: 'hsl(224 71% 4% / 0.9)',
				titleFont: { size: 11 },
				bodyFont: { size: 11 },
				padding: 8,
				cornerRadius: 4,
			},
		},
		cutout: '60%',
	};
}

export function baseBarOptions(): ChartOptions<'bar'> {
	return {
		responsive: true,
		maintainAspectRatio: false,
		animation: false,
		interaction: { mode: 'index', intersect: false },
		plugins: {
			legend: { display: false },
			tooltip: {
				backgroundColor: 'hsl(224 71% 4% / 0.9)',
				titleFont: { size: 11 },
				bodyFont: { size: 11 },
				padding: 8,
				cornerRadius: 4,
			},
		},
		scales: {
			x: {
				grid: { display: false },
				ticks: {
					color: getCssColor('muted-foreground', 0.7),
					font: { size: 10 },
				},
				border: { display: false },
			},
			y: {
				beginAtZero: true,
				grid: {
					color: getCssColor('border', 0.5),
				},
				ticks: {
					color: getCssColor('muted-foreground', 0.7),
					font: { size: 10 },
					precision: 0,
				},
				border: { display: false },
			},
		},
	};
}
