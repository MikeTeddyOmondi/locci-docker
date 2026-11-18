<script>
	// Who may use an environment: every signed-in user, or only chosen users.
	// Admins can always use every environment, so only plain users are listed.
	import { Switch } from '$lib/components/ui/switch/index.js';
	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import UsersIcon from '@lucide/svelte/icons/users';
	import FormDialog from '$lib/components/FormDialog.svelte';
	import { get, api } from '$lib/api.js';
	import { attempt } from '$lib/session.svelte.js';

	/** env: the environment ({name, label, access}); onsaved runs after a save. */
	let { open = $bindable(false), env, onsaved } = $props();

	let users = $state(null); // plain users, or null while loading
	let restricted = $state(false);
	let chosen = $state([]);

	$effect(() => {
		if (!open || !env) return;
		restricted = env.access?.restricted ?? false;
		chosen = [...(env.access?.users ?? [])];
		users = null;
		attempt(() => get('/users')).then((r) => {
			users = (r?.users ?? []).filter((u) => u.role !== 'admin');
		});
	});

	function toggle(name, on) {
		chosen = on ? [...chosen, name] : chosen.filter((n) => n !== name);
	}

	async function save() {
		const ok = await attempt(
			() => api('PUT', `/environments/${encodeURIComponent(env.name)}/access`, { restricted, users: restricted ? chosen : [] }),
			restricted
				? chosen.length
					? `${env.label}: limited to ${chosen.length} user${chosen.length === 1 ? '' : 's'}`
					: `${env.label}: admins only`
				: `${env.label}: open to everyone`
		);
		if (ok) onsaved?.();
		return ok;
	}
</script>

<FormDialog
	bind:open
	kicker="Environment · {env?.name}"
	title="Who can use {env?.label}"
	description="Admins can always use every environment."
	submitLabel="Save access"
	icon={UsersIcon}
	onsubmit={save}
>
	<div class="flex items-start justify-between gap-4 rounded-md border p-3">
		<div class="grid gap-1">
			<Label for="env-restricted">Only chosen users</Label>
			<p class="text-xs text-muted-foreground">
				Off: everyone who can sign in sees and uses it. On: only the users ticked below; for everyone
				else it doesn't exist.
			</p>
		</div>
		<Switch id="env-restricted" bind:checked={restricted} />
	</div>

	{#if restricted}
		<div class="grid gap-2">
			{#if users === null}
				{#each [1, 2, 3] as i (i)}<Skeleton class="h-8 w-full" />{/each}
			{:else if users.length === 0}
				<p class="text-sm text-muted-foreground">No plain users yet: this environment will be admin-only.</p>
			{:else}
				<ul class="grid max-h-64 gap-1 overflow-y-auto">
					{#each users as u (u.username)}
						<li>
							<label class="flex cursor-pointer items-center gap-3 rounded-md px-2 py-1.5 hover:bg-muted/60">
								<Checkbox checked={chosen.includes(u.username)} onCheckedChange={(v) => toggle(u.username, v)} />
								<span class="text-sm">{u.username}</span>
							</label>
						</li>
					{/each}
				</ul>
				{#if chosen.length === 0}
					<p class="text-xs text-muted-foreground">Nobody ticked: admins only.</p>
				{/if}
			{/if}
		</div>
	{/if}
</FormDialog>
