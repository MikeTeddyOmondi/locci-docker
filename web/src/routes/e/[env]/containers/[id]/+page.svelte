<script>
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import * as Tabs from '$lib/components/ui/tabs/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Switch } from '$lib/components/ui/switch/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import InfoIcon from '@lucide/svelte/icons/info';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import TerminalIcon from '@lucide/svelte/icons/terminal';
	import BracesIcon from '@lucide/svelte/icons/braces';
	import EraserIcon from '@lucide/svelte/icons/eraser';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CpuIcon from '@lucide/svelte/icons/cpu';
	import MemoryStickIcon from '@lucide/svelte/icons/memory-stick';
	import ArrowDownUpIcon from '@lucide/svelte/icons/arrow-down-up';
	import BoxIcon from '@lucide/svelte/icons/box';
	import CircleSlashIcon from '@lucide/svelte/icons/circle-slash';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import ContainerActions from '$lib/components/ContainerActions.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import CopyId from '$lib/components/CopyId.svelte';
	import Terminal from '$lib/Terminal.svelte';
	import { href } from '$lib/env.svelte.js';
	import { get, post, del, stream } from '$lib/api.js';
	import { bytes, shortId } from '$lib/format.js';
	import { attempt, confirm } from '$lib/session.svelte.js';

	const id = $derived(page.params.id);

	let info = $state(null);
	let tab = $state('overview');
	let stats = $state(null);
	let cpuHistory = $state([]);
	let logLines = $state([]);
	let follow = $state(true);
	let shell = $state('sh');
	let termKey = $state(0);
	let busy = $state(false);
	let logBox = $state();

	const load = () => attempt(async () => (info = await get(`/containers/${id}`)));
	$effect(() => {
		load();
	});

	// Stats: poll while the tab is open and the container runs.
	$effect(() => {
		if (tab !== 'stats' || !info?.State?.Running) return;
		const tick = () =>
			get(`/containers/${id}/stats`)
				.then((s) => {
					stats = s;
					cpuHistory = [...cpuHistory, s.cpu_percent].slice(-40);
				})
				.catch(() => {});
		tick();
		const t = setInterval(tick, 3000);
		return () => clearInterval(t);
	});

	// Logs: stream while the tab is open; restart when "follow" changes.
	$effect(() => {
		if (tab !== 'logs') return;
		const ctrl = new AbortController();
		logLines = [];
		stream(
			`/containers/${id}/logs?tail=500&follow=${follow}`,
			(line) => {
				logLines.push(line);
				if (logLines.length > 3000) logLines.splice(0, logLines.length - 3000);
				queueMicrotask(() => logBox && (logBox.scrollTop = logBox.scrollHeight));
			},
			ctrl.signal
		).catch((e) => e.name !== 'AbortError' && logLines.push('[error] ' + e.message));
		return () => ctrl.abort();
	});

	async function act(action) {
		if (action === 'remove') {
			const ok = await confirm({
				title: `Remove ${name}?`,
				description: 'The container is force-removed, even if it is running. Named volumes are kept.',
				action: 'Remove'
			});
			if (ok && (await attempt(() => del(`/containers/${id}?force=true`), `Removed ${name}`)))
				goto(href('/containers'));
			return;
		}
		busy = true;
		await attempt(() => post(`/containers/${id}/${action}`), `${name}: ${action} done`);
		busy = false;
		load();
	}

	const state = $derived(info?.State?.Status);
	const name = $derived(info?.Name?.replace(/^\//, ''));
	const fmtDate = (s) => (!s || s.startsWith('0001') ? 'never' : new Date(s).toLocaleString());

	// No Console tab where the user may not open one (the server refuses too).
	const tabs = $derived(
		[
			['overview', 'Overview', InfoIcon],
			['logs', 'Logs', FileTextIcon],
			['stats', 'Stats', ActivityIcon],
			['console', 'Console', TerminalIcon],
			['inspect', 'Inspect', BracesIcon]
		].filter(([value]) => value !== 'console' || !info?.LocciLocked)
	);
	const shells = ['sh', 'bash', 'ash'];

	// CPU sparkline path in a 100x30 box.
	const spark = $derived.by(() => {
		if (cpuHistory.length < 2) return '';
		const max = Math.max(5, ...cpuHistory);
		return cpuHistory
			.map((v, i) => `${(i / (cpuHistory.length - 1)) * 100},${30 - (v / max) * 28}`)
			.join(' ');
	});
</script>

<svelte:head><title>{name ?? 'Container'} · locci</title></svelte:head>

<Button href={href('/containers')} variant="ghost" size="sm" class="-ml-2 mb-2 text-muted-foreground">
	<ChevronLeftIcon /> Containers
</Button>

{#if !info}
	<div class="space-y-6" aria-busy="true">
		<div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
			<div class="space-y-2"><Skeleton class="h-3 w-28" /><Skeleton class="h-9 w-56" /></div>
			<Skeleton class="h-8 w-32" />
		</div>
		<Skeleton class="h-9 w-full max-w-md" />
		<div class="grid gap-4 md:grid-cols-2">
			{#each Array(4) as _}<Skeleton class="h-40 rounded-xl" />{/each}
		</div>
	</div>
{:else}
	<PageHeader kicker="Container · {shortId(info.Id)}" title={name}>
		<StatusBadge {state} />
		{#snippet actions()}
			<ContainerActions {state} {busy} locked={info.LocciLocked} withKill size="default" onaction={act} />
		{/snippet}
	</PageHeader>

	<Tabs.Root bind:value={tab} class="gap-5">
		<div class="-mx-4 overflow-x-auto px-4 sm:mx-0 sm:px-0">
			<Tabs.List class="w-max">
				{#each tabs as [value, label, Icon]}
					<Tabs.Trigger {value} class="gap-1.5 px-3"><Icon class="size-4" />{label}</Tabs.Trigger>
				{/each}
			</Tabs.List>
		</div>

		<Tabs.Content value="overview" class="min-w-0">
			<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
				<Card.Root class="rise md:col-span-2">
					<Card.Content>
						<dl class="grid grid-cols-1 gap-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
							<div class="min-w-0"><dt class="kicker">ID</dt><dd><CopyId value={info.Id} label={shortId(info.Id)} /></dd></div>
							<div class="min-w-0"><dt class="kicker">Image</dt><dd class="truncate font-mono text-xs" title={info.Config.Image}>{info.Config.Image}</dd></div>
							<div><dt class="kicker">Created</dt><dd class="text-xs">{fmtDate(info.Created)}</dd></div>
							<div><dt class="kicker">Started</dt><dd class="text-xs">{fmtDate(info.State.StartedAt)}</dd></div>
							<div class="min-w-0 sm:col-span-2 lg:col-span-3"><dt class="kicker">Command</dt>
								<dd class="truncate font-mono text-xs">{[info.Path, ...(info.Args || [])].join(' ')}</dd></div>
							<div><dt class="kicker">Restart policy</dt><dd class="font-mono text-xs">{info.HostConfig.RestartPolicy?.Name || 'no'}</dd></div>
						</dl>
					</Card.Content>
				</Card.Root>

				{#snippet section(title, items, render, empty)}
					<Card.Root class="rise min-w-0">
						<Card.Header><Card.Title class="kicker">{title}</Card.Title></Card.Header>
						<Card.Content>
							{#if items.length}
								<ul class="space-y-1.5">
									{#each items as item}<li class="min-w-0">{@render render(item)}</li>{/each}
								</ul>
							{:else}
								<p class="text-sm text-muted-foreground">{empty}</p>
							{/if}
						</Card.Content>
					</Card.Root>
				{/snippet}

				{#snippet port([port, binds])}
					<div class="flex flex-wrap items-center gap-2 text-sm">
						<code class="rounded bg-muted px-1.5 py-0.5 text-xs">{port}</code>
						<span class="text-muted-foreground">to</span>
						<span class="font-mono text-xs">{(binds || []).map((b) => `${b.HostIp || '0.0.0.0'}:${b.HostPort}`).join(', ') || 'not published'}</span>
					</div>
				{/snippet}
				{#snippet net([n, v])}
					<div class="flex items-center justify-between gap-2 text-sm"><span>{n}</span><span class="font-mono text-xs text-muted-foreground">{v.IPAddress || 'no ip'}</span></div>
				{/snippet}
				{#snippet mount(m)}
					<div class="flex flex-wrap items-center gap-1.5 text-xs">
						<code class="truncate">{m.Name || m.Source}</code><span class="text-muted-foreground">to</span><code class="truncate">{m.Destination}</code>
						<span class="kicker ml-auto">{m.RW ? 'rw' : 'ro'}</span>
					</div>
				{/snippet}
				{#snippet code(s)}<code class="block truncate text-xs" title={s}>{s}</code>{/snippet}

				{@render section('Ports', Object.entries(info.NetworkSettings.Ports || {}), port, 'No exposed ports')}
				{@render section('Networks', Object.entries(info.NetworkSettings.Networks || {}), net, 'Not attached')}
				{@render section('Mounts', info.Mounts || [], mount, 'No mounts')}
				{@render section('Environment', info.Config.Env || [], code, 'No variables')}
				<div class="min-w-0 md:col-span-2">
					{@render section('Labels', Object.entries(info.Config.Labels || {}).map(([k, v]) => `${k}=${v}`), code, 'No labels')}
				</div>
			</div>
		</Tabs.Content>

		<Tabs.Content value="logs">
			<div class="overflow-hidden rounded-xl border bg-[oklch(0.16_0.02_255)] text-[oklch(0.88_0.01_85)]">
				<div class="flex flex-wrap items-center justify-between gap-2 border-b border-white/10 px-3 py-2">
					<div class="flex items-center gap-2 font-mono text-xs text-white/50">
						<span class="signal size-1.5 {follow ? 'signal-live text-[oklch(0.76_0.17_152)]' : 'text-white/30'}"></span>
						{logLines.length} lines
					</div>
					<div class="flex items-center gap-3">
						<div class="flex items-center gap-2">
							<Switch id="follow" bind:checked={follow} />
							<Label for="follow" class="text-xs text-white/70">Follow</Label>
						</div>
						<Button variant="ghost" size="sm" class="text-white/70 hover:bg-white/10 hover:text-white" onclick={() => (logLines = [])}>
							<EraserIcon /> Clear
						</Button>
					</div>
				</div>
				<pre
					bind:this={logBox}
					class="h-[60dvh] overflow-auto p-4 text-xs leading-relaxed break-all whitespace-pre-wrap md:h-[480px]">{logLines.join('\n') || 'Waiting for output...'}</pre>
			</div>
		</Tabs.Content>

		<Tabs.Content value="stats">
			{#if !info.State.Running}
				<EmptyState icon={CircleSlashIcon} title="Not running" description="Start the container to see live resource usage." />
			{:else}
				<div class="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
					{#if stats}
						<Card.Root class="col-span-2 lg:col-span-2">
							<Card.Content class="flex h-full flex-col">
								<div class="flex items-center justify-between"><span class="kicker">CPU</span><CpuIcon class="size-4 text-primary" /></div>
								<div class="mt-2 font-heading text-3xl font-semibold tabular-nums">{stats.cpu_percent.toFixed(1)}<span class="text-lg text-muted-foreground">%</span></div>
								<svg viewBox="0 0 100 30" preserveAspectRatio="none" class="mt-3 h-16 w-full text-primary">
									{#if spark}
										<polygon points="0,30 {spark} 100,30" class="fill-primary/10" />
										<polyline points={spark} fill="none" stroke="currentColor" stroke-width="1.2" vector-effect="non-scaling-stroke" />
									{/if}
								</svg>
							</Card.Content>
						</Card.Root>
						<Card.Root>
							<Card.Content>
								<div class="flex items-center justify-between"><span class="kicker">Memory</span><MemoryStickIcon class="size-4 text-sky-500" /></div>
								<div class="mt-2 font-heading text-2xl font-semibold tabular-nums">{bytes(stats.mem_usage)}</div>
								<div class="mt-3 h-1.5 overflow-hidden rounded-full bg-muted">
									<div class="h-full rounded-full bg-sky-500 transition-[width] duration-500" style="width: {Math.min(100, stats.mem_percent)}%"></div>
								</div>
								<div class="mt-1.5 text-xs text-muted-foreground">{stats.mem_percent.toFixed(1)}% of {bytes(stats.mem_limit)}</div>
							</Card.Content>
						</Card.Root>
						<Card.Root>
							<Card.Content class="space-y-3">
								<div class="flex items-center justify-between"><span class="kicker">Network</span><ArrowDownUpIcon class="size-4 text-teal-500" /></div>
								<div class="flex gap-2 font-mono text-sm"><span class="text-muted-foreground">rx</span>{bytes(stats.net_rx)}</div>
								<div class="flex gap-2 font-mono text-sm"><span class="text-muted-foreground">tx</span>{bytes(stats.net_tx)}</div>
								<div class="flex items-center gap-1.5 text-xs text-muted-foreground"><BoxIcon class="size-3.5" /> {stats.pids} processes</div>
							</Card.Content>
						</Card.Root>
					{:else}
						<Skeleton class="col-span-2 h-44 rounded-xl" />
						<Skeleton class="h-44 rounded-xl" />
						<Skeleton class="h-44 rounded-xl" />
					{/if}
				</div>
				<p class="mt-3 flex items-center gap-2 font-mono text-xs text-muted-foreground">
					<span class="signal signal-live size-1.5 text-ok"></span> Refreshing every 3 seconds
				</p>
			{/if}
		</Tabs.Content>

		<Tabs.Content value="console">
			{#if info.State.Running}
				<div class="mb-3 flex flex-wrap items-center gap-2">
					<Select.Root type="single" bind:value={shell}>
						<Select.Trigger class="w-36 font-mono"><TerminalIcon class="size-4 text-muted-foreground" />{shell}</Select.Trigger>
						<Select.Content>
							{#each shells as s}<Select.Item value={s} label={s} class="font-mono" />{/each}
						</Select.Content>
					</Select.Root>
					<Button variant="outline" onclick={() => termKey++}><RefreshCwIcon /> Reconnect</Button>
				</div>
				{#key termKey + shell}
					<Terminal {id} cmd={shell} />
				{/key}
			{:else}
				<EmptyState icon={TerminalIcon} title="Console offline" description="Start the container to open a shell." />
			{/if}
		</Tabs.Content>

		<Tabs.Content value="inspect">
			<div class="relative">
				<div class="absolute top-2 right-2"><CopyId value={JSON.stringify(info, null, 2)} label="Copy JSON" class="bg-card px-2 py-1" /></div>
				<pre class="max-h-[70dvh] overflow-auto rounded-xl border bg-card p-4 text-xs leading-relaxed">{JSON.stringify(info, null, 2)}</pre>
			</div>
		</Tabs.Content>
	</Tabs.Root>
{/if}
