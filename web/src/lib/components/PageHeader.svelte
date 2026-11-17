<script>
	// Page title block: a mono "manifest" kicker, a heading and optional actions.
	let { kicker = '', title, description = '', actions, children } = $props();
</script>

<header class="rise mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
	<div class="min-w-0">
		{#if kicker}
			<div class="kicker mb-1.5 flex items-center gap-2">
				<span class="inline-block h-px w-5 bg-primary"></span>{kicker}
			</div>
		{/if}
		<h1 class="flex flex-wrap items-center gap-3 text-2xl font-semibold sm:text-3xl">
			<span class="truncate">{title}</span>
			{@render children?.()}
		</h1>
		{#if description}<p class="mt-1 text-sm text-muted-foreground">{description}</p>{/if}
	</div>
	{#if actions}
		<div class="flex flex-wrap items-center gap-2">{@render actions()}</div>
	{/if}
</header>
