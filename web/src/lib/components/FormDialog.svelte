<script>
	// A form in a modal: header, fields (children), cancel + submit footer.
	// onsubmit should return something truthy on success; the dialog then closes.
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import Loader2Icon from '@lucide/svelte/icons/loader-2';

	let {
		open = $bindable(false),
		kicker = '',
		title,
		description = '',
		submitLabel = 'Save',
		busyLabel = 'Saving',
		icon: Icon,
		onsubmit,
		children
	} = $props();

	let busy = $state(false);

	async function submit(e) {
		e.preventDefault();
		busy = true;
		try {
			if (await onsubmit()) open = false;
		} finally {
			busy = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="overflow-hidden sm:max-w-md">
		<div class="ribbed absolute inset-x-0 top-0 h-1.5 text-primary" aria-hidden="true"></div>
		<form onsubmit={submit} class="grid gap-5">
			<Dialog.Header>
				{#if kicker}<div class="kicker">{kicker}</div>{/if}
				<Dialog.Title class="font-heading text-xl">{title}</Dialog.Title>
				{#if description}<Dialog.Description>{description}</Dialog.Description>{/if}
			</Dialog.Header>

			<div class="grid gap-4">{@render children()}</div>

			<Dialog.Footer>
				<Dialog.Close>
					{#snippet child({ props })}
						<Button {...props} variant="outline" disabled={busy}>Cancel</Button>
					{/snippet}
				</Dialog.Close>
				<Button type="submit" disabled={busy}>
					{#if busy}<Loader2Icon class="animate-spin" /> {busyLabel}{:else}{#if Icon}<Icon />{/if} {submitLabel}{/if}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
