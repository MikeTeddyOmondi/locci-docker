<script>
	import { goto } from '$app/navigation';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import Loader2Icon from '@lucide/svelte/icons/loader-2';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import Logo from '$lib/components/Logo.svelte';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';
	import { api, auth } from '$lib/api.js';
	import { session } from '$lib/session.svelte.js';

	let username = $state('');
	let password = $state('');
	let error = $state('');
	let busy = $state(false);

	async function login(e) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			const res = await api('POST', '/auth/login', { username, password });
			auth.token = res.token;
			session.user = res.user;
			goto('/');
		} catch (err) {
			error = err.message;
		} finally {
			busy = false;
		}
	}

	// A stack of containers for the manifest panel: [col, row, tone].
	const stack = [
		[0, 3, 'bg-primary'], [1, 3, 'bg-white/20'], [2, 3, 'bg-white/10'], [3, 3, 'bg-primary/70'],
		[0, 2, 'bg-white/15'], [1, 2, 'bg-primary/80'], [2, 2, 'bg-white/25'],
		[1, 1, 'bg-white/10'], [2, 1, 'bg-primary']
	];
</script>

<svelte:head><title>Sign in · locci</title></svelte:head>

<div class="grid min-h-dvh lg:grid-cols-[1fr_1.1fr]">
	<div class="relative flex flex-col px-5 py-6 sm:px-10">
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2.5">
				<Logo class="size-8" />
				<span class="font-heading text-xl font-bold">locci</span>
			</div>
			<ThemeToggle />
		</div>

		<div class="flex flex-1 items-center justify-center py-10">
			<form class="rise w-full max-w-sm space-y-6" onsubmit={login}>
				<div>
					<div class="kicker mb-2 flex items-center gap-2">
						<span class="inline-block h-px w-5 bg-primary"></span>Gate 01
					</div>
					<h1 class="text-3xl font-semibold sm:text-4xl">Welcome aboard.</h1>
					<p class="mt-2 text-sm text-muted-foreground">
						Sign in to manage the containers on this host.
					</p>
				</div>

				<div class="space-y-4">
					<div class="space-y-2">
						<Label for="username">Username</Label>
						<Input id="username" bind:value={username} autocomplete="username" required class="h-11" />
					</div>
					<div class="space-y-2">
						<Label for="password">Password</Label>
						<Input
							id="password"
							type="password"
							bind:value={password}
							autocomplete="current-password"
							required
							class="h-11"
						/>
					</div>
				</div>

				{#if error}
					<div
						class="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2.5 text-sm text-destructive"
						role="alert"
					>
						<CircleAlertIcon class="mt-0.5 size-4 shrink-0" />
						{error}
					</div>
				{/if}

				<Button type="submit" class="group h-11 w-full text-base" disabled={busy}>
					{#if busy}
						<Loader2Icon class="animate-spin" /> Signing in
					{:else}
						Sign in
						<ArrowRightIcon class="transition-transform group-hover:translate-x-0.5" />
					{/if}
				</Button>

				<p class="text-xs leading-relaxed text-muted-foreground">
					No account? Create one on the server with
					<code class="rounded bg-muted px-1.5 py-0.5 text-foreground">locci user add &lt;name&gt; --admin</code>
				</p>
			</form>
		</div>
	</div>

	<aside
		class="relative hidden overflow-hidden bg-[oklch(0.19_0.03_255)] text-white lg:flex lg:flex-col lg:justify-between lg:p-12"
	>
		<div
			class="absolute inset-0 opacity-60"
			style="background-image: linear-gradient(oklch(1 0 0 / 5%) 1px, transparent 1px), linear-gradient(90deg, oklch(1 0 0 / 5%) 1px, transparent 1px); background-size: 28px 28px;"
		></div>
		<div class="relative flex items-center justify-between font-mono text-xs tracking-[0.2em] text-white/50 uppercase">
			<span>Manifest · locci/docker</span>
			<span class="flex items-center gap-2"><span class="signal signal-live size-1.5 text-[oklch(0.76_0.17_152)]"></span>Harbour open</span>
		</div>

		<div class="relative mx-auto grid w-full max-w-md grid-cols-4 grid-rows-3 gap-2">
			{#each stack as [col, row, tone], i}
				<div
					class="ribbed rise h-16 rounded-md text-black/30 {tone}"
					style="grid-column: {col + 1}; grid-row: {row}; animation-delay: {i * 70}ms"
				></div>
			{/each}
		</div>

		<div class="relative">
			<p class="font-heading text-4xl leading-tight font-semibold">
				Every container,<br /><span class="text-[oklch(0.72_0.18_48)]">accounted for.</span>
			</p>
			<p class="mt-3 max-w-sm text-sm text-white/60">
				Ship, inspect and steer the workloads on your Docker host from one quiet deck.
			</p>
		</div>
	</aside>
</div>
