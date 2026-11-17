<script>
	// Picks the environment (Docker host) the fleet pages show.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import ChevronsUpDownIcon from '@lucide/svelte/icons/chevrons-up-down';
	import CheckIcon from '@lucide/svelte/icons/check';
	import ServerIcon from '@lucide/svelte/icons/server';
	import { envs, preferred, statusTone } from '$lib/env.svelte.js';

	const shown = $derived(envs.list.find((e) => e.name === (envs.current || preferred())));

	// Same section in the other environment; a container id wouldn't exist there.
	function target(name) {
		const m = page.url.pathname.match(/^\/e\/[^/]+\/([^/]+)/);
		return `/e/${encodeURIComponent(name)}${m ? '/' + m[1] : ''}`;
	}
</script>

{#if !envs.loaded}
	<Skeleton class="h-12 w-full rounded-lg" />
{:else}
	<DropdownMenu.Root>
		<DropdownMenu.Trigger
			class="ribbed flex w-full items-center gap-3 rounded-lg border bg-card px-3 py-2 text-left text-muted-foreground/30 transition-colors hover:border-primary/40"
			aria-label="Switch environment"
		>
			<span class="signal size-2 {statusTone(shown?.status)}"></span>
			<span class="min-w-0 flex-1">
				<span class="kicker block text-[0.6rem]">Environment</span>
				<span class="block truncate text-sm font-medium text-foreground">{shown?.label ?? 'None'}</span>
			</span>
			<ChevronsUpDownIcon class="size-4 text-muted-foreground" />
		</DropdownMenu.Trigger>
		<DropdownMenu.Content align="start" class="w-(--bits-dropdown-menu-anchor-width) min-w-56">
			<DropdownMenu.Label class="kicker">Docker hosts</DropdownMenu.Label>
			{#each envs.list as e (e.name)}
				<DropdownMenu.Item onclick={() => goto(target(e.name))} class="gap-3">
					<span class="signal size-2 {statusTone(e.status)}"></span>
					<span class="min-w-0 flex-1">
						<span class="block truncate">{e.label}</span>
						<span class="block font-mono text-[0.7rem] text-muted-foreground">{e.name} · {e.kind}{e.is_default ? ' · default' : ''}</span>
					</span>
					{#if e.name === shown?.name}<CheckIcon class="size-4 text-primary" />{/if}
				</DropdownMenu.Item>
			{/each}
			<DropdownMenu.Separator />
			<DropdownMenu.Item onclick={() => goto('/environments')}>
				<ServerIcon /> Manage environments
			</DropdownMenu.Item>
		</DropdownMenu.Content>
	</DropdownMenu.Root>
{/if}
