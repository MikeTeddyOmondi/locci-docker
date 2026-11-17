<script>
	import { get, post, del } from '$lib/api.js';
	import { attempt, session } from '$lib/session.svelte.js';

	let users = $state([]);
	let f = $state({ username: '', password: '', role: 'user' });

	const load = () => attempt(async () => (users = (await get('/users')).users));
	$effect(() => {
		load();
	});

	async function create(e) {
		e.preventDefault();
		await attempt(() => post('/users', f), `Created ${f.username}`);
		f = { username: '', password: '', role: 'user' };
		load();
	}

	async function remove(u) {
		if (!confirm(`Delete user ${u.username}?`)) return;
		await attempt(() => del(`/users/${encodeURIComponent(u.username)}`), `Deleted ${u.username}`);
		load();
	}
</script>

<h1>Users</h1>

<form class="row panel" onsubmit={create} style="margin-bottom:1rem">
	<input bind:value={f.username} placeholder="username" required style="flex:1" />
	<input type="password" bind:value={f.password} placeholder="password (8+ chars)" minlength="8" required style="flex:1" />
	<select bind:value={f.role} style="width:auto"><option>user</option><option>admin</option></select>
	<button class="primary">Add user</button>
</form>

<table>
	<thead><tr><th>Username</th><th>Role</th><th>Created</th><th></th></tr></thead>
	<tbody>
		{#each users as u (u.id)}
			<tr>
				<td>{u.username}</td>
				<td>{u.role}</td>
				<td class="muted">{u.created_at}</td>
				<td style="text-align:right">
					{#if u.username !== session.user?.username}<button class="danger" onclick={() => remove(u)}>Delete</button>{/if}
				</td>
			</tr>
		{/each}
	</tbody>
</table>
