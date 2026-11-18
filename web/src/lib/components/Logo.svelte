<script>
	// Three stacked containers, the top one in signal orange.
	let { class: className = '' } = $props();
</script>

<svg viewBox="0 0 32 32" class={className} aria-hidden="true">
	<rect x="3" y="18" width="12" height="9" rx="1.5" class="fill-foreground/85" />
	<rect x="17" y="18" width="12" height="9" rx="1.5" class="fill-foreground/55" />
	<rect x="10" y="6" width="12" height="9" rx="1.5" class="fill-primary" />
	{#each [6, 9, 12] as x}
		<path d="M{x} 20v5" class="stroke-background/60" stroke-width="1" />
		<path d="M{x + 14} 20v5" class="stroke-background/60" stroke-width="1" />
		<path d="M{x + 7} 8v5" class="stroke-primary-foreground/50" stroke-width="1" />
	{/each}
</svg>
