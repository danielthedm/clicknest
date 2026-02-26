/**
 * Exports an array of objects as a CSV file download.
 */
export function exportCSV(data: Record<string, unknown>[], filename: string): void {
	if (!data.length) return;

	const keys = Object.keys(data[0]);
	const escape = (v: unknown): string => {
		if (v == null) return '';
		const s = String(v);
		if (s.includes(',') || s.includes('"') || s.includes('\n') || s.includes('\r')) {
			return '"' + s.replace(/"/g, '""') + '"';
		}
		return s;
	};

	const rows = [
		keys.join(','),
		...data.map(row => keys.map(k => escape(row[k])).join(',')),
	];

	const blob = new Blob([rows.join('\n')], { type: 'text/csv;charset=utf-8;' });
	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = filename;
	a.style.display = 'none';
	document.body.appendChild(a);
	a.click();
	document.body.removeChild(a);
	URL.revokeObjectURL(url);
}
