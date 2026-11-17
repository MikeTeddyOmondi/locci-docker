<script>
	import * as Table from '$lib/components/ui/table/index.js';
	import * as Tooltip from '$lib/components/ui/tooltip/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import Loader2Icon from '@lucide/svelte/icons/loader-2';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import TrashIcon from '@lucide/svelte/icons/trash';
	import LayersIcon from '@lucide/svelte/icons/layers';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ListSkeleton from '$lib/components/ListSkeleton.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import CopyId from '$lib/components/CopyId.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { get, post, del } from '$lib/api.js';
	import { bytes, shortId, ago } from '$lib/format.js';
	import { attempt, confirm, notify } from '$lib/session.svelte.js';

	let images = $state([]);
	let loading = $state(true);
	let ref = $state('');
	let pulling = $state(false);

	async function load() {
		await attempt(async () => (images = (await get('/images')).images));
		loading = false;
	}
	$effect(() => {
		load();
	});

	const total = $derived(images.reduce((s, im) => s + (im.Size || 0), 0));
	const largest = $derived(Math.max(1, ...images.map((im) => im.Size || 0)));
	const tags = (im) => (im.RepoTags?.length ? im.RepoTags : ['<none>']);
	const title = (im) => im.RepoTags?.[0] || shortId(im.Id);

	async function pull(e) {
		e.preventDefault();
		pulling = true;
		await attempt(() => post('/images/pull', { image: ref }), `Pulled ${ref}`);
		pulling = false;
		ref = '';
		load();
	}

	async function remove(im) {
		const ok = await confirm({ title: `Remove ${title(im)}?`, description: 'The image is force-removed from this host.', action: 'Remove' });
		if (!ok) return;
		await attempt(() => del(`/images/${encodeURIComponent(im.Id)}?force=true`), `Removed ${title(im)}`);
		load();
	}

	async function prune() {
		const ok = await confirm({
			title: 'Prune dangling images?',
			description: 'Untagged images not used by any container are deleted.',
			action: 'Prune'
		});
		if (!ok) return;
		const r = await attempt(() => post('/images/prune'));
		if (r) notify(`Reclaimed ${bytes(r.SpaceReclaimed || 0)}`);
		load();
	}
</script>

<svelte:head><title>Images · locci</title></svelte:head>

<PageHeader kicker="Manifest 02 · Images" title="Images" description={loading ? 'Reading the hold...' : `${images.length} images, ${bytes(total)} on disk`}>
	{#snippet actions()}
		<Button variant="outline" onclick={prune}><SparklesIcon /> Prune dangling</Button>
	{/snippet}
</PageHeader>

<form onsubmit={pull} class="rise mb-5 flex flex-col gap-2 rounded-xl border bg-card p-3 sm:flex-row">
	<Input bind:value={ref} placeholder="Image to pull, e.g. nginx:alpine" required class="h-10 flex-1 font-mono" />
	<Button type="submit" class="h-10" disabled={pulling}>
		{#if pulling}<Loader2Icon class="animate-spin" /> Pulling{:else}<DownloadIcon /> Pull image{/if}
	</Button>
</form>

{#if loading}
	<ListSkeleton cols={4} />
{:else if !images.length}
	<EmptyState icon={LayersIcon} title="No images yet" description="Pull one above, or create a container and it will be pulled for you." />
{:else}
	<DataTable
		items={images}
		key={(im) => im.Id}
		noun="images"
		columns={[{ label: 'Tags' }, { label: 'ID' }, { label: 'Size' }, { label: 'Created' }, { label: '' }]}
	>
		{#snippet row(im)}
			<Table.Cell class="pl-4">
				{#each tags(im) as t}<div class="font-mono text-xs {t === '<none>' ? 'text-muted-foreground' : ''}">{t}</div>{/each}
			</Table.Cell>
			<Table.Cell><CopyId value={im.Id} label={shortId(im.Id)} /></Table.Cell>
			<Table.Cell>
				<div class="flex items-center gap-2">
					<div class="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
						<div class="h-full bg-primary/70" style="width: {Math.max(4, (im.Size / largest) * 100)}%"></div>
					</div>
					<span class="font-mono text-xs tabular-nums">{bytes(im.Size)}</span>
				</div>
			</Table.Cell>
			<Table.Cell class="text-xs text-muted-foreground">{ago(im.Created)}</Table.Cell>
			<Table.Cell class="pr-4 text-right">
				<Tooltip.Root>
					<Tooltip.Trigger>
						{#snippet child({ props })}
							<Button {...props} variant="ghost" size="icon-sm" class="text-muted-foreground hover:text-destructive" onclick={() => remove(im)} aria-label="Remove image"><TrashIcon /></Button>
						{/snippet}
					</Tooltip.Trigger>
					<Tooltip.Content>Remove image</Tooltip.Content>
				</Tooltip.Root>
			</Table.Cell>
		{/snippet}
		{#snippet card(im)}
			<div class="flex items-start gap-3">
				<div class="ribbed grid size-9 shrink-0 place-items-center rounded-md border text-muted-foreground"><LayersIcon class="size-4" /></div>
				<div class="min-w-0 flex-1">
					{#each tags(im) as t}<div class="truncate font-mono text-xs font-medium">{t}</div>{/each}
					<div class="mt-1 flex flex-wrap items-center gap-x-3 text-xs text-muted-foreground">
						<CopyId value={im.Id} label={shortId(im.Id)} />
						<span>{bytes(im.Size)}</span><span>{ago(im.Created)}</span>
					</div>
				</div>
				<Button variant="ghost" size="icon-sm" class="text-muted-foreground" onclick={() => remove(im)} aria-label="Remove image"><TrashIcon /></Button>
			</div>
		{/snippet}
	</DataTable>
{/if}
