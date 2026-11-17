<script>
	import * as Card from '$lib/components/ui/card/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import ContainerIcon from '@lucide/svelte/icons/container';
	import LayersIcon from '@lucide/svelte/icons/layers';
	import NetworkIcon from '@lucide/svelte/icons/network';
	import HardDriveIcon from '@lucide/svelte/icons/hard-drive';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import ServerIcon from '@lucide/svelte/icons/server';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import { get } from '$lib/api.js';
	import { bytes, containerName } from '$lib/format.js';
	import { attempt, session } from '$lib/session.svelte.js';

	let info = $state(null);
	let counts = $state(null);
	let recent = $state([]);

	$effect(() => {
		attempt(async () => {
			const [i, c, im, n, v] = await Promise.all([
				get('/system/info'),
				get('/containers'),
				get('/images'),
				get('/networks'),
				get('/volumes')
			]);
			info = i;
			const running = c.containers.filter((x) => x.State === 'running').length;
			counts = {
				containers: c.containers.length,
				running,
				stopped: c.containers.length - running,
				images: im.images.length,
				networks: n.networks.length,
				volumes: (v.volumes || []).length
			};
			recent = [...c.containers].sort((a, b) => b.Created - a.Created).slice(0, 5);
		});
	});

	const tiles = $derived([
		{ href: '/containers', label: 'Containers', value: counts?.containers, icon: ContainerIcon, tone: 'text-primary' },
		{ href: '/images', label: 'Images', value: counts?.images, icon: LayersIcon, tone: 'text-sky-600 dark:text-sky-400' },
		{ href: '/networks', label: 'Networks', value: counts?.networks, icon: NetworkIcon, tone: 'text-teal-600 dark:text-teal-400' },
		{ href: '/volumes', label: 'Volumes', value: counts?.volumes, icon: HardDriveIcon, tone: 'text-violet-600 dark:text-violet-400' }
	]);

	const share = $derived(counts?.containers ? (counts.running / counts.containers) * 100 : 0);

	const greeting = (() => {
		const h = new Date().getHours();
		return h < 5 ? 'Night watch' : h < 12 ? 'Good morning' : h < 18 ? 'Good afternoon' : 'Good evening';
	})();
</script>

<svelte:head><title>Overview · locci</title></svelte:head>

<PageHeader kicker="Manifest 00 · Overview" title="{greeting}, {session.user?.username}">
	{#snippet actions()}
		<Button href="/containers/new"><PlusIcon /> New container</Button>
	{/snippet}
</PageHeader>

<section class="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
	{#each tiles as t, i}
		<a
			href={t.href}
			class="group rise relative overflow-hidden rounded-xl border bg-card p-4 transition-all hover:-translate-y-0.5 hover:shadow-lg hover:shadow-primary/5 sm:p-5"
			style="animation-delay: {i * 60}ms"
		>
			<div class="ribbed absolute inset-x-0 top-0 h-2 {t.tone} opacity-70"></div>
			<div class="flex items-start justify-between">
				<span class="kicker">{t.label}</span>
				<t.icon class="size-4 {t.tone}" />
			</div>
			{#if counts}
				<div class="mt-3 font-heading text-3xl font-semibold tabular-nums sm:text-4xl">{t.value}</div>
			{:else}
				<Skeleton class="mt-4 h-9 w-14" />
			{/if}
			<ArrowRightIcon
				class="absolute right-4 bottom-4 size-4 -translate-x-1 text-muted-foreground opacity-0 transition-all group-hover:translate-x-0 group-hover:opacity-100"
			/>
		</a>
	{/each}
</section>

<div class="mt-6 grid gap-6 lg:grid-cols-5">
	<Card.Root class="rise lg:col-span-3" style="animation-delay: 240ms">
		<Card.Header>
			<div class="kicker">Fleet status</div>
			<Card.Title class="font-heading text-lg">Recently shipped</Card.Title>
			<Card.Action>
				<Button href="/containers" variant="ghost" size="sm">All <ArrowRightIcon /></Button>
			</Card.Action>
		</Card.Header>
		<Card.Content class="space-y-5">
			<div>
				<div class="mb-2 flex items-center justify-between text-xs">
					{#if counts}
						<span class="font-mono"><span class="text-ok">{counts.running} running</span>
							<span class="text-muted-foreground"> / {counts.stopped} stopped</span></span>
						<span class="font-mono text-muted-foreground">{Math.round(share)}% afloat</span>
					{:else}
						<Skeleton class="h-3 w-32" /><Skeleton class="h-3 w-16" />
					{/if}
				</div>
				<div class="flex h-2.5 overflow-hidden rounded-full bg-muted">
					<div class="h-full rounded-full bg-ok transition-[width] duration-700" style="width: {share}%"></div>
				</div>
			</div>

			{#if !counts}
				<div class="space-y-3">
					{#each Array(4) as _}
						<div class="flex items-center gap-3">
							<Skeleton class="size-8 rounded-md" />
							<div class="flex-1 space-y-1.5"><Skeleton class="h-3.5 w-1/3" /><Skeleton class="h-3 w-1/4" /></div>
							<Skeleton class="h-5 w-16 rounded-full" />
						</div>
					{/each}
				</div>
			{:else if recent.length}
				<ul class="-mx-2 divide-y">
					{#each recent as c (c.Id)}
						<li>
							<a href="/containers/{c.Id}" class="flex items-center gap-3 rounded-lg px-2 py-2.5 transition-colors hover:bg-muted/60">
								<div class="ribbed grid size-8 shrink-0 place-items-center rounded-md border bg-background text-muted-foreground">
									<ContainerIcon class="size-4" />
								</div>
								<div class="min-w-0 flex-1">
									<div class="truncate text-sm font-medium">{containerName(c)}</div>
									<div class="truncate font-mono text-xs text-muted-foreground">{c.Image}</div>
								</div>
								<StatusBadge state={c.State} />
							</a>
						</li>
					{/each}
				</ul>
			{:else}
				<EmptyState icon={ContainerIcon} title="The dock is empty" description="Ship your first container to see it here.">
					<Button href="/containers/new" size="sm"><PlusIcon /> New container</Button>
				</EmptyState>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root class="rise lg:col-span-2" style="animation-delay: 300ms">
		<Card.Header>
			<div class="kicker">Host</div>
			<Card.Title class="flex items-center gap-2 font-heading text-lg">
				<ServerIcon class="size-4 text-muted-foreground" />
				{#if info}<span class="truncate">{info.Name}</span>{:else}<Skeleton class="h-5 w-32" />{/if}
			</Card.Title>
		</Card.Header>
		<Card.Content>
			<dl class="grid grid-cols-1 gap-x-4 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2">
				{#if info}
					{#each [
						['Docker', info.ServerVersion],
						['Arch', info.Architecture],
						['CPUs', info.NCPU],
						['Memory', bytes(info.MemTotal)],
						['Storage', info.Driver],
						['Kernel', info.KernelVersion],
						['OS', info.OperatingSystem],
						['Root', info.DockerRootDir]
					] as [k, v]}
						<div class="min-w-0 border-l-2 border-border pl-3">
							<dt class="kicker text-[0.6rem]">{k}</dt>
							<dd class="truncate font-mono text-[0.8rem]" title={String(v)}>{v}</dd>
						</div>
					{/each}
				{:else}
					{#each Array(8) as _}
						<div class="space-y-1.5 border-l-2 border-border pl-3">
							<Skeleton class="h-2.5 w-12" /><Skeleton class="h-3.5 w-24" />
						</div>
					{/each}
				{/if}
			</dl>
		</Card.Content>
	</Card.Root>
</div>
