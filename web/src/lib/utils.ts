import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

export function formatTime(ts: string): string {
	return new Date(ts).toLocaleString();
}

export function relativeTime(ts: string): string {
	const diff = Date.now() - new Date(ts).getTime();
	const seconds = Math.floor(diff / 1000);
	if (seconds < 60) return `${seconds}s ago`;
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	const days = Math.floor(hours / 24);
	return `${days}d ago`;
}

export function eventDisplayName(event: { event_name?: string; event_type: string; element_tag?: string; element_id?: string; element_text?: string; url_path: string }): string {
	if (event.event_name) return event.event_name;
	if (event.event_type === 'pageview') return `Pageview: ${event.url_path}`;

	const parts = [event.event_type];
	if (event.element_tag) parts.push(`<${event.element_tag}>`);
	if (event.element_id) parts.push(`#${event.element_id}`);
	else if (event.element_text) parts.push(`"${event.element_text.substring(0, 30)}"`);
	return parts.join(' ');
}
