<script>
	// Lifecycle actions for one container, all in one options menu: what can
	// run now (Start, or Stop, Restart, Pause...), then Remove. While an action
	// runs the trigger shows a spinner. A container the user may not manage
	// (locked: the server's reason) shows the reason instead of actions.
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import { buttonVariants } from '$lib/components/ui/button/index.js';
	import PlayIcon from '@lucide/svelte/icons/play';
	import SquareIcon from '@lucide/svelte/icons/square';
	import RotateCwIcon from '@lucide/svelte/icons/rotate-cw';
	import PauseIcon from '@lucide/svelte/icons/pause';
	import SkullIcon from '@lucide/svelte/icons/skull';
	import TrashIcon from '@lucide/svelte/icons/trash';
	import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
	import LockIcon from '@lucide/svelte/icons/lock';
	import Loader2Icon from '@lucide/svelte/icons/loader-2';

	/** state: docker state string; onaction(action) runs it; busy disables everything. */
	let { state, onaction, busy = false, locked = '', withKill = false, size = 'sm' } = $props();

	const running = $derived(state === 'running' || state === 'restarting');
	const paused = $derived(state === 'paused');

	// The menu closes on select; run the action after it has, so focus and
	// the confirm dialog (for Remove) don't fight the closing menu.
	const run = (action) => setTimeout(() => onaction(action), 0);
</script>

<div class="flex items-center justify-end">
	<DropdownMenu.Root>
		<DropdownMenu.Trigger
			class={buttonVariants({ variant: size === 'sm' ? 'ghost' : 'outline', size: size === 'sm' ? 'icon-sm' : 'icon' })}
			disabled={busy}
			aria-label={busy ? 'Working' : 'Container actions'}
		>
			{#if busy}<Loader2Icon class="animate-spin" />{:else}<EllipsisIcon />{/if}
		</DropdownMenu.Trigger>
		<DropdownMenu.Content align="end" class="w-52">
			{#if locked}
				<DropdownMenu.Label class="flex items-start gap-2 text-xs font-normal text-muted-foreground">
					<LockIcon class="mt-0.5 size-3.5 shrink-0" />
					<span>Only admins can manage this container: {locked}.</span>
				</DropdownMenu.Label>
			{:else}
				{#if running}
					<DropdownMenu.Item onSelect={() => run('stop')}><SquareIcon /> Stop</DropdownMenu.Item>
					<DropdownMenu.Item onSelect={() => run('restart')}><RotateCwIcon /> Restart</DropdownMenu.Item>
					<DropdownMenu.Item onSelect={() => run('pause')}><PauseIcon /> Pause</DropdownMenu.Item>
					{#if withKill}
						<DropdownMenu.Item onSelect={() => run('kill')}><SkullIcon /> Kill</DropdownMenu.Item>
					{/if}
				{:else if paused}
					<DropdownMenu.Item onSelect={() => run('unpause')}><PlayIcon /> Resume</DropdownMenu.Item>
					<DropdownMenu.Item onSelect={() => run('stop')}><SquareIcon /> Stop</DropdownMenu.Item>
				{:else}
					<DropdownMenu.Item onSelect={() => run('start')}><PlayIcon /> Start</DropdownMenu.Item>
				{/if}
				<DropdownMenu.Separator />
				<DropdownMenu.Item variant="destructive" onSelect={() => run('remove')}>
					<TrashIcon /> Remove
				</DropdownMenu.Item>
			{/if}
		</DropdownMenu.Content>
	</DropdownMenu.Root>
</div>
