<script>
	// A short mono ID that copies the full value on click.
	import CopyIcon from '@lucide/svelte/icons/copy';
	import CheckIcon from '@lucide/svelte/icons/check';
	import { toast } from 'svelte-sonner';
	import { cn } from '$lib/utils.js';

	let { value, label = value, class: className = '' } = $props();
	let copied = $state(false);

	async function copy(e) {
		e.preventDefault();
		e.stopPropagation();
		try {
			await navigator.clipboard.writeText(value);
			copied = true;
			toast('Copied to clipboard', { description: label });
			setTimeout(() => (copied = false), 1400);
		} catch {
			toast.error('Clipboard not available');
		}
	}
</script>

<button
	type="button"
	onclick={copy}
	title="Copy {value}"
	class={cn(
		'group/copy inline-flex max-w-full items-center gap-1 rounded-md px-1 -mx-1 font-mono text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground',
		className
	)}
>
	<span class="truncate">{label}</span>
	{#if copied}
		<CheckIcon class="size-3 shrink-0 text-ok" />
	{:else}
		<CopyIcon class="size-3 shrink-0 opacity-0 transition-opacity group-hover/copy:opacity-100" />
	{/if}
</button>
