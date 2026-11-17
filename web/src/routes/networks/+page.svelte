<script>
	import { get, post, del } from '$lib/api.js';
	import { shortId } from '$lib/format.js';
	import { attempt } from '$lib/session.svelte.js';

	let networks = $state([]);
	let name = $state('');
	let driver = $state('bridge');
	const builtin = ['bridge', 'host', 'none'];

	const load = () => attempt(async () => (networks = (await get('/networks')).networks));
	$effect(() => {
		load();
	});

	async function create(e) {
		e.preventDefault();
		await attempt(() => post('/networks', { name, driver }), `Created ${name}`);
		name = '';
		load();
	}

	async function remove(n) {
		if (!confirm(`Remove network ${n.Name}?`)) return;
		await attempt(() => del(`/networks/${n.Id}`), `Removed ${n.Name}`);
		load();
	}
</script>

<h1>Networks</h1>

<form class="row panel" onsubmit={create} style="margin-bottom:1rem">
	<input bind:value={name} placeholder="network name" required style="flex:1" />
	<select bind:value={driver} style="width:auto"><option>bridge</option><option>overlay</option><option>macvlan</option></select>
	<button class="primary">Create</button>
</form>

<table>
	<thead><tr><th>Name</th><th>ID</th><th>Driver</th><th>Scope</th><th>Subnet</th><th></th></tr></thead>
	<tbody>
		{#each networks as n (n.Id)}
			<tr>
				<td>{n.Name}</td>
				<td><code>{shortId(n.Id)}</code></td>
				<td>{n.Driver}</td>
				<td>{n.Scope}</td>
				<td class="small">{(n.IPAM?.Config || []).map((c) => c.Subnet).join(', ')}</td>
				<td style="text-align:right">
					{#if !builtin.includes(n.Name)}<button class="danger" onclick={() => remove(n)}>Remove</button>{/if}
				</td>
			</tr>
		{/each}
	</tbody>
</table>
