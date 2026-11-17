<script>
	import * as Table from '$lib/components/ui/table/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ScrollTextIcon from '@lucide/svelte/icons/scroll-text';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ListSkeleton from '$lib/components/ListSkeleton.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { get } from '$lib/api.js';
	import { attempt } from '$lib/session.svelte.js';

	let entries = $state([]);
	let loading = $state(true);
	let refreshing = $state(false);
	let outcome = $state('all');

	const outcomes = [
		['all', 'All requests'],
		['ok', 'Succeeded'],
		['fail', 'Failed']
	];

	async function load() {
		refreshing = true;
		await attempt(async () => (entries = (await get('/audit?limit=500')).audit));
		loading = refreshing = false;
	}
	$effect(() => {
		load();
	});

	const shown = $derived(
		entries.filter((a) => outcome === 'all' || (outcome === 'ok' ? a.status < 400 : a.status >= 400))
	);

	const methodTone = {
		POST: 'text-sky-600 dark:text-sky-400',
		PUT: 'text-amber-600 dark:text-amber-400',
		PATCH: 'text-amber-600 dark:text-amber-400',
		DELETE: 'text-destructive'
	};
	const statusTone = (s) =>
		s < 400 ? 'bg-ok/10 text-ok ring-ok/25' : 'bg-destructive/10 text-destructive ring-destructive/25';
</script>

<svelte:head><title>Audit log · locci</title></svelte:head>

<PageHeader kicker="Harbour master · Log" title="Audit log" description="Every change made through the API, newest first.">
	{#snippet actions()}
		<Select.Root type="single" bind:value={outcome}>
			<Select.Trigger class="w-40">{outcomes.find(([v]) => v === outcome)?.[1]}</Select.Trigger>
			<Select.Content>
				{#each outcomes as [value, label]}<Select.Item {value} {label} />{/each}
			</Select.Content>
		</Select.Root>
		<Button variant="outline" size="icon" onclick={load} aria-label="Refresh"><RefreshCwIcon class={refreshing ? 'animate-spin' : ''} /></Button>
	{/snippet}
</PageHeader>

{#if loading}
	<ListSkeleton rows={8} cols={5} />
{:else if !shown.length}
	<EmptyState icon={ScrollTextIcon} title="Nothing logged" description="Changes made through the API will appear here." />
{:else}
	<DataTable
		items={shown}
		key={(a) => a.id}
		noun="entries"
		resetKey={outcome}
		columns={[{ label: 'Time (UTC)' }, { label: 'User' }, { label: 'Request' }, { label: 'Status', class: 'text-right' }]}
		cardsClass="relative ml-2 space-y-4 border-l pl-5"
		cardClass="relative"
	>
		{#snippet row(a)}
			<Table.Cell class="pl-4 font-mono text-xs text-muted-foreground">{a.at}</Table.Cell>
			<Table.Cell class="font-medium">{a.username}</Table.Cell>
			<Table.Cell class="max-w-md">
				<div class="truncate font-mono text-xs"><span class="font-semibold {methodTone[a.method] ?? ''}">{a.method}</span> {a.path}</div>
			</Table.Cell>
			<Table.Cell class="pr-4 text-right">
				<span class="rounded-full px-2 py-0.5 font-mono text-xs ring-1 ring-inset {statusTone(a.status)}">{a.status}</span>
			</Table.Cell>
		{/snippet}
		{#snippet card(a)}
			<span class="absolute top-1.5 -left-[25px] size-2.5 rounded-full ring-4 ring-background {a.status < 400 ? 'bg-ok' : 'bg-destructive'}"></span>
			<div class="flex items-center justify-between gap-2 text-xs">
				<span class="font-medium">{a.username}</span>
				<span class="font-mono text-muted-foreground">{a.at}</span>
			</div>
			<div class="mt-1 font-mono text-xs break-all">
				<span class="font-semibold {methodTone[a.method] ?? ''}">{a.method}</span> {a.path}
				<span class="ml-1 rounded-full px-1.5 py-0.5 ring-1 ring-inset {statusTone(a.status)}">{a.status}</span>
			</div>
		{/snippet}
	</DataTable>
{/if}
