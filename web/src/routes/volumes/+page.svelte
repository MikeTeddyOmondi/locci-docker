<script>
	import { get, post, del } from '$lib/api.js';
	import { ago } from '$lib/format.js';
	import { attempt } from '$lib/session.svelte.js';

	let volumes = $state([]);
	let name = $state('');

	const load = () => attempt(async () => (volumes = (await get('/volumes')).volumes || []));
	$effect(() => {
		load();
	});

	async function create(e) {
		e.preventDefault();
		await attempt(() => post('/volumes', { name }), `Created ${name}`);
		name = '';
		load();
	}

	async function remove(v) {
		if (!confirm(`Remove volume ${v.Name}? Its data will be lost.`)) return;
		await attempt(() => del(`/volumes/${encodeURIComponent(v.Name)}`), `Removed ${v.Name}`);
		load();
	}
</script>

<h1>Volumes</h1>

<form class="row panel" onsubmit={create} style="margin-bottom:1rem">
	<input bind:value={name} placeholder="volume name" required style="flex:1" />
	<button class="primary">Create</button>
</form>

<table>
	<thead><tr><th>Name</th><th>Driver</th><th>Mountpoint</th><th>Created</th><th></th></tr></thead>
	<tbody>
		{#each volumes as v (v.Name)}
			<tr>
				<td>{v.Name}</td>
				<td>{v.Driver}</td>
				<td><code class="small">{v.Mountpoint}</code></td>
				<td class="muted">{v.CreatedAt ? ago(v.CreatedAt) : ''}</td>
				<td style="text-align:right"><button class="danger" onclick={() => remove(v)}>Remove</button></td>
			</tr>
		{:else}
			<tr><td colspan="5" class="muted">No volumes.</td></tr>
		{/each}
	</tbody>
</table>
