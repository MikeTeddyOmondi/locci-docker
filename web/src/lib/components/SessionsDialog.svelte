<script>
	// The signed-in user's sessions (API tokens): where they signed in, when the
	// token was last used and when it expires, with a button to sign one out.
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import MonitorSmartphoneIcon from '@lucide/svelte/icons/monitor-smartphone';
	import { get, del } from '$lib/api.js';
	import { attempt, confirm } from '$lib/session.svelte.js';

	let { open = $bindable(false) } = $props();

	let tokens = $state(null);

	async function load() {
		tokens = null;
		tokens = (await attempt(() => get('/auth/tokens')))?.tokens ?? [];
	}

	$effect(() => {
		if (open) load();
	});

	const parse = (v) => new Date(v.includes('T') ? v : v.replace(' ', 'T') + 'Z');
	const when = (v) => (v ? parse(v).toLocaleString() : 'never');
	const day = (v) => (v ? parse(v).toLocaleDateString() : 'never');

	// A browser's User-Agent is long; show the part that says what it is.
	function label(name) {
		if (!name) return 'Unnamed';
		if (name.startsWith('sso-device:')) return 'CLI, single sign-on (' + name.slice(11) + ')';
		if (name.startsWith('sso:')) return 'Single sign-on (' + name.slice(4) + ')';
		if (name.startsWith('locci/') || name.startsWith('Go-http-client')) return 'CLI';
		const browser = /(Edg|Firefox|Chrome|Safari)\/[\d.]+/.exec(name)?.[1];
		const os = /(Mac OS X|Windows|Linux|Android|iPhone|iPad)/.exec(name)?.[1];
		return browser ? `${browser === 'Edg' ? 'Edge' : browser}${os ? ' on ' + os.replace('Mac OS X', 'macOS') : ''}` : name.slice(0, 40);
	}

	async function revoke(t) {
		const ok = await confirm({
			title: 'Sign out this session?',
			description: `${label(t.name)}, last used ${when(t.last_used_at || t.created_at)}. It will have to sign in again.`,
			action: 'Sign out'
		});
		if (ok && (await attempt(() => del('/auth/tokens/' + t.id), 'Session signed out'))) load();
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
		<div class="ribbed absolute inset-x-0 top-0 h-1.5 text-primary" aria-hidden="true"></div>
		<Dialog.Header>
			<div class="kicker">Account</div>
			<Dialog.Title class="font-heading text-xl">Sessions</Dialog.Title>
			<Dialog.Description>
				Every browser and CLI signed in as you. Using a session pushes its expiry date back; an
				unused one ends on that date.
			</Dialog.Description>
		</Dialog.Header>

		<ul class="grid gap-2">
			{#if tokens === null}
				{#each [1, 2] as i (i)}<Skeleton class="h-16 w-full" />{/each}
			{:else if tokens.length === 0}
				<li class="text-sm text-muted-foreground">
					No stored sessions (signed in with the server's static token).
				</li>
			{:else}
				{#each tokens as t (t.id)}
					<li class="flex items-center gap-3 rounded-md border p-3">
						<MonitorSmartphoneIcon class="size-5 shrink-0 text-muted-foreground" />
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2 text-sm font-medium">
								<span class="truncate" title={t.name}>{label(t.name)}</span>
								{#if t.current}<Badge variant="secondary">This browser</Badge>{/if}
							</div>
							<div class="text-xs text-muted-foreground">
								Last used {when(t.last_used_at || t.created_at)} · expires {day(t.expires_at)}
							</div>
						</div>
						{#if !t.current}
							<Button variant="ghost" size="icon" aria-label="Sign out this session" onclick={() => revoke(t)}>
								<LogOutIcon />
							</Button>
						{/if}
					</li>
				{/each}
			{/if}
		</ul>
	</Dialog.Content>
</Dialog.Root>
