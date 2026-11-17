<script>
	// Placeholder rows while a list loads; mirrors the table on wide screens.
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';

	let { rows = 5, cols = 4 } = $props();
</script>

<div class="overflow-hidden rounded-xl border bg-card" aria-busy="true" aria-label="Loading">
	<div class="hidden gap-4 border-b bg-muted/40 px-4 py-3 md:flex">
		{#each Array(cols) as _, i}<Skeleton class="h-3 {i === 0 ? 'w-32' : 'w-16'}" />{/each}
	</div>
	{#each Array(rows) as _, r}
		<div class="flex items-center gap-4 border-b px-4 py-3.5 last:border-0" style="opacity: {1 - r * 0.13}">
			<Skeleton class="size-8 shrink-0 rounded-md" />
			<div class="flex flex-1 flex-col gap-2">
				<Skeleton class="h-3.5 w-2/5 max-w-48" />
				<Skeleton class="h-3 w-1/4 max-w-28" />
			</div>
			{#each Array(Math.max(0, cols - 2)) as _}
				<Skeleton class="hidden h-3.5 w-20 md:block" />
			{/each}
			<Skeleton class="h-8 w-20 rounded-md" />
		</div>
	{/each}
</div>
