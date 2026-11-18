<script>
	import { goto } from '$app/navigation';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Button, buttonVariants } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Textarea } from '$lib/components/ui/textarea/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import { Switch } from '$lib/components/ui/switch/index.js';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import CopyId from '$lib/components/CopyId.svelte';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import ServerIcon from '@lucide/svelte/icons/server';
	import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
	import StarIcon from '@lucide/svelte/icons/star';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import TrashIcon from '@lucide/svelte/icons/trash';
	import PlugIcon from '@lucide/svelte/icons/plug';
	import Loader2Icon from '@lucide/svelte/icons/loader-2';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import CircleXIcon from '@lucide/svelte/icons/circle-x';
	import FileUpIcon from '@lucide/svelte/icons/file-up';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import FormDialog from '$lib/components/FormDialog.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import { api, get, post, del } from '$lib/api.js';
	import { bytes, ago } from '$lib/format.js';
	import { attempt, confirm, session } from '$lib/session.svelte.js';
	import { envs, loadEnvs, statusTone } from '$lib/env.svelte.js';

	const isAdmin = $derived(session.user?.role === 'admin');
	const patch = (path, body) => api('PATCH', path, body);

	// Host details per environment name: undefined = loading, null = unavailable.
	let details = $state({});

	function loadDetails(list) {
		for (const e of list) {
			if (e.status === 'down') {
				details[e.name] = null;
				continue;
			}
			get(`/environments/${encodeURIComponent(e.name)}/info`)
				.then((i) => (details[e.name] = i))
				.catch(() => (details[e.name] = null));
		}
	}

	async function refresh() {
		const list = await attempt(loadEnvs);
		if (list) loadDetails(list);
	}
	$effect(() => {
		refresh();
	});

	const where = (e) =>
		e.kind === 'agent'
			? e.agent_connected
				? 'outbound agent, connected'
				: 'outbound agent, waiting for it to connect'
			: e.url || 'DOCKER_HOST or local socket';

	// A freshly issued agent token, shown once with the commands to start the agent.
	let issued = $state(null); // { name, token }

	async function rotate(e) {
		const ok = await confirm({
			title: `New agent token for ${e.label}?`,
			description: 'The agent using the current token is disconnected and must be restarted with the new one.',
			action: 'Rotate token'
		});
		if (!ok) return;
		const r = await attempt(() => patch(`/environments/${encodeURIComponent(e.name)}`, { rotate_agent_token: true }), 'New agent token issued');
		if (r?.agent_token) issued = { name: r.name, token: r.agent_token };
		refresh();
	}

	async function setDefault(e) {
		if (await attempt(() => post(`/environments/${encodeURIComponent(e.name)}/default`), `${e.label} is now the default`))
			refresh();
	}

	async function remove(e) {
		const ok = await confirm({
			title: `Remove ${e.label}?`,
			description: 'locci forgets this host and its stored keys. Nothing on the host itself is touched.',
			action: 'Remove'
		});
		if (ok && (await attempt(() => del(`/environments/${encodeURIComponent(e.name)}`), `Removed ${e.label}`)))
			refresh();
	}

	// ---- add / edit dialog ----

	const kinds = [
		['ssh', 'SSH', 'Reach the Docker socket over SSH; no Docker port exposed'],
		['tcp+tls', 'TCP + TLS', "Docker's TLS port (2376) with client certificates"],
		['local', 'Local socket', 'A socket or URL reachable from this server'],
		['agent', 'Agent (outbound)', 'The host runs locci agent and dials out; works behind NAT, no open ports']
	];
	const blank = () => ({
		name: '', label: '', kind: 'ssh', url: '', tls_ca: '', tls_cert: '', tls_key: '', ssh_key: '', reset_host_key: false
	});

	let dialogOpen = $state(false);
	let editing = $state(null); // the environment being edited, or null when adding
	let f = $state(blank());
	let test = $state(null); // { ok, host?, error? }
	let testing = $state(false);

	function openAdd() {
		editing = null;
		f = blank();
		dialogOpen = true;
	}

	// Name and kind are fixed; certificates are prefilled; keys stay blank
	// (blank = keep the stored one, which is never sent back to the browser).
	function openEdit(e) {
		editing = e;
		f = { ...blank(), name: e.name, label: e.label, kind: e.kind, url: e.url, tls_ca: e.tls_ca ?? '', tls_cert: e.tls_cert ?? '' };
		dialogOpen = true;
	}

	// Any change to the connection invalidates an earlier test result.
	$effect(() => {
		void [f.kind, f.url, f.tls_ca, f.tls_cert, f.tls_key, f.ssh_key, f.reset_host_key, dialogOpen];
		test = null;
	});

	const placeholder = $derived(
		{ ssh: 'ssh://deploy@10.0.0.5', 'tcp+tls': 'tcp://10.0.0.5:2376', local: 'unix:///var/run/docker.sock (empty = DOCKER_HOST)' }[f.kind]
	);
	// When editing, the server already holds a key unless the kind is local.
	const keyOptional = $derived(!!editing?.has_secret);
	const urlChanged = $derived(!!editing && f.url.trim() !== editing.url);

	function body() {
		const b = { name: f.name.trim(), label: f.label.trim(), kind: f.kind, url: f.url.trim() };
		if (f.kind === 'tcp+tls') {
			Object.assign(b, { tls_ca: f.tls_ca, tls_cert: f.tls_cert });
			if (f.tls_key) b.tls_key = f.tls_key;
		}
		if (f.kind === 'ssh' && f.ssh_key) b.ssh_key = f.ssh_key;
		if (editing && f.reset_host_key) b.reset_host_key = true;
		return b;
	}

	async function runTest() {
		testing = true;
		test = (await attempt(() => post('/environments/test', body()))) ?? null;
		testing = false;
	}

	async function save() {
		const b = body();
		// Pin exactly the SSH host key that the test showed.
		if (test?.ok && test.host?.ssh_host_key) b.ssh_host_key = test.host.ssh_host_key;
		const label = b.label || b.name;
		const saved = editing
			? await attempt(() => patch(`/environments/${encodeURIComponent(editing.name)}`, b), `Saved ${label}`)
			: await attempt(() => post('/environments', b), `Added ${label}`);
		if (saved?.agent_token) issued = { name: saved.name, token: saved.agent_token };
		if (saved) refresh();
		return saved;
	}

	const server = typeof location === 'undefined' ? '' : location.origin;

	async function loadFile(field, e) {
		const file = e.currentTarget.files?.[0];
		if (file) f[field] = await file.text();
		e.currentTarget.value = '';
	}
</script>

<svelte:head><title>Environments · locci</title></svelte:head>

<PageHeader kicker="Fleet · Hosts" title="Environments" description="Every Docker host this locci manages.">
	{#snippet actions()}
		<Button variant="outline" size="icon" onclick={refresh} aria-label="Refresh"><RefreshCwIcon /></Button>
		{#if isAdmin}<Button onclick={openAdd}><PlusIcon /> Add environment</Button>{/if}
	{/snippet}
</PageHeader>

{#if !envs.loaded}
	<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
		{#each Array(3) as _}<Skeleton class="h-52 rounded-xl" />{/each}
	</div>
{:else if !envs.list.length}
	<EmptyState icon={ServerIcon} title="No environments" description="Add a Docker host to get started." />
{:else}
	<ul class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
		{#each envs.list as e, i (e.name)}
			{@const d = details[e.name]}
			<li class="rise group relative flex flex-col overflow-hidden rounded-xl border bg-card transition-all hover:-translate-y-0.5 hover:shadow-lg hover:shadow-primary/5" style="animation-delay: {i * 50}ms">
				<div class="ribbed h-2 {e.status === 'up' ? 'text-ok' : e.status === 'down' ? 'text-destructive' : 'text-muted-foreground'} opacity-70"></div>
				<div class="flex items-start justify-between gap-2 p-4 pb-2">
					<a href="/e/{encodeURIComponent(e.name)}" class="min-w-0 flex-1 after:absolute after:inset-0">
						<div class="flex items-center gap-2">
							<span class="signal size-2 {statusTone(e.status)}"></span>
							<span class="truncate font-heading text-lg font-semibold">{e.label}</span>
						</div>
						<div class="mt-0.5 font-mono text-xs text-muted-foreground">{e.name}</div>
					</a>
					<div class="relative z-10 flex items-center gap-1.5">
						{#if e.is_default}<Badge variant="secondary" class="gap-1"><StarIcon class="size-3" />default</Badge>{/if}
						{#if isAdmin}
							<DropdownMenu.Root>
								<DropdownMenu.Trigger class={buttonVariants({ variant: 'ghost', size: 'icon-sm' })} aria-label="Actions for {e.label}">
									<EllipsisIcon />
								</DropdownMenu.Trigger>
								<DropdownMenu.Content align="end" class="w-48">
									<DropdownMenu.Item onclick={() => goto(`/e/${encodeURIComponent(e.name)}`)}><ServerIcon /> Open</DropdownMenu.Item>
									<DropdownMenu.Item onclick={() => openEdit(e)}><PencilIcon /> Edit</DropdownMenu.Item>
									{#if e.kind === 'agent'}
										<DropdownMenu.Item onclick={() => rotate(e)}><KeyRoundIcon /> Rotate agent token</DropdownMenu.Item>
									{/if}
									{#if !e.is_default}
										<DropdownMenu.Item onclick={() => setDefault(e)}><StarIcon /> Set as default</DropdownMenu.Item>
									{/if}
									<DropdownMenu.Item onclick={refresh}><RefreshCwIcon /> Check now</DropdownMenu.Item>
									<DropdownMenu.Separator />
									<DropdownMenu.Item variant="destructive" disabled={e.is_default} onclick={() => remove(e)}>
										<TrashIcon /> Remove
									</DropdownMenu.Item>
								</DropdownMenu.Content>
							</DropdownMenu.Root>
						{/if}
					</div>
				</div>

				<div class="flex flex-wrap items-center gap-1.5 px-4 text-xs text-muted-foreground">
					<Badge variant="outline" class="font-mono">{e.kind}</Badge>
					<span class="truncate font-mono" title={where(e)}>{where(e)}</span>
				</div>

				<div class="mt-auto px-4 pt-4 pb-4">
					{#if d === undefined}
						<div class="grid grid-cols-2 gap-2">
							{#each Array(2) as _}<Skeleton class="h-10 rounded-md" />{/each}
						</div>
					{:else if d === null}
						<p class="rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">
							Unreachable{e.last_seen_at ? `, last seen ${ago(e.last_seen_at)}` : ''}
						</p>
					{:else}
						<dl class="grid grid-cols-2 gap-2 text-xs">
							<div class="rounded-md bg-muted/60 px-2 py-1.5"><dt class="kicker text-[0.55rem]">Docker</dt><dd class="truncate font-mono">{d.ServerVersion}</dd></div>
							<div class="rounded-md bg-muted/60 px-2 py-1.5"><dt class="kicker text-[0.55rem]">Running</dt><dd class="font-mono"><span class="text-ok">{d.ContainersRunning}</span> of {d.Containers}</dd></div>
						</dl>
						<p class="mt-2 truncate text-xs text-muted-foreground" title="{d.OperatingSystem} · {d.Architecture}">
							{d.NCPU} CPUs · {bytes(d.MemTotal)} · {d.OperatingSystem}
						</p>
					{/if}
				</div>
			</li>
		{/each}
	</ul>
{/if}

{#snippet pemField(field, label, hint, secret = false)}
	<div class="grid content-start gap-2">
		<div class="flex items-center justify-between">
			<Label for="env-{field}">{label}</Label>
			<label class="{buttonVariants({ variant: 'ghost', size: 'xs' })} cursor-pointer text-muted-foreground">
				<FileUpIcon /> Load file
				<input type="file" class="sr-only" onchange={(e) => loadFile(field, e)} />
			</label>
		</div>
		<Textarea
			id="env-{field}"
			bind:value={f[field]}
			required={!(secret && keyOptional)}
			placeholder={secret && keyOptional ? 'Stored. Leave blank to keep it, or paste a new one.' : '-----BEGIN ...'}
			class="min-h-20 font-mono text-[0.7rem]"
		/>
		{#if hint}<p class="text-xs text-muted-foreground">{hint}</p>{/if}
	</div>
{/snippet}

<FormDialog
	bind:open={dialogOpen}
	kicker={editing ? 'Refit' : 'New berth'}
	title={editing ? `Edit ${editing.label}` : 'Add an environment'}
	description={editing
		? 'Name and connection kind are fixed. Leave a key blank to keep the stored one.'
		: 'Connect another Docker host. Keys are stored encrypted and never shown again.'}
	submitLabel={editing ? 'Save changes' : 'Save environment'}
	busyLabel="Saving"
	icon={editing ? PencilIcon : PlusIcon}
	onsubmit={save}
	class="sm:max-w-xl"
>
	<div class="grid gap-4 sm:grid-cols-2">
		<div class="grid content-start gap-2">
			<Label for="env-name">Name</Label>
			<Input id="env-name" bind:value={f.name} disabled={!!editing} required pattern="[a-z0-9][a-z0-9\-]{'{'}0,62{'}'}" placeholder="edge" class="font-mono" />
			<p class="text-xs text-muted-foreground">Used in URLs and the CLI. Lowercase, digits, dashes.</p>
		</div>
		<div class="grid content-start gap-2">
			<Label for="env-label">Display name</Label>
			<Input id="env-label" bind:value={f.label} placeholder={f.name || 'Edge box'} />
		</div>
	</div>

	<div class="grid content-start gap-2">
		<Label>Connection</Label>
		<Select.Root type="single" bind:value={f.kind} disabled={!!editing}>
			<Select.Trigger class="w-full">{kinds.find(([k]) => k === f.kind)?.[1]}</Select.Trigger>
			<Select.Content>
				{#each kinds as [value, label, hint]}
					<Select.Item {value} {label}>
						<div><div>{label}</div><div class="text-xs text-muted-foreground">{hint}</div></div>
					</Select.Item>
				{/each}
			</Select.Content>
		</Select.Root>
	</div>

	{#if f.kind === 'agent'}
		<p class="rounded-lg border bg-muted/40 p-3 text-xs text-muted-foreground">
			No address needed: after saving you get a token, and the host connects with
			<code>locci agent</code>. It only needs to reach this server over HTTPS.
		</p>
	{:else}
	<div class="grid content-start gap-2">
		<Label for="env-url">URL</Label>
		<Input id="env-url" bind:value={f.url} required={f.kind !== 'local'} placeholder={placeholder} class="font-mono" />
		{#if f.kind === 'ssh'}
			<p class="text-xs text-muted-foreground">
				Add a path for a non-default socket, e.g. <code>ssh://me@host/run/user/1000/docker.sock</code>. The server's sshd must allow
				<code>AllowStreamLocalForwarding</code>.
			</p>
		{/if}
	</div>
	{/if}

	{#if f.kind === 'ssh' && editing?.ssh_fingerprint}
		<div class="flex items-start justify-between gap-3 rounded-lg border p-3">
			<div class="min-w-0 text-xs">
				<div class="flex items-center gap-1.5 text-sm font-medium"><KeyRoundIcon class="size-3.5" /> Pinned host key</div>
				<code class="mt-1 block break-all text-muted-foreground">{editing.ssh_fingerprint}</code>
				<p class="mt-1 text-muted-foreground">
					{urlChanged ? 'The URL changed, so this pin is dropped and the new host key is pinned on save.' : 'Forget it if the host was rebuilt and its key changed on purpose.'}
				</p>
			</div>
			<Switch bind:checked={f.reset_host_key} disabled={urlChanged} aria-label="Forget pinned host key" />
		</div>
	{/if}

	{#if f.kind === 'ssh'}
		{@render pemField('ssh_key', 'Private key', 'An unencrypted deploy key whose public half is in the remote authorized_keys.', true)}
	{:else if f.kind === 'tcp+tls'}
		{@render pemField('tls_ca', 'CA certificate', '')}
		<div class="grid gap-4 sm:grid-cols-2">
			{@render pemField('tls_cert', 'Client certificate', '')}
			{@render pemField('tls_key', 'Client key', '', true)}
		</div>
	{/if}

	{#if test}
		<div class="rounded-lg border p-3 text-sm {test.ok ? 'border-ok/30 bg-ok/5' : 'border-destructive/30 bg-destructive/5'}" role="status">
			{#if test.ok}
				<div class="flex items-center gap-2 font-medium text-ok"><CircleCheckIcon class="size-4" /> Connected to {test.host.name}</div>
				<p class="mt-1 text-xs text-muted-foreground">
					Docker {test.host.server_version} · {test.host.os} {test.host.arch} · {test.host.cpus} CPUs · {bytes(test.host.memory)} · {test.host.containers} containers
				</p>
				{#if test.host.ssh_fingerprint}
					<p class="mt-2 flex items-start gap-1.5 text-xs">
						<KeyRoundIcon class="mt-0.5 size-3.5 shrink-0" />
						<span>Host key <code class="break-all">{test.host.ssh_fingerprint}</code>. Compare it with <code>ssh-keygen -lf</code> on the host; it is pinned when you save.</span>
					</p>
				{/if}
			{:else}
				<div class="flex items-center gap-2 font-medium text-destructive"><CircleXIcon class="size-4" /> Can't connect</div>
				<p class="mt-1 text-xs break-words text-muted-foreground">{test.error}</p>
			{/if}
		</div>
	{/if}

	{#snippet extra()}
		<Button type="button" variant="outline" onclick={runTest} disabled={testing || f.kind === 'agent' || (f.kind !== 'local' && !f.url)}>
			{#if testing}<Loader2Icon class="animate-spin" /> Testing{:else}<PlugIcon /> Test connection{/if}
		</Button>
	{/snippet}
</FormDialog>

<Dialog.Root open={!!issued} onOpenChange={(o) => !o && (issued = null)}>
	<Dialog.Content class="max-h-[90dvh] overflow-y-auto sm:max-w-xl">
		<div class="ribbed absolute inset-x-0 top-0 h-1.5 text-primary" aria-hidden="true"></div>
		<Dialog.Header>
			<div class="kicker">Shown once</div>
			<Dialog.Title class="font-heading text-xl">Start the agent for {issued?.name}</Dialog.Title>
			<Dialog.Description>
				Copy the token now: locci keeps only its hash. Run one of these on the Docker host; it connects out to this
				server and shows up as connected within a few seconds.
			</Dialog.Description>
		</Dialog.Header>
		{#if issued}
			<div class="grid gap-3 text-xs">
				<div class="grid gap-1.5">
					<span class="kicker text-[0.6rem]">Agent token</span>
					<CopyId value={issued.token} label={issued.token} class="max-w-full rounded-md border px-2 py-1.5" />
				</div>
				<div class="grid gap-1.5">
					<span class="kicker text-[0.6rem]">With the locci binary</span>
					<pre class="overflow-x-auto rounded-md bg-muted p-3 whitespace-pre-wrap break-all">LOCCI_AGENT_TOKEN={issued.token} locci agent --server {server}</pre>
				</div>
				<div class="grid gap-1.5">
					<span class="kicker text-[0.6rem]">In Docker</span>
					<pre class="overflow-x-auto rounded-md bg-muted p-3 whitespace-pre-wrap break-all">docker run -d --name locci-agent --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e LOCCI_AGENT_TOKEN={issued.token} \
  &lt;locci image&gt; agent --server {server}</pre>
				</div>
			</div>
		{/if}
		<Dialog.Footer>
			<Button onclick={() => (issued = null)}>Done</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
