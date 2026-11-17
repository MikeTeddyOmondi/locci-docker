<script>
	import '../app.css';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, auth } from '$lib/api.js';
	import { session, toast } from '$lib/session.svelte.js';

	let { children } = $props();

	const nav = [
		['/', 'Dashboard'],
		['/containers', 'Containers'],
		['/images', 'Images'],
		['/networks', 'Networks'],
		['/volumes', 'Volumes']
	];
	const adminNav = [
		['/users', 'Users'],
		['/audit', 'Audit log']
	];

	const isLogin = $derived(page.url.pathname === '/login');

	// Load the current user once a token exists; bounce to /login otherwise.
	$effect(() => {
		if (isLogin) return;
		if (!auth.token) {
			goto('/login');
			return;
		}
		if (!session.user) api('GET', '/auth/me').then((u) => (session.user = u)).catch(() => {});
	});

	async function logout() {
		await api('POST', '/auth/logout').catch(() => {});
		auth.token = null;
		session.user = null;
		goto('/login');
	}

	const active = (href) =>
		href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(href);
</script>

{#if isLogin}
	{@render children()}
{:else if session.user}
	<div class="shell">
		<aside>
			<a class="brand" href="/">locci</a>
			<nav>
				{#each nav as [href, label]}
					<a {href} class:active={active(href)}>{label}</a>
				{/each}
				{#if session.user.role === 'admin'}
					<div class="sep muted small">Admin</div>
					{#each adminNav as [href, label]}
						<a {href} class:active={active(href)}>{label}</a>
					{/each}
				{/if}
			</nav>
			<div class="me">
				<div>{session.user.username} <span class="muted small">({session.user.role})</span></div>
				<button onclick={logout}>Log out</button>
			</div>
		</aside>
		<main>{@render children()}</main>
	</div>
{/if}

{#if toast.text}
	<div class="toast" class:error={toast.error}>{toast.text}</div>
{/if}

<style>
	.shell { display: grid; grid-template-columns: 210px 1fr; min-height: 100vh; }
	aside { background: var(--panel); border-right: 1px solid var(--border); padding: 1rem; display: flex; flex-direction: column; gap: 1rem; position: sticky; top: 0; height: 100vh; }
	.brand { font-weight: 700; font-size: 1.25rem; color: var(--text); }
	nav { display: flex; flex-direction: column; gap: 2px; flex: 1; }
	nav a { color: var(--text); padding: .45rem .6rem; border-radius: 7px; }
	nav a:hover { background: var(--bg); text-decoration: none; }
	nav a.active { background: color-mix(in srgb, var(--accent) 14%, transparent); color: var(--accent); }
	.sep { margin: 1rem .6rem .25rem; text-transform: uppercase; letter-spacing: .05em; }
	.me { display: flex; flex-direction: column; gap: .5rem; }
	main { padding: 1.5rem 2rem; min-width: 0; }
	.toast { position: fixed; bottom: 1.25rem; right: 1.25rem; background: var(--text); color: var(--bg); padding: .65rem 1rem; border-radius: 8px; max-width: 420px; box-shadow: 0 6px 24px rgb(0 0 0 / .2); }
	.toast.error { background: var(--danger); color: #fff; }
	@media (max-width: 760px) {
		.shell { grid-template-columns: 1fr; }
		aside { position: static; height: auto; }
		nav { flex-direction: row; flex-wrap: wrap; }
		main { padding: 1rem; }
	}
</style>
