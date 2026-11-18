<script>
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { Button, buttonVariants } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Switch } from '$lib/components/ui/switch/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import TrashIcon from '@lucide/svelte/icons/trash';
	import RadarIcon from '@lucide/svelte/icons/radar';
	import Loader2Icon from '@lucide/svelte/icons/loader-2';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import CircleXIcon from '@lucide/svelte/icons/circle-x';
	import LockKeyholeIcon from '@lucide/svelte/icons/lock-keyhole';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import FormDialog from '$lib/components/FormDialog.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import CopyId from '$lib/components/CopyId.svelte';
	import { api, get, post, del } from '$lib/api.js';
	import { attempt, confirm } from '$lib/session.svelte.js';

	const patch = (path, body) => api('PATCH', path, body);
	const put = (path, body) => api('PUT', path, body);

	let providers = $state(null);
	let passwordLogin = $state(true);

	async function load() {
		const r = await attempt(() => get('/sso/providers'));
		if (r) {
			providers = r.providers;
			passwordLogin = r.password_login;
		}
	}
	$effect(() => {
		load();
	});

	const anyEnabled = $derived(!!providers?.some((p) => p.enabled));

	async function setPasswordLogin(on) {
		const r = await attempt(() => put('/sso/settings', { password_login: on }), on ? 'Password login is on' : 'Password login is off');
		if (r) passwordLogin = r.password_login;
		else load(); // put the switch back
	}

	async function setEnabled(p, on) {
		const r = await attempt(() => patch(`/sso/providers/${encodeURIComponent(p.slug)}`, { enabled: on }), `${p.display_name} ${on ? 'enabled' : 'disabled'}`);
		if (!r) load();
		else p.enabled = r.enabled;
	}

	async function remove(p) {
		const ok = await confirm({
			title: `Remove ${p.display_name}?`,
			description: 'People who only sign in through it lose access until they get a password or another provider. Their accounts stay.',
			action: 'Remove'
		});
		if (ok && (await attempt(() => del(`/sso/providers/${encodeURIComponent(p.slug)}`), `Removed ${p.display_name}`))) load();
	}

	// ---- add / edit dialog ----

	const blank = () => ({
		slug: '', display_name: '', issuer_url: '', client_id: '', client_secret: '',
		scopes: 'openid email profile', default_role: 'user', admin_group: '', groups_claim: 'groups',
		allowed_email_domains: '', auto_create_users: true, enabled: true
	});
	let open = $state(false);
	let editing = $state(null);
	let f = $state(blank());
	let discovery = $state(null);
	let discovering = $state(false);

	function openAdd() {
		editing = null;
		f = blank();
		discovery = null;
		open = true;
	}
	function openEdit(p) {
		editing = p;
		f = { ...blank(), ...p, client_secret: '' };
		discovery = null;
		open = true;
	}

	// The callback to register at the provider (the server's value once saved).
	const redirectURI = $derived(
		editing?.redirect_uri ?? `${location.origin}/api/v1/auth/oidc/${f.slug || '<slug>'}/callback`
	);

	async function discover() {
		discovering = true;
		discovery = (await attempt(() => post('/sso/providers/test', { issuer_url: f.issuer_url.trim() }))) ?? null;
		discovering = false;
	}

	async function save() {
		const b = { ...f };
		if (!b.client_secret) delete b.client_secret; // keep the stored one
		const saved = editing
			? await attempt(() => patch(`/sso/providers/${encodeURIComponent(editing.slug)}`, b), `Saved ${b.display_name || b.slug}`)
			: await attempt(() => post('/sso/providers', b), `Added ${b.display_name || b.slug}`);
		if (saved) load();
		return saved;
	}

	const roles = [
		['user', 'User'],
		['admin', 'Admin']
	];
</script>

<svelte:head><title>Sign-in · locci</title></svelte:head>

<PageHeader kicker="Harbour master · Gates" title="Sign-in" description="How people get in: passwords, single sign-on, or both.">
	{#snippet actions()}
		<Button onclick={openAdd}><PlusIcon /> Add provider</Button>
	{/snippet}
</PageHeader>

<Card.Root class="rise mb-6">
	<Card.Content>
		<div class="flex items-start justify-between gap-4">
		<div class="flex items-start gap-3">
			<div class="ribbed grid size-10 shrink-0 place-items-center rounded-lg border text-muted-foreground"><LockKeyholeIcon class="size-5" /></div>
			<div>
				<div class="font-heading font-semibold">Password login</div>
				<p class="mt-0.5 text-sm text-muted-foreground">
					{#if passwordLogin}
						On. People can sign in with a username and password{anyEnabled ? ' or single sign-on' : ''}.
					{:else}
						Off. Only single sign-on works. On the server, <code>locci auth password-login on</code> turns it back on.
					{/if}
				</p>
				{#if passwordLogin && !anyEnabled}
					<p class="mt-1 text-xs text-muted-foreground">Enable a provider below before turning it off.</p>
				{/if}
			</div>
		</div>
		<Switch
			checked={passwordLogin}
			disabled={!providers || (passwordLogin && !anyEnabled)}
			onCheckedChange={setPasswordLogin}
			aria-label="Password login"
		/>
		</div>
	</Card.Content>
</Card.Root>

{#if !providers}
	<div class="grid gap-4 md:grid-cols-2">{#each Array(2) as _}<Skeleton class="h-44 rounded-xl" />{/each}</div>
{:else if !providers.length}
	<EmptyState
		icon={KeyRoundIcon}
		title="No single sign-on yet"
		description="Password login is the only way in. Add an OpenID Connect provider such as Keycloak, Authentik, Zitadel, Google or Entra ID."
	>
		<Button size="sm" onclick={openAdd}><PlusIcon /> Add provider</Button>
	</EmptyState>
{:else}
	<ul class="grid gap-4 md:grid-cols-2">
		{#each providers as p, i (p.slug)}
			<li class="rise flex flex-col overflow-hidden rounded-xl border bg-card" style="animation-delay: {i * 50}ms">
				<div class="ribbed h-2 {p.enabled ? 'text-primary' : 'text-muted-foreground'} opacity-70"></div>
				<div class="flex items-start justify-between gap-3 p-4 pb-2">
					<div class="min-w-0">
						<div class="truncate font-heading text-lg font-semibold">{p.display_name}</div>
						<div class="truncate font-mono text-xs text-muted-foreground" title={p.issuer_url}>{p.issuer_url}</div>
					</div>
					<div class="flex items-center gap-2">
						<Switch checked={p.enabled} onCheckedChange={(on) => setEnabled(p, on)} aria-label="Enable {p.display_name}" />
						<DropdownMenu.Root>
							<DropdownMenu.Trigger class={buttonVariants({ variant: 'ghost', size: 'icon-sm' })} aria-label="Actions for {p.display_name}">
								<EllipsisIcon />
							</DropdownMenu.Trigger>
							<DropdownMenu.Content align="end" class="w-40">
								<DropdownMenu.Item onclick={() => openEdit(p)}><PencilIcon /> Edit</DropdownMenu.Item>
								<DropdownMenu.Separator />
								<DropdownMenu.Item variant="destructive" onclick={() => remove(p)}><TrashIcon /> Remove</DropdownMenu.Item>
							</DropdownMenu.Content>
						</DropdownMenu.Root>
					</div>
				</div>
				<div class="flex flex-wrap gap-1.5 px-4 text-xs">
					<Badge variant="outline" class="font-mono">{p.slug}</Badge>
					<Badge variant="secondary">{p.auto_create_users ? `new users: ${p.default_role}` : 'existing users only'}</Badge>
					{#if p.admin_group}<Badge variant="secondary">admins: {p.admin_group}</Badge>{/if}
					{#if p.allowed_email_domains}<Badge variant="secondary">@{p.allowed_email_domains}</Badge>{/if}
					{#if !p.has_secret}<Badge variant="outline">public client</Badge>{/if}
				</div>
				<div class="mt-auto px-4 pt-4 pb-4">
					<div class="kicker text-[0.55rem]">Redirect URI</div>
					<CopyId value={p.redirect_uri} label={p.redirect_uri} class="max-w-full" />
				</div>
			</li>
		{/each}
	</ul>
{/if}

<FormDialog
	bind:open
	kicker={editing ? 'Refit' : 'New gate'}
	title={editing ? `Edit ${editing.display_name}` : 'Add a sign-in provider'}
	description="Any OpenID Connect provider with discovery works. Register the redirect URI below as an allowed callback."
	submitLabel={editing ? 'Save changes' : 'Add provider'}
	busyLabel="Saving"
	icon={editing ? PencilIcon : PlusIcon}
	onsubmit={save}
	class="sm:max-w-xl"
>
	<div class="grid gap-4 sm:grid-cols-2">
		<div class="grid content-start gap-2">
			<Label for="p-name">Button label</Label>
			<Input id="p-name" bind:value={f.display_name} placeholder="Keycloak" />
		</div>
		<div class="grid content-start gap-2">
			<Label for="p-slug">Slug</Label>
			<Input id="p-slug" bind:value={f.slug} required disabled={!!editing} pattern="[a-z0-9][a-z0-9\-]{'{'}0,62{'}'}" placeholder="keycloak" class="font-mono" />
		</div>
	</div>

	<div class="grid content-start gap-2">
		<Label for="p-issuer">Issuer URL</Label>
		<div class="flex gap-2">
			<Input id="p-issuer" bind:value={f.issuer_url} required placeholder="https://id.example.com/realms/main" class="font-mono" />
			<Button type="button" variant="outline" onclick={discover} disabled={discovering || !f.issuer_url}>
				{#if discovering}<Loader2Icon class="animate-spin" />{:else}<RadarIcon />{/if} Discover
			</Button>
		</div>
		{#if discovery}
			<div class="rounded-lg border p-3 text-xs {discovery.ok ? 'border-ok/30 bg-ok/5' : 'border-destructive/30 bg-destructive/5'}" role="status">
				{#if discovery.ok}
					<div class="flex items-center gap-1.5 font-medium text-ok"><CircleCheckIcon class="size-3.5" /> Found the provider</div>
					<div class="mt-1 break-all text-muted-foreground">Authorize: {discovery.authorization_endpoint}</div>
					{#each discovery.warnings as w}<div class="mt-1 text-warn">{w}</div>{/each}
				{:else}
					<div class="flex items-center gap-1.5 font-medium text-destructive"><CircleXIcon class="size-3.5" /> Not found</div>
					<div class="mt-1 break-words text-muted-foreground">{discovery.error}</div>
				{/if}
			</div>
		{/if}
	</div>

	<div class="grid gap-4 sm:grid-cols-2">
		<div class="grid content-start gap-2">
			<Label for="p-client">Client ID</Label>
			<Input id="p-client" bind:value={f.client_id} required class="font-mono" />
		</div>
		<div class="grid content-start gap-2">
			<Label for="p-secret">Client secret</Label>
			<Input
				id="p-secret"
				type="password"
				bind:value={f.client_secret}
				autocomplete="off"
				placeholder={editing?.has_secret ? 'Stored. Blank keeps it.' : 'Empty for a public client'}
			/>
		</div>
	</div>

	<div class="grid content-start gap-2">
		<Label>Redirect URI to register</Label>
		<CopyId value={redirectURI} label={redirectURI} class="max-w-full rounded-md border px-2 py-1.5" />
	</div>

	<div class="grid gap-4 sm:grid-cols-2">
		<div class="grid content-start gap-2">
			<Label for="p-domains">Allowed email domains</Label>
			<Input id="p-domains" bind:value={f.allowed_email_domains} placeholder="example.com, corp.io (empty = any)" />
		</div>
		<div class="grid content-start gap-2">
			<Label for="p-admin">Admin group</Label>
			<Input id="p-admin" bind:value={f.admin_group} placeholder="locci-admins (empty = roles not synced)" />
		</div>
	</div>

	<label class="flex items-center justify-between gap-3 rounded-lg border p-3">
		<span>
			<span class="block text-sm font-medium">Create accounts on first sign-in</span>
			<span class="block text-xs text-muted-foreground">Otherwise only existing users (username = verified email) can use it</span>
		</span>
		<Switch bind:checked={f.auto_create_users} />
	</label>

	<div class="grid gap-4 sm:grid-cols-3">
		<div class="grid content-start gap-2">
			<Label>New users are</Label>
			<Select.Root type="single" bind:value={f.default_role}>
				<Select.Trigger class="w-full">{roles.find(([v]) => v === f.default_role)?.[1]}</Select.Trigger>
				<Select.Content>{#each roles as [value, label]}<Select.Item {value} {label} />{/each}</Select.Content>
			</Select.Root>
		</div>
		<div class="grid content-start gap-2">
			<Label for="p-claim">Groups claim</Label>
			<Input id="p-claim" bind:value={f.groups_claim} class="font-mono" />
		</div>
		<div class="grid content-start gap-2">
			<Label for="p-scopes">Scopes</Label>
			<Input id="p-scopes" bind:value={f.scopes} class="font-mono" />
		</div>
	</div>

	<label class="flex items-center justify-between gap-3 rounded-lg border p-3">
		<span>
			<span class="block text-sm font-medium">Show on the login page</span>
			<span class="block text-xs text-muted-foreground">Disabled providers can't be used at all</span>
		</span>
		<Switch bind:checked={f.enabled} />
	</label>
</FormDialog>
