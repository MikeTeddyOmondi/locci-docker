<script>
	import '../app.css';
	import { goto, afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { ModeWatcher } from 'mode-watcher';
	import { Toaster } from '$lib/components/ui/sonner/index.js';
	import * as Sheet from '$lib/components/ui/sheet/index.js';
	import * as Tooltip from '$lib/components/ui/tooltip/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import MenuIcon from '@lucide/svelte/icons/menu';
	import Nav from '$lib/components/Nav.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';
	import ConfirmHost from '$lib/components/ConfirmHost.svelte';
	import { api, auth } from '$lib/api.js';
	import { session } from '$lib/session.svelte.js';
	import { envs, loadEnvs } from '$lib/env.svelte.js';

	let { children } = $props();
	let menuOpen = $state(false);

	const isLogin = $derived(page.url.pathname === '/login');

	// Load the current user once a token exists; bounce to /login otherwise.
	$effect(() => {
		if (isLogin) return;
		if (!auth.token) {
			goto('/login');
			return;
		}
		if (!session.user)
			api('GET', '/auth/me')
				.then((u) => {
					session.user = u;
				})
				.catch(() => {});
	});

	// Environments (and their up/down dots) refresh on the server's 30s health cadence.
	$effect(() => {
		if (!session.user) return;
		loadEnvs().catch(() => {});
		const t = setInterval(() => loadEnvs().catch(() => {}), 30_000);
		return () => clearInterval(t);
	});

	afterNavigate(() => (menuOpen = false));

	async function logout() {
		await api('POST', '/auth/logout').catch(() => {});
		auth.token = null;
		session.user = null;
		envs.list = [];
		envs.loaded = false;
		goto('/login');
	}
</script>

<ModeWatcher />
<Toaster position="bottom-right" richColors closeButton />
<ConfirmHost />

<Tooltip.Provider delayDuration={250}>
	{#if isLogin}
		{@render children()}
	{:else}
		<div class="min-h-dvh lg:grid lg:grid-cols-[256px_1fr]">
			<aside
				class="sticky top-0 hidden h-dvh border-r bg-sidebar/85 backdrop-blur-sm lg:block"
			>
				{#if session.user}
					<Nav onlogout={logout} />
				{:else}
					<div class="flex flex-col gap-3 p-5">
						<Skeleton class="h-8 w-32" />
						{#each Array(6) as _}<Skeleton class="mt-2 h-8 w-full" />{/each}
					</div>
				{/if}
			</aside>

			<header
				class="sticky top-0 z-30 flex h-14 items-center justify-between gap-2 border-b bg-background/80 px-3 backdrop-blur-md lg:hidden"
			>
				<Sheet.Root bind:open={menuOpen}>
					<Sheet.Trigger>
						{#snippet child({ props })}
							<Button {...props} variant="ghost" size="icon" aria-label="Open menu">
								<MenuIcon />
							</Button>
						{/snippet}
					</Sheet.Trigger>
					<Sheet.Content side="left" class="w-72 bg-sidebar p-0" showCloseButton={false}>
						<Sheet.Title class="sr-only">Navigation</Sheet.Title>
						{#if session.user}<Nav onlogout={logout} />{/if}
					</Sheet.Content>
				</Sheet.Root>
				<a href="/" class="flex items-center gap-2">
					<Logo class="size-7" />
					<span class="font-heading text-lg font-bold">locci</span>
				</a>
				<ThemeToggle />
			</header>

			<main class="mx-auto w-full max-w-7xl min-w-0 px-4 py-6 sm:px-6 lg:px-10 lg:py-10">
				{#if session.user}
					{@render children()}
				{:else}
					<div class="space-y-6">
						<Skeleton class="h-9 w-56" />
						<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
							{#each Array(4) as _}<Skeleton class="h-28 rounded-xl" />{/each}
						</div>
						<Skeleton class="h-64 rounded-xl" />
					</div>
				{/if}
			</main>
		</div>
	{/if}
</Tooltip.Provider>
