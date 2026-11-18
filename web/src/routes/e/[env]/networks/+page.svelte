<script>
	import * as Table from '$lib/components/ui/table/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import TrashIcon from '@lucide/svelte/icons/trash';
	import NetworkIcon from '@lucide/svelte/icons/network';
	import LockIcon from '@lucide/svelte/icons/lock';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ListSkeleton from '$lib/components/ListSkeleton.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import CopyId from '$lib/components/CopyId.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import FormDialog from '$lib/components/FormDialog.svelte';
	import { get, post, del } from '$lib/api.js';
	import { shortId } from '$lib/format.js';
	import { attempt, confirm } from '$lib/session.svelte.js';

	let networks = $state([]);
	let loading = $state(true);
	let name = $state('');
	let driver = $state('bridge');
	const builtin = ['bridge', 'host', 'none'];
	const drivers = [
		['bridge', 'Containers on this host'],
		['overlay', 'Spans hosts in a swarm'],
		['macvlan', 'Own MAC address on the LAN']
	];
	let createOpen = $state(false);

	async function load() {
		await attempt(async () => (networks = (await get('/networks')).networks));
		loading = false;
	}
	$effect(() => {
		load();
	});

	const subnets = (n) => (n.IPAM?.Config || []).map((c) => c.Subnet).filter(Boolean);

	async function create() {
		const ok = await attempt(() => post('/networks', { name, driver }), `Created ${name}`);
		if (ok) {
			name = '';
			driver = 'bridge';
			load();
		}
		return ok;
	}

	async function remove(n) {
		const ok = await confirm({ title: `Remove network ${n.Name}?`, description: 'Containers still attached will make this fail.', action: 'Remove' });
		if (!ok) return;
		await attempt(() => del(`/networks/${n.Id}`), `Removed ${n.Name}`);
		load();
	}
</script>

<svelte:head><title>Networks · locci</title></svelte:head>

<PageHeader kicker="Manifest 03 · Networks" title="Networks" description="Shipping lanes between your containers.">
	{#snippet actions()}
		<Button onclick={() => (createOpen = true)}><PlusIcon /> New network</Button>
	{/snippet}
</PageHeader>

<FormDialog
	bind:open={createOpen}
	kicker="Open a lane"
	title="Create a network"
	description="Containers on the same network can reach each other by name."
	submitLabel="Create network"
	busyLabel="Creating"
	icon={PlusIcon}
	onsubmit={create}
>
	<div class="grid gap-2">
		<Label for="net-name">Name</Label>
		<Input id="net-name" bind:value={name} placeholder="backend" required />
	</div>
	<div class="grid gap-2">
		<Label>Driver</Label>
		<Select.Root type="single" bind:value={driver}>
			<Select.Trigger class="w-full font-mono">{driver}</Select.Trigger>
			<Select.Content>
				{#each drivers as [d, hint]}
					<Select.Item value={d} label={d}>
						<div><div class="font-mono">{d}</div><div class="text-xs text-muted-foreground">{hint}</div></div>
					</Select.Item>
				{/each}
			</Select.Content>
		</Select.Root>
	</div>
</FormDialog>

{#if loading}
	<ListSkeleton cols={5} />
{:else if !networks.length}
	<EmptyState icon={NetworkIcon} title="No networks" description="Docker could not list any networks on this host." />
{:else}
	<DataTable
		items={networks}
		key={(n) => n.Id}
		noun="networks"
		columns={[{ label: 'Name' }, { label: 'ID' }, { label: 'Driver' }, { label: 'Scope' }, { label: 'Subnet' }, { label: '' }]}
	>
		{#snippet row(n)}
			<Table.Cell class="pl-4 font-medium">
				<span class="flex items-center gap-2">{n.Name}
					{#if builtin.includes(n.Name)}<LockIcon class="size-3 text-muted-foreground" />{/if}</span>
			</Table.Cell>
			<Table.Cell><CopyId value={n.Id} label={shortId(n.Id)} /></Table.Cell>
			<Table.Cell><Badge variant="secondary" class="font-mono">{n.Driver}</Badge></Table.Cell>
			<Table.Cell class="text-xs text-muted-foreground">{n.Scope}</Table.Cell>
			<Table.Cell class="font-mono text-xs">{subnets(n).join(', ') || '-'}</Table.Cell>
			<Table.Cell class="pr-4 text-right">
				{#if !builtin.includes(n.Name)}
					<Button variant="ghost" size="icon-sm" class="text-muted-foreground hover:text-destructive" onclick={() => remove(n)} aria-label="Remove {n.Name}"><TrashIcon /></Button>
				{/if}
			</Table.Cell>
		{/snippet}
		{#snippet card(n)}
			<div class="flex items-start gap-3">
				<div class="ribbed grid size-9 shrink-0 place-items-center rounded-md border text-muted-foreground"><NetworkIcon class="size-4" /></div>
				<div class="min-w-0 flex-1">
					<div class="flex items-center gap-2 font-medium">{n.Name}
						{#if builtin.includes(n.Name)}<LockIcon class="size-3 text-muted-foreground" />{/if}</div>
					<div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
						<Badge variant="secondary" class="font-mono">{n.Driver}</Badge>
						<span>{n.Scope}</span>
						{#if subnets(n).length}<span class="font-mono">{subnets(n).join(', ')}</span>{/if}
					</div>
				</div>
				{#if !builtin.includes(n.Name)}
					<Button variant="ghost" size="icon-sm" class="text-muted-foreground" onclick={() => remove(n)} aria-label="Remove {n.Name}"><TrashIcon /></Button>
				{/if}
			</div>
		{/snippet}
	</DataTable>
{/if}
