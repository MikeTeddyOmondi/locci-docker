<script>
	import * as Table from '$lib/components/ui/table/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import SearchIcon from '@lucide/svelte/icons/search';
	import ContainerIcon from '@lucide/svelte/icons/container';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import ContainerActions from '$lib/components/ContainerActions.svelte';
	import ListSkeleton from '$lib/components/ListSkeleton.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import CopyId from '$lib/components/CopyId.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { get, post, del } from '$lib/api.js';
	import { shortId, containerName } from '$lib/format.js';
	import { attempt, confirm } from '$lib/session.svelte.js';

	let containers = $state([]);
	let loading = $state(true);
	let refreshing = $state(false);
	let filter = $state('');
	let stateFilter = $state('all');
	let busy = $state('');

	const states = [
		['all', 'All states'],
		['running', 'Running'],
		['exited', 'Exited'],
		['paused', 'Paused'],
		['created', 'Created']
	];

	const shown = $derived(
		containers.filter(
			(c) =>
				(stateFilter === 'all' || c.State === stateFilter) &&
				(containerName(c) + c.Image + c.State).toLowerCase().includes(filter.toLowerCase())
		)
	);

	async function load() {
		refreshing = true;
		await attempt(async () => (containers = (await get('/containers')).containers));
		loading = refreshing = false;
	}
	$effect(() => {
		load();
	});

	// Docker lists IPv4 and IPv6 bindings separately; show each mapping once.
	const ports = (c) => {
		const seen = new Set();
		return (c.Ports || []).filter((p) => {
			const k = `${p.PublicPort}:${p.PrivatePort}/${p.Type}`;
			return p.PublicPort && !seen.has(k) && seen.add(k);
		});
	};

	async function act(c, action) {
		const name = containerName(c);
		if (action === 'remove') {
			const ok = await confirm({
				title: `Remove ${name}?`,
				description: 'The container is force-removed, even if it is running. Named volumes are kept.',
				action: 'Remove'
			});
			if (!ok) return;
		}
		busy = c.Id;
		if (action === 'remove') await attempt(() => del(`/containers/${c.Id}?force=true`), `Removed ${name}`);
		else await attempt(() => post(`/containers/${c.Id}/${action}`), `${name}: ${action} done`);
		busy = '';
		load();
	}
</script>

<svelte:head><title>Containers · locci</title></svelte:head>

<PageHeader kicker="Manifest 01 · Containers" title="Containers" description="Everything on the dock, live or idle.">
	{#snippet actions()}
		<Button href="/containers/new"><PlusIcon /> New container</Button>
	{/snippet}
</PageHeader>

<div class="rise mb-4 flex flex-col gap-2 sm:flex-row">
	<div class="relative flex-1">
		<SearchIcon class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
		<Input bind:value={filter} placeholder="Search name, image or state" class="h-10 pl-9" />
	</div>
	<div class="flex gap-2">
		<Select.Root type="single" bind:value={stateFilter}>
			<Select.Trigger class="h-10 flex-1 sm:w-40 sm:flex-none">
				{states.find(([v]) => v === stateFilter)?.[1]}
			</Select.Trigger>
			<Select.Content>
				{#each states as [value, label]}<Select.Item {value} {label} />{/each}
			</Select.Content>
		</Select.Root>
		<Button variant="outline" size="icon" class="size-10" onclick={load} aria-label="Refresh">
			<RefreshCwIcon class={refreshing ? 'animate-spin' : ''} />
		</Button>
	</div>
</div>

{#if loading}
	<ListSkeleton cols={5} />
{:else if !shown.length}
	{#if containers.length}
		<EmptyState icon={SearchIcon} title="No matches" description="Nothing on the dock matches that filter." />
	{:else}
		<EmptyState icon={ContainerIcon} title="The dock is empty" description="Create a container from any image to get started.">
			<Button href="/containers/new" size="sm"><PlusIcon /> New container</Button>
		</EmptyState>
	{/if}
{:else}
	<DataTable
		items={shown}
		key={(c) => c.Id}
		noun="containers"
		resetKey={filter + stateFilter}
		columns={[
			{ label: 'Name' },
			{ label: 'State' },
			{ label: 'Image' },
			{ label: 'Ports', class: 'hidden lg:table-cell' },
			{ label: 'Status', class: 'hidden xl:table-cell' },
			{ label: '' }
		]}
	>
		{#snippet row(c)}
			<Table.Cell class="pl-4">
				<a href="/containers/{c.Id}" class="font-medium hover:text-primary">{containerName(c)}</a>
				<div><CopyId value={c.Id} label={shortId(c.Id)} /></div>
			</Table.Cell>
			<Table.Cell><StatusBadge state={c.State} /></Table.Cell>
			<Table.Cell class="max-w-56"><div class="truncate font-mono text-xs" title={c.Image}>{c.Image}</div></Table.Cell>
			<Table.Cell class="hidden lg:table-cell">
				<div class="flex flex-wrap gap-1">
					{#each ports(c) as p}
						<span class="rounded bg-muted px-1.5 py-0.5 font-mono text-[0.7rem]">{p.PublicPort}:{p.PrivatePort}/{p.Type}</span>
					{:else}<span class="text-xs text-muted-foreground">none</span>{/each}
				</div>
			</Table.Cell>
			<Table.Cell class="hidden text-xs text-muted-foreground xl:table-cell">{c.Status}</Table.Cell>
			<Table.Cell class="pr-4">
				<ContainerActions state={c.State} busy={busy === c.Id} onaction={(a) => act(c, a)} />
			</Table.Cell>
		{/snippet}
		{#snippet card(c)}
			<div class="flex items-start justify-between gap-3">
				<div class="min-w-0">
					<a href="/containers/{c.Id}" class="block truncate font-medium">{containerName(c)}</a>
					<div class="truncate font-mono text-xs text-muted-foreground">{c.Image}</div>
				</div>
				<StatusBadge state={c.State} />
			</div>
			<div class="mt-3 flex items-end justify-between gap-2">
				<div class="min-w-0 space-y-1 text-xs text-muted-foreground">
					<div class="truncate">{c.Status}</div>
					{#if ports(c).length}
						<div class="flex flex-wrap gap-1">
							{#each ports(c) as p}<span class="rounded bg-muted px-1.5 py-0.5 font-mono text-[0.7rem] text-foreground">{p.PublicPort}:{p.PrivatePort}</span>{/each}
						</div>
					{/if}
				</div>
				<ContainerActions state={c.State} busy={busy === c.Id} onaction={(a) => act(c, a)} />
			</div>
		{/snippet}
	</DataTable>
{/if}
