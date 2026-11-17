<script>
	import * as Table from '$lib/components/ui/table/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import TrashIcon from '@lucide/svelte/icons/trash';
	import HardDriveIcon from '@lucide/svelte/icons/hard-drive';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ListSkeleton from '$lib/components/ListSkeleton.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import CopyId from '$lib/components/CopyId.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import FormDialog from '$lib/components/FormDialog.svelte';
	import { get, post, del } from '$lib/api.js';
	import { ago } from '$lib/format.js';
	import { attempt, confirm } from '$lib/session.svelte.js';

	let volumes = $state([]);
	let loading = $state(true);
	let name = $state('');
	let createOpen = $state(false);

	async function load() {
		await attempt(async () => (volumes = (await get('/volumes')).volumes || []));
		loading = false;
	}
	$effect(() => {
		load();
	});

	async function create() {
		const ok = await attempt(() => post('/volumes', { name }), `Created ${name}`);
		if (ok) {
			name = '';
			load();
		}
		return ok;
	}

	async function remove(v) {
		const ok = await confirm({ title: `Remove volume ${v.Name}?`, description: 'Its data will be lost for good.', action: 'Delete data' });
		if (!ok) return;
		await attempt(() => del(`/volumes/${encodeURIComponent(v.Name)}`), `Removed ${v.Name}`);
		load();
	}
</script>

<svelte:head><title>Volumes · locci</title></svelte:head>

<PageHeader kicker="Manifest 04 · Volumes" title="Volumes" description="Cargo that outlives its container.">
	{#snippet actions()}
		<Button onclick={() => (createOpen = true)}><PlusIcon /> New volume</Button>
	{/snippet}
</PageHeader>

<FormDialog
	bind:open={createOpen}
	kicker="Reserve a hold"
	title="Create a volume"
	description="A named volume keeps data across container restarts and removals."
	submitLabel="Create volume"
	busyLabel="Creating"
	icon={PlusIcon}
	onsubmit={create}
>
	<div class="grid gap-2">
		<Label for="vol-name">Name</Label>
		<Input id="vol-name" bind:value={name} placeholder="pgdata" required />
		<p class="text-xs text-muted-foreground">Mount it from a container as <code>{name || 'name'}:/path</code>.</p>
	</div>
</FormDialog>

{#if loading}
	<ListSkeleton cols={4} />
{:else if !volumes.length}
	<EmptyState icon={HardDriveIcon} title="No volumes" description="Create one above, or let a container create it on first run." />
{:else}
	<DataTable
		items={volumes}
		key={(v) => v.Name}
		noun="volumes"
		columns={[{ label: 'Name' }, { label: 'Driver' }, { label: 'Mountpoint' }, { label: 'Created' }, { label: '' }]}
	>
		{#snippet row(v)}
			<Table.Cell class="max-w-64 pl-4"><div class="truncate font-medium" title={v.Name}>{v.Name}</div></Table.Cell>
			<Table.Cell><Badge variant="secondary" class="font-mono">{v.Driver}</Badge></Table.Cell>
			<Table.Cell class="max-w-72"><CopyId value={v.Mountpoint} /></Table.Cell>
			<Table.Cell class="text-xs text-muted-foreground">{v.CreatedAt ? ago(v.CreatedAt) : '-'}</Table.Cell>
			<Table.Cell class="pr-4 text-right">
				<Button variant="ghost" size="icon-sm" class="text-muted-foreground hover:text-destructive" onclick={() => remove(v)} aria-label="Remove {v.Name}"><TrashIcon /></Button>
			</Table.Cell>
		{/snippet}
		{#snippet card(v)}
			<div class="flex items-start gap-3">
				<div class="ribbed grid size-9 shrink-0 place-items-center rounded-md border text-muted-foreground"><HardDriveIcon class="size-4" /></div>
				<div class="min-w-0 flex-1">
					<div class="truncate font-medium">{v.Name}</div>
					<CopyId value={v.Mountpoint} />
					<div class="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
						<Badge variant="secondary" class="font-mono">{v.Driver}</Badge>
						{#if v.CreatedAt}<span>{ago(v.CreatedAt)}</span>{/if}
					</div>
				</div>
				<Button variant="ghost" size="icon-sm" class="text-muted-foreground" onclick={() => remove(v)} aria-label="Remove {v.Name}"><TrashIcon /></Button>
			</div>
		{/snippet}
	</DataTable>
{/if}
