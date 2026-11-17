<script>
	import { get } from '$lib/api.js';
	import { attempt } from '$lib/session.svelte.js';

	let entries = $state([]);
	const load = () => attempt(async () => (entries = (await get('/audit?limit=500')).audit));
	$effect(() => {
		load();
	});
</script>

<div class="row spread">
	<h1>Audit log</h1>
	<button onclick={load}>Refresh</button>
</div>

<table>
	<thead><tr><th>Time (UTC)</th><th>User</th><th>Method</th><th>Path</th><th>Status</th></tr></thead>
	<tbody>
		{#each entries as a (a.id)}
			<tr>
				<td class="small">{a.at}</td>
				<td>{a.username}</td>
				<td><code>{a.method}</code></td>
				<td><code>{a.path}</code></td>
				<td><span class="badge" class:running={a.status < 400} class:exited={a.status >= 400}>{a.status}</span></td>
			</tr>
		{:else}
			<tr><td colspan="5" class="muted">Nothing yet.</td></tr>
		{/each}
	</tbody>
</table>
