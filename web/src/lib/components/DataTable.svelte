<script>
	// A paginated listing: a table on wide screens, cards on phones.
	// Pages pass already-filtered `items`; this slices out the current page.
	import * as Table from '$lib/components/ui/table/index.js';
	import * as Pagination from '$lib/components/ui/pagination/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import { cn } from '$lib/utils.js';

	let {
		items = [],
		/** [{ label, class? }]; an empty label renders a screen-reader-only "Actions" head */
		columns = [],
		key = (item) => item.id,
		/** snippet(item): the row's <Table.Cell>s */
		row,
		/** snippet(item): the phone card's contents; without it phones get the table */
		card,
		noun = 'items',
		pageSize = 10,
		pageSizes = [10, 20, 50, 100],
		cardsClass = 'space-y-2.5',
		cardClass = 'rounded-xl border bg-card p-4',
		/** reset to page 1 whenever this value changes, e.g. a search filter */
		resetKey = undefined
	} = $props();

	const STORE = 'locci:page-size';
	const saved = (() => {
		try {
			return Number(localStorage.getItem(STORE)) || 0;
		} catch {
			return 0;
		}
	})();

	let page = $state(1);
	let perPage = $state(pageSizes.includes(saved) ? saved : pageSize);
	let top = $state();

	const pages = $derived(Math.max(1, Math.ceil(items.length / perPage)));
	const start = $derived((page - 1) * perPage);
	const visible = $derived(items.slice(start, start + perPage));

	// Keep the page in range when the list shrinks (filtering, deletes).
	$effect(() => {
		if (page > pages) page = pages;
	});

	// New filter: back to the first page.
	$effect(() => {
		resetKey;
		page = 1;
	});

	function setPerPage(v) {
		perPage = Number(v);
		page = 1;
		try {
			localStorage.setItem(STORE, String(perPage));
		} catch {}
	}

	function goTo(p) {
		page = p;
		// bring the top of the list back into view when paging from the bottom
		if (top && top.getBoundingClientRect().top < 0)
			top.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}
</script>

<div bind:this={top} class="scroll-mt-20">
	<div class={cn('rise overflow-hidden rounded-xl border bg-card', card && 'hidden md:block')}>
		<Table.Root>
			<Table.Header class="bg-muted/40">
				<Table.Row>
					{#each columns as col, i}
						<Table.Head
							class={cn('kicker', i === 0 && 'pl-4', i === columns.length - 1 && 'pr-4', col.class)}
						>
							{#if col.label}{col.label}{:else}<span class="sr-only">Actions</span>{/if}
						</Table.Head>
					{/each}
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each visible as item (key(item))}
					<Table.Row>{@render row(item)}</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	</div>

	{#if card}
		<ul class={cn(cardsClass, 'md:hidden')}>
			{#each visible as item, i (key(item))}
				<li class={cn('rise', cardClass)} style="animation-delay: {i * 30}ms">{@render card(item)}</li>
			{/each}
		</ul>
	{/if}

	<div class="mt-4 flex flex-col-reverse items-center gap-3 sm:flex-row sm:justify-between">
		<div class="flex flex-wrap items-center justify-center gap-x-4 gap-y-2 text-xs text-muted-foreground">
			<span class="font-mono tabular-nums">
				{#if items.length}
					{start + 1}-{Math.min(start + perPage, items.length)} of {items.length} {noun}
				{:else}
					0 {noun}
				{/if}
			</span>
			<div class="flex items-center gap-2">
				<span>Rows</span>
				<Select.Root type="single" value={String(perPage)} onValueChange={setPerPage}>
					<Select.Trigger size="sm" class="h-8 w-[4.5rem] font-mono">{perPage}</Select.Trigger>
					<Select.Content>
						{#each pageSizes as n}
							<Select.Item value={String(n)} label={String(n)} class="font-mono" />
						{/each}
					</Select.Content>
				</Select.Root>
			</div>
		</div>

		{#if pages > 1}
			<Pagination.Root
				count={items.length}
				{perPage}
				bind:page={() => page, goTo}
				siblingCount={1}
				class="mx-0 w-auto"
			>
				{#snippet children({ pages: links, currentPage })}
					<Pagination.Content>
						<Pagination.Item><Pagination.Previous /></Pagination.Item>
						{#each links as link (link.key)}
							{#if link.type === 'ellipsis'}
								<Pagination.Item class="hidden sm:block"><Pagination.Ellipsis /></Pagination.Item>
							{:else}
								<Pagination.Item
									class={currentPage === link.value ? '' : 'hidden sm:block'}
								>
									<Pagination.Link page={link} isActive={currentPage === link.value} size="icon-sm" class="font-mono">
										{link.value}
									</Pagination.Link>
								</Pagination.Item>
							{/if}
						{/each}
						<Pagination.Item class="px-1 font-mono text-xs text-muted-foreground sm:hidden">
							of {pages}
						</Pagination.Item>
						<Pagination.Item><Pagination.Next /></Pagination.Item>
					</Pagination.Content>
				{/snippet}
			</Pagination.Root>
		{/if}
	</div>
</div>
