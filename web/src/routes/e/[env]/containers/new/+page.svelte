<script>
	import { goto } from '$app/navigation';
	import * as Card from '$lib/components/ui/card/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Textarea } from '$lib/components/ui/textarea/index.js';
	import { Switch } from '$lib/components/ui/switch/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import Loader2Icon from '@lucide/svelte/icons/loader-2';
	import ShipIcon from '@lucide/svelte/icons/ship';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { href } from '$lib/env.svelte.js';
	import { get, post } from '$lib/api.js';
	import { parsePairs, lines } from '$lib/format.js';
	import { attempt } from '$lib/session.svelte.js';

	let f = $state({
		name: '',
		image: '',
		subdomain: '',
		ports: '',
		env: '',
		volumes: '',
		labels: '',
		command: '',
		networks: [],
		start: true
	});
	let networks = $state(null);
	let busy = $state(false);

	$effect(() => {
		attempt(async () => (networks = (await get('/networks')).networks.map((n) => n.Name)));
	});

	const spec = $derived.by(() => {
		// "80:8080" lines -> { "80": "8080" } (container port -> host port)
		const ports = {};
		for (const l of lines(f.ports)) {
			const [c, h = ''] = l.split(':');
			ports[c] = h;
		}
		return {
			name: f.name,
			image: f.image,
			subdomain: f.subdomain || f.name,
			ports,
			environment: lines(f.env),
			volumes: lines(f.volumes),
			labels: parsePairs(f.labels),
			networks: f.networks,
			command: f.command.trim() ? f.command.trim().split(/\s+/) : []
		};
	});

	async function submit(e) {
		e.preventDefault();
		busy = true;
		const created = await attempt(async () => {
			const c = await post('/containers', $state.snapshot(spec));
			if (f.start) await post(`/containers/${c.container_id}/start`);
			return c;
		}, `Created ${f.name}`);
		busy = false;
		if (created?.container_id) goto(href(`/containers/${created.container_id}`));
	}

	const netLabel = $derived(
		f.networks.length === 0
			? 'Default bridge'
			: f.networks.length <= 2
				? f.networks.join(', ')
				: `${f.networks.length} networks`
	);
</script>

<svelte:head><title>New container · locci</title></svelte:head>

<Button href={href('/containers')} variant="ghost" size="sm" class="-ml-2 mb-2 text-muted-foreground">
	<ChevronLeftIcon /> Containers
</Button>
<PageHeader kicker="Manifest 01 · New" title="Ship a container" description="Pulls the image if needed, then creates the container." />

<form onsubmit={submit} class="grid gap-6 pb-24 lg:grid-cols-[1fr_340px] lg:pb-0">
	<div class="space-y-5">
		<Card.Root class="rise">
			<Card.Header>
				<div class="kicker">01 · Cargo</div>
				<Card.Title class="font-heading">Image and identity</Card.Title>
			</Card.Header>
			<Card.Content class="grid gap-4 sm:grid-cols-2">
				<div class="space-y-2">
					<Label for="name">Name <span class="text-primary">*</span></Label>
					<Input id="name" bind:value={f.name} required pattern="[a-zA-Z0-9][a-zA-Z0-9_.\-]*" placeholder="web" />
				</div>
				<div class="space-y-2">
					<Label for="image">Image <span class="text-primary">*</span></Label>
					<Input id="image" bind:value={f.image} required placeholder="nginx:alpine" class="font-mono" />
				</div>
				<div class="space-y-2">
					<Label for="subdomain">Subdomain</Label>
					<Input id="subdomain" bind:value={f.subdomain} placeholder={f.name || 'defaults to the name'} />
				</div>
				<div class="space-y-2">
					<Label for="command">Command</Label>
					<Input id="command" bind:value={f.command} placeholder="overrides the image CMD" class="font-mono" />
				</div>
			</Card.Content>
		</Card.Root>

		<Card.Root class="rise" style="animation-delay: 60ms">
			<Card.Header>
				<div class="kicker">02 · Berths</div>
				<Card.Title class="font-heading">Ports and networks</Card.Title>
			</Card.Header>
			<Card.Content class="grid gap-4 sm:grid-cols-2">
				<div class="space-y-2">
					<Label for="ports">Ports</Label>
					<Textarea id="ports" bind:value={f.ports} placeholder="80:8080" class="min-h-24 font-mono text-xs" />
					<p class="text-xs text-muted-foreground">CONTAINER:HOST per line. Leave HOST empty or 0 for a random port.</p>
				</div>
				<div class="space-y-2">
					<Label>Networks</Label>
					{#if networks}
						<Select.Root type="multiple" bind:value={f.networks}>
							<Select.Trigger class="w-full">
								<span class="truncate {f.networks.length ? '' : 'text-muted-foreground'}">{netLabel}</span>
							</Select.Trigger>
							<Select.Content>
								{#each networks as n}<Select.Item value={n} label={n} />{/each}
							</Select.Content>
						</Select.Root>
					{:else}
						<Skeleton class="h-9 w-full" />
					{/if}
					<p class="text-xs text-muted-foreground">Pick any number. None means the default bridge.</p>
				</div>
			</Card.Content>
		</Card.Root>

		<Card.Root class="rise" style="animation-delay: 120ms">
			<Card.Header>
				<div class="kicker">03 · Hold</div>
				<Card.Title class="font-heading">Environment, volumes and labels</Card.Title>
			</Card.Header>
			<Card.Content class="grid gap-4 md:grid-cols-3">
				<div class="space-y-2">
					<Label for="env">Environment</Label>
					<Textarea id="env" bind:value={f.env} placeholder="KEY=value" class="min-h-28 font-mono text-xs" />
				</div>
				<div class="space-y-2">
					<Label for="volumes">Volumes</Label>
					<Textarea id="volumes" bind:value={f.volumes} placeholder="data:/var/lib/data" class="min-h-28 font-mono text-xs" />
				</div>
				<div class="space-y-2">
					<Label for="labels">Labels</Label>
					<Textarea id="labels" bind:value={f.labels} placeholder="team=web" class="min-h-28 font-mono text-xs" />
				</div>
				<p class="text-xs text-muted-foreground md:col-span-3">One entry per line. Volumes take a name or host path, then the container path.</p>
			</Card.Content>
		</Card.Root>
	</div>

	<aside class="lg:sticky lg:top-10 lg:self-start">
		<Card.Root class="rise overflow-hidden" style="animation-delay: 180ms">
			<div class="ribbed h-2 text-primary"></div>
			<Card.Header>
				<div class="kicker">Manifest preview</div>
				<Card.Title class="truncate font-heading">{f.name || 'unnamed'}</Card.Title>
				<Card.Description class="truncate font-mono text-xs">{f.image || 'no image yet'}</Card.Description>
			</Card.Header>
			<Card.Content class="space-y-4">
				<pre class="hidden max-h-72 overflow-auto rounded-lg bg-muted/60 p-3 text-[0.7rem] leading-relaxed lg:block">{JSON.stringify(spec, null, 2)}</pre>
				<label class="flex items-center justify-between gap-3 rounded-lg border p-3">
					<span>
						<span class="block text-sm font-medium">Start after creating</span>
						<span class="block text-xs text-muted-foreground">Launch as soon as it is built</span>
					</span>
					<Switch bind:checked={f.start} />
				</label>
			</Card.Content>
			<Card.Footer
				class="fixed inset-x-0 bottom-0 z-20 gap-2 border-t bg-background/90 p-3 backdrop-blur-md lg:static lg:bg-transparent lg:p-6 lg:pt-0 lg:backdrop-blur-none"
			>
				<Button href={href('/containers')} variant="outline" class="flex-1 lg:hidden">Cancel</Button>
				<Button type="submit" class="flex-1" disabled={busy}>
					{#if busy}<Loader2Icon class="animate-spin" /> Pulling and creating{:else}<ShipIcon /> Create container{/if}
				</Button>
			</Card.Footer>
		</Card.Root>
	</aside>
</form>
