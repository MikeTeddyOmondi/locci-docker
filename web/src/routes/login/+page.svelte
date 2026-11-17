<script>
	import { goto } from '$app/navigation';
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
</script>

<div class="wrap">
	<form class="panel" onsubmit={login}>
		<h1>Sign in to locci</h1>
		<label><span>Username</span><input bind:value={username} autocomplete="username" required /></label>
		<label><span>Password</span><input type="password" bind:value={password} autocomplete="current-password" required /></label>
		{#if error}<p style="color: var(--danger)">{error}</p>{/if}
		<button class="primary" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
		<p class="muted small">No account? Create one on the server with <code>locci user add &lt;name&gt; --admin</code>.</p>
	</form>
</div>

<style>
	.wrap { min-height: 100vh; display: grid; place-items: center; padding: 1rem; }
	form { width: 100%; max-width: 360px; }
</style>
