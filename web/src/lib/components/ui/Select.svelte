<script lang="ts">
	interface Option {
		value: string;
		label: string;
		disabled?: boolean;
	}

	let {
		value = $bindable(''),
		options = [] as Option[],
		label = '',
		placeholder = '',
		size = 'md' as 'sm' | 'md' | 'lg',
		fullWidth = true,
		disabled = false,
		onchange,
	}: {
		value?: string;
		options?: Option[];
		label?: string;
		placeholder?: string;
		size?: 'sm' | 'md' | 'lg';
		fullWidth?: boolean;
		disabled?: boolean;
		onchange?: (e: Event) => void;
	} = $props();

	const sizes = {
		sm: 'h-8 px-2.5 text-xs',
		md: 'h-9 px-3 text-sm',
		lg: 'h-10 px-3 text-sm',
	};
</script>

{#if label}
	<label class="text-xs font-medium text-muted-foreground block mb-1">{label}</label>
{/if}
<select
	bind:value
	{disabled}
	{onchange}
	class="{fullWidth ? 'w-full' : ''} {sizes[size]} appearance-none border border-border rounded-md bg-background text-foreground
		disabled:opacity-50 disabled:cursor-not-allowed
		hover:border-muted-foreground/30
		focus:outline-none focus:ring-2 focus:ring-ring/20 focus:border-ring
		transition-colors cursor-pointer
		bg-[length:16px_16px] bg-[position:right_8px_center] bg-no-repeat"
	style="background-image: url(&quot;data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 24 24' fill='none' stroke='%23888' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E&quot;); padding-right: 2rem;"
>
	{#if placeholder}
		<option value="" disabled>{placeholder}</option>
	{/if}
	{#each options as opt}
		<option value={opt.value} disabled={opt.disabled}>{opt.label}</option>
	{/each}
</select>
