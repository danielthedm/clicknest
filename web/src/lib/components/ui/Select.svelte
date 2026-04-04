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
		placeholder = 'Select...',
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

	let open = $state(false);
	let triggerEl: HTMLButtonElement;
	let listEl: HTMLDivElement;
	let highlightIndex = $state(-1);

	const sizes = {
		sm: 'h-8 px-2.5 text-xs gap-1',
		md: 'h-9 px-3 text-sm gap-1.5',
		lg: 'h-10 px-3 text-sm gap-2',
	};

	const selectedLabel = $derived(
		options.find(o => o.value === value)?.label ?? placeholder
	);

	const isPlaceholder = $derived(!options.some(o => o.value === value));

	function toggle() {
		if (disabled) return;
		open = !open;
		if (open) {
			highlightIndex = options.findIndex(o => o.value === value);
			if (highlightIndex < 0) highlightIndex = 0;
		}
	}

	function select(opt: Option) {
		if (opt.disabled) return;
		value = opt.value;
		open = false;
		triggerEl?.focus();
		if (onchange) {
			onchange(new Event('change'));
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (!open) {
			if (e.key === 'Enter' || e.key === ' ' || e.key === 'ArrowDown') {
				e.preventDefault();
				open = true;
				highlightIndex = options.findIndex(o => o.value === value);
				if (highlightIndex < 0) highlightIndex = 0;
			}
			return;
		}

		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				highlightIndex = Math.min(highlightIndex + 1, options.length - 1);
				scrollIntoView();
				break;
			case 'ArrowUp':
				e.preventDefault();
				highlightIndex = Math.max(highlightIndex - 1, 0);
				scrollIntoView();
				break;
			case 'Enter':
			case ' ':
				e.preventDefault();
				if (highlightIndex >= 0 && highlightIndex < options.length) {
					select(options[highlightIndex]);
				}
				break;
			case 'Escape':
				e.preventDefault();
				open = false;
				triggerEl?.focus();
				break;
		}
	}

	function scrollIntoView() {
		requestAnimationFrame(() => {
			listEl?.querySelector(`[data-index="${highlightIndex}"]`)?.scrollIntoView({ block: 'nearest' });
		});
	}

	function handleClickOutside(e: MouseEvent) {
		if (open && triggerEl && !triggerEl.contains(e.target as Node) && listEl && !listEl.contains(e.target as Node)) {
			open = false;
		}
	}
</script>

<svelte:window onclick={handleClickOutside} />

{#if label}
	<label class="text-xs font-medium text-muted-foreground block mb-1">{label}</label>
{/if}
<div class="relative {fullWidth ? 'w-full' : 'inline-block'}">
	<button
		bind:this={triggerEl}
		type="button"
		{disabled}
		onclick={toggle}
		onkeydown={handleKeydown}
		aria-haspopup="listbox"
		aria-expanded={open}
		class="{fullWidth ? 'w-full' : ''} {sizes[size]} inline-flex items-center justify-between
			border border-border rounded-md bg-background
			disabled:opacity-50 disabled:cursor-not-allowed
			hover:border-muted-foreground/30
			focus:outline-none focus:ring-2 focus:ring-ring/20 focus:border-ring
			transition-colors cursor-pointer text-left"
	>
		<span class="truncate {isPlaceholder ? 'text-muted-foreground' : 'text-foreground'}">{selectedLabel}</span>
		<svg
			class="shrink-0 text-muted-foreground transition-transform {open ? 'rotate-180' : ''}"
			width="14" height="14" viewBox="0 0 24 24" fill="none"
			stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
		><path d="m6 9 6 6 6-6"/></svg>
	</button>

	{#if open}
		<div
			bind:this={listEl}
			role="listbox"
			class="absolute z-50 mt-1 w-full min-w-[8rem] max-h-60 overflow-auto
				border border-border rounded-md bg-background shadow-lg
				py-1 text-sm animate-in fade-in-0 zoom-in-95"
		>
			{#each options as opt, i}
				<button
					type="button"
					role="option"
					data-index={i}
					aria-selected={opt.value === value}
					disabled={opt.disabled}
					onclick={() => select(opt)}
					onmouseenter={() => (highlightIndex = i)}
					class="w-full text-left px-3 py-1.5 flex items-center gap-2 cursor-pointer
						disabled:opacity-40 disabled:cursor-not-allowed
						{i === highlightIndex ? 'bg-accent text-accent-foreground' : 'text-foreground'}
						{opt.value === value ? 'font-medium' : ''}"
				>
					<svg
						class="shrink-0 {opt.value === value ? 'opacity-100' : 'opacity-0'}"
						width="14" height="14" viewBox="0 0 24 24" fill="none"
						stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"
					><path d="M20 6 9 17l-5-5"/></svg>
					<span class="truncate">{opt.label}</span>
				</button>
			{/each}
		</div>
	{/if}
</div>
