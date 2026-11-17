<script>
	import * as Select from '$lib/components/ui/select/index.js';
	import * as Avatar from '$lib/components/ui/avatar/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import * as Table from '$lib/components/ui/table/index.js';
	import UserPlusIcon from '@lucide/svelte/icons/user-plus';
	import TrashIcon from '@lucide/svelte/icons/trash';
	import ShieldIcon from '@lucide/svelte/icons/shield';
	import UsersIcon from '@lucide/svelte/icons/users';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import ListSkeleton from '$lib/components/ListSkeleton.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import FormDialog from '$lib/components/FormDialog.svelte';
	import { get, post, del } from '$lib/api.js';
	import { attempt, confirm, session } from '$lib/session.svelte.js';

	let users = $state([]);
	let loading = $state(true);
	let f = $state({ username: '', password: '', role: 'user' });
	let addOpen = $state(false);

	async function load() {
		await attempt(async () => (users = (await get('/users')).users));
		loading = false;
	}
	$effect(() => {
		load();
	});

	async function create() {
		const ok = await attempt(() => post('/users', $state.snapshot(f)), `Created ${f.username}`);
		if (ok) {
			f = { username: '', password: '', role: 'user' };
			load();
		}
		return ok;
	}

	async function remove(u) {
		const ok = await confirm({ title: `Delete ${u.username}?`, description: 'Their API tokens stop working at once.', action: 'Delete user' });
		if (!ok) return;
		await attempt(() => del(`/users/${encodeURIComponent(u.username)}`), `Deleted ${u.username}`);
		load();
	}

	const isMe = (u) => u.username === session.user?.username;
	const when = (s) => {
		const d = new Date(s);
		return isNaN(d) ? s : d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
	};
</script>

<svelte:head><title>Users · locci</title></svelte:head>

<PageHeader kicker="Harbour master · Crew" title="Users" description="Who can come aboard, and what they can touch.">
	{#snippet actions()}
		<Button onclick={() => (addOpen = true)}><UserPlusIcon /> Add user</Button>
	{/snippet}
</PageHeader>

<FormDialog
	bind:open={addOpen}
	kicker="Sign on"
	title="Add a user"
	description="They sign in with this username and password."
	submitLabel="Add user"
	busyLabel="Adding"
	icon={UserPlusIcon}
	onsubmit={create}
>
	<div class="grid gap-2">
		<Label for="u-name">Username</Label>
		<Input id="u-name" bind:value={f.username} required autocomplete="off" />
	</div>
	<div class="grid gap-2">
		<Label for="u-pass">Password</Label>
		<Input id="u-pass" type="password" bind:value={f.password} minlength={8} required autocomplete="new-password" placeholder="8+ characters" />
	</div>
	<div class="grid gap-2">
		<Label>Role</Label>
		<Select.Root type="single" bind:value={f.role}>
			<Select.Trigger class="w-full capitalize">{f.role}</Select.Trigger>
			<Select.Content>
				<Select.Item value="user" label="User">
					<div><div>User</div><div class="text-xs text-muted-foreground">Manage containers and images</div></div>
				</Select.Item>
				<Select.Item value="admin" label="Admin">
					<div><div>Admin</div><div class="text-xs text-muted-foreground">Also users and the audit log</div></div>
				</Select.Item>
			</Select.Content>
		</Select.Root>
	</div>
</FormDialog>

{#if loading}
	<ListSkeleton rows={4} cols={4} />
{:else if !users.length}
	<EmptyState icon={UsersIcon} title="No crew yet" />
{:else}
	{#snippet avatar(u)}
		<Avatar.Root class="size-9 rounded-lg">
			<Avatar.Fallback class="ribbed rounded-lg font-mono text-xs font-semibold {u.role === 'admin' ? 'bg-primary/15 text-primary' : 'bg-muted'}">
				{u.username.slice(0, 2).toUpperCase()}
			</Avatar.Fallback>
		</Avatar.Root>
	{/snippet}
	{#snippet role(u)}
		{#if u.role === 'admin'}<span class="flex items-center gap-1 text-xs text-primary"><ShieldIcon class="size-3" />admin</span>{:else}<span class="text-xs text-muted-foreground">user</span>{/if}
	{/snippet}
	{#snippet youBadge(u)}
		{#if isMe(u)}<Badge variant="outline" class="text-[0.65rem]">you</Badge>{/if}
		{#if !u.has_password}<Badge variant="secondary" class="text-[0.65rem]" title="Signs in through single sign-on only">SSO</Badge>{/if}
	{/snippet}
	{#snippet deleteBtn(u)}
		{#if !isMe(u)}
			<Button variant="ghost" size="icon-sm" class="text-muted-foreground hover:text-destructive" onclick={() => remove(u)} aria-label="Delete {u.username}"><TrashIcon /></Button>
		{/if}
	{/snippet}

	<DataTable
		items={users}
		key={(u) => u.id}
		noun="users"
		columns={[{ label: 'User' }, { label: 'Role' }, { label: 'Since' }, { label: '' }]}
	>
		{#snippet row(u)}
			<Table.Cell class="pl-4">
				<div class="flex items-center gap-3">
					{@render avatar(u)}
					<span class="font-medium">{u.username}</span>
					{@render youBadge(u)}
				</div>
			</Table.Cell>
			<Table.Cell>{@render role(u)}</Table.Cell>
			<Table.Cell class="text-xs text-muted-foreground">{when(u.created_at)}</Table.Cell>
			<Table.Cell class="pr-4 text-right">{@render deleteBtn(u)}</Table.Cell>
		{/snippet}
		{#snippet card(u)}
			<div class="flex items-center gap-3">
				{@render avatar(u)}
				<div class="min-w-0 flex-1">
					<div class="flex items-center gap-2"><span class="truncate font-medium">{u.username}</span>{@render youBadge(u)}</div>
					<div class="flex items-center gap-2 text-xs text-muted-foreground">{@render role(u)}<span>since {when(u.created_at)}</span></div>
				</div>
				{@render deleteBtn(u)}
			</div>
		{/snippet}
	</DataTable>
{/if}
