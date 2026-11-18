<script>
	// The one confirm dialog for the whole app, driven by confirm() in session.svelte.js.
	import * as AlertDialog from '$lib/components/ui/alert-dialog/index.js';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import { dialog } from '$lib/session.svelte.js';

	function settle(answer) {
		dialog.resolve?.(answer);
		dialog.resolve = null;
		dialog.open = false;
	}
</script>

<AlertDialog.Root bind:open={() => dialog.open, (v) => !v && settle(false)}>
	<AlertDialog.Content class="overflow-hidden">
		{#if dialog.destructive}
			<div class="hazard absolute inset-x-0 top-0 h-1.5 opacity-80" aria-hidden="true"></div>
		{/if}
		<AlertDialog.Header>
			<AlertDialog.Title class="flex items-center gap-2 font-heading text-lg">
				{#if dialog.destructive}<TriangleAlertIcon class="size-5 text-destructive" />{/if}
				{dialog.title}
			</AlertDialog.Title>
			{#if dialog.description}
				<AlertDialog.Description>{dialog.description}</AlertDialog.Description>
			{/if}
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel onclick={() => settle(false)}>Cancel</AlertDialog.Cancel>
			<AlertDialog.Action
				class={dialog.destructive
					? 'bg-destructive text-white hover:bg-destructive/90'
					: ''}
				onclick={() => settle(true)}
			>
				{dialog.action}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
