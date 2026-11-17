<script>
	import MoreHorizontalIcon from '@lucide/svelte/icons/more-horizontal';
	import { cn } from "$lib/utils.js";
	let {
		ref = $bindable(null),
		class: className,
		...restProps
	} = $props();
</script>

<span
	bind:this={ref}
	aria-hidden="true"
	data-slot="pagination-ellipsis"
	class={cn("size-9 items-center justify-center [&_svg:not([class*='size-'])]:size-4 flex items-center justify-center", className)}
	{...restProps}
>
	<MoreHorizontalIcon  />
	<span class="sr-only">More pages</span>
</span>