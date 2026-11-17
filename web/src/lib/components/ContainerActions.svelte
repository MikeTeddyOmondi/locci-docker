<script>
	// Lifecycle actions for one container: a primary button plus a menu.
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import { Button, buttonVariants } from '$lib/components/ui/button/index.js';
	import PlayIcon from '@lucide/svelte/icons/play';
	import SquareIcon from '@lucide/svelte/icons/square';
	import RotateCwIcon from '@lucide/svelte/icons/rotate-cw';
	import PauseIcon from '@lucide/svelte/icons/pause';
	import SkullIcon from '@lucide/svelte/icons/skull';
	import TrashIcon from '@lucide/svelte/icons/trash';
	import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
	import Loader2Icon from '@lucide/svelte/icons/loader-2';

	/** state: docker state string; onaction(action) runs it; busy disables everything. */
	let { state, onaction, busy = false, withKill = false, size = 'sm' } = $props();

	const running = $derived(state === 'running');
	const paused = $derived(state === 'paused');

	const primary = $derived(
		running
			? ['stop', 'Stop', SquareIcon]
			: paused
				? ['unpause', 'Resume', PlayIcon]
				: ['start', 'Start', PlayIcon]
	);
	const PrimaryIcon = $derived(primary[2]);
</script>

<div class="flex items-center justify-end gap-1.5">
	<Button variant="outline" {size} disabled={busy} onclick={() => onaction(primary[0])}>
		{#if busy}<Loader2Icon class="animate-spin" />{:else}<PrimaryIcon />{/if}
		{primary[1]}
	</Button>
	<DropdownMenu.Root>
		<DropdownMenu.Trigger
			class={buttonVariants({ variant: 'ghost', size: size === 'sm' ? 'icon-sm' : 'icon' })}
			disabled={busy}
			aria-label="More actions"
		>
			<EllipsisIcon />
		</DropdownMenu.Trigger>
		<DropdownMenu.Content align="end" class="w-44">
			{#if running}
				<DropdownMenu.Item onclick={() => onaction('stop')}><SquareIcon /> Stop</DropdownMenu.Item>
				<DropdownMenu.Item onclick={() => onaction('restart')}><RotateCwIcon /> Restart</DropdownMenu.Item>
				<DropdownMenu.Item onclick={() => onaction('pause')}><PauseIcon /> Pause</DropdownMenu.Item>
				{#if withKill}
					<DropdownMenu.Item onclick={() => onaction('kill')}><SkullIcon /> Kill</DropdownMenu.Item>
				{/if}
			{:else if paused}
				<DropdownMenu.Item onclick={() => onaction('unpause')}><PlayIcon /> Resume</DropdownMenu.Item>
				<DropdownMenu.Item onclick={() => onaction('stop')}><SquareIcon /> Stop</DropdownMenu.Item>
			{:else}
				<DropdownMenu.Item onclick={() => onaction('start')}><PlayIcon /> Start</DropdownMenu.Item>
			{/if}
			<DropdownMenu.Separator />
			<DropdownMenu.Item variant="destructive" onclick={() => onaction('remove')}>
				<TrashIcon /> Remove
			</DropdownMenu.Item>
		</DropdownMenu.Content>
	</DropdownMenu.Root>
</div>
