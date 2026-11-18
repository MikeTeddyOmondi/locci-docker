<script>
	// Shown when a list is empty: an open, empty container bay.
	let { title, description = '', icon: Icon, children } = $props();
</script>

<div
	class="flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed px-6 py-14 text-center"
>
	<div class="ribbed grid size-14 place-items-center rounded-lg border bg-card text-muted-foreground">
		{#if Icon}<Icon class="size-6" />{/if}
	</div>
	<div>
		<p class="font-heading text-base font-semibold">{title}</p>
		{#if description}<p class="mt-1 max-w-sm text-sm text-muted-foreground">{description}</p>{/if}
	</div>
	{@render children?.()}
</div>
