<script>
	// Everything under /e/<env>/ talks to that environment's Docker host.
	import { page } from '$app/state';
	import { Button } from '$lib/components/ui/button/index.js';
	import ServerOffIcon from '@lucide/svelte/icons/server-off';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import { envs, remember } from '$lib/env.svelte.js';

	let { children } = $props();
	const name = $derived(page.params.env);

	// Set before the first child renders (so its first API call is scoped),
	// then again whenever the URL switches environment.
	remember(page.params.env);
	$effect.pre(() => remember(name));

	const known = $derived(!envs.loaded || envs.list.some((e) => e.name === name));
</script>

{#if known}
	{#key name}{@render children()}{/key}
{:else}
	<EmptyState
		icon={ServerOffIcon}
		title="No environment called {name}"
		description="It may have been removed or renamed. Pick another one to continue."
	>
		<Button href="/environments" size="sm">See environments</Button>
	</EmptyState>
{/if}
