<script>
	// Sidebar contents, shared by the desktop rail and the mobile sheet.
	import { page } from '$app/state';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import * as Avatar from '$lib/components/ui/avatar/index.js';
	import Logo from '$lib/components/Logo.svelte';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';
	import GaugeIcon from '@lucide/svelte/icons/gauge';
	import ContainerIcon from '@lucide/svelte/icons/container';
	import LayersIcon from '@lucide/svelte/icons/layers';
	import NetworkIcon from '@lucide/svelte/icons/network';
	import HardDriveIcon from '@lucide/svelte/icons/hard-drive';
	import UsersIcon from '@lucide/svelte/icons/users';
	import ScrollTextIcon from '@lucide/svelte/icons/scroll-text';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import ChevronsUpDownIcon from '@lucide/svelte/icons/chevrons-up-down';
	import ServerIcon from '@lucide/svelte/icons/server';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import EnvSwitcher from '$lib/components/EnvSwitcher.svelte';
	import { session } from '$lib/session.svelte.js';
	import { href as envHref } from '$lib/env.svelte.js';

	let { onlogout } = $props();

	// Fleet links stay inside the current environment; envHref reads
	// envs.current, so they update when the environment changes.
	const groups = $derived([
		{
			label: 'Fleet',
			items: [
				[envHref('/'), 'Overview', GaugeIcon],
				[envHref('/containers'), 'Containers', ContainerIcon],
				[envHref('/images'), 'Images', LayersIcon],
				[envHref('/networks'), 'Networks', NetworkIcon],
				[envHref('/volumes'), 'Volumes', HardDriveIcon],
				['/environments', 'Environments', ServerIcon]
			]
		},
		...(session.user?.role === 'admin'
			? [
					{
						label: 'Harbour master',
						items: [
							['/users', 'Users', UsersIcon],
							['/sign-in', 'Sign-in', KeyRoundIcon],
							['/audit', 'Audit log', ScrollTextIcon]
						]
					}
				]
			: [])
	]);

	// The overview (/e/<env>) is active only on itself; sections match their subtree.
	const active = (href) =>
		href === envHref('/') ? page.url.pathname === href : page.url.pathname.startsWith(href);

	const initials = $derived((session.user?.username ?? '?').slice(0, 2).toUpperCase());
</script>

<div class="flex h-full flex-col">
	<div class="flex items-center justify-between gap-2 px-4 pt-5 pb-6">
		<a href={envHref('/')} class="flex items-center gap-2.5">
			<Logo class="size-8" />
			<span class="font-heading text-xl font-bold tracking-tight">locci</span>
			<span class="kicker rounded border px-1.5 py-0.5 text-[0.6rem]">docker</span>
		</a>
		<ThemeToggle />
	</div>

	<div class="px-3 pb-5"><EnvSwitcher /></div>

	<nav class="flex-1 space-y-6 overflow-y-auto px-3">
		{#each groups as group}
			<div>
				<div class="kicker px-3 pb-2">{group.label}</div>
				<ul class="space-y-0.5">
					{#each group.items as [href, label, Icon]}
						{@const on = active(href)}
						<li>
							<a
								{href}
								aria-current={on ? 'page' : undefined}
								class="group relative flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors
									{on
									? 'bg-sidebar-accent text-sidebar-accent-foreground'
									: 'text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground'}"
							>
								<span
									class="absolute top-1.5 bottom-1.5 left-0 w-[3px] rounded-full bg-primary transition-opacity {on
										? 'opacity-100'
										: 'opacity-0'}"
								></span>
								<Icon class="size-4 {on ? 'text-primary' : ''}" />
								{label}
							</a>
						</li>
					{/each}
				</ul>
			</div>
		{/each}
	</nav>

	<div class="border-t p-3">
		<DropdownMenu.Root>
			<DropdownMenu.Trigger
				class="flex w-full items-center gap-3 rounded-lg p-2 text-left transition-colors hover:bg-sidebar-accent"
			>
				<Avatar.Root class="size-9 rounded-lg">
					<Avatar.Fallback class="ribbed rounded-lg bg-primary/15 font-mono text-xs font-semibold text-primary">
						{initials}
					</Avatar.Fallback>
				</Avatar.Root>
				<div class="min-w-0 flex-1">
					<div class="truncate text-sm font-medium">{session.user?.username}</div>
					<div class="kicker text-[0.6rem]">{session.user?.role}</div>
				</div>
				<ChevronsUpDownIcon class="size-4 text-muted-foreground" />
			</DropdownMenu.Trigger>
			<DropdownMenu.Content side="top" align="start" class="w-(--bits-dropdown-menu-anchor-width) min-w-52">
				<DropdownMenu.Label class="font-normal">
					<div class="text-sm font-medium">{session.user?.username}</div>
					<div class="text-xs text-muted-foreground">Signed in as {session.user?.role}</div>
				</DropdownMenu.Label>
				<DropdownMenu.Separator />
				<DropdownMenu.Item variant="destructive" onclick={onlogout}>
					<LogOutIcon />
					Log out
				</DropdownMenu.Item>
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	</div>
</div>
