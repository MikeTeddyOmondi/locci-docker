<script>
	import { get, post, del } from '$lib/api.js';
	import { shortId, containerName } from '$lib/format.js';
	import { attempt } from '$lib/session.svelte.js';

	let containers = $state([]);
	let filter = $state('');
	let busy = $state('');

	const shown = $derived(
		containers.filter((c) =>
			(containerName(c) + c.Image + c.State).toLowerCase().includes(filter.toLowerCase())
		)
	);

	const load = () => attempt(async () => (containers = (await get('/containers')).containers));
	$effect(() => {
		load();
	});

	async function act(c, action) {
		busy = c.Id + action;
		if (action === 'remove') {
			if (!confirm(`Remove ${containerName(c)}?`)) return (busy = '');
			await attempt(() => del(`/containers/${c.Id}?force=true`), `Removed ${containerName(c)}`);
		} else {
			await attempt(() => post(`/containers/${c.Id}/${action}`), `${action}: ${containerName(c)}`);
		}
		busy = '';
		load();
	}
</script>

<div class="row spread">
	<h1>Containers</h1>
	<div class="row">
		<input placeholder="Filter…" bind:value={filter} style="width: 220px" />
		<button onclick={load}>Refresh</button>
		<a class="btn primary" href="/containers/new" style="color:#fff;background:var(--accent)">+ New container</a>
	</div>
</div>

<table>
	<thead><tr><th>Name</th><th>State</th><th>Image</th><th>Ports</th><th>Status</th><th></th></tr></thead>
	<tbody>
		{#each shown as c (c.Id)}
			<tr>
				<td><a href="/containers/{c.Id}">{containerName(c)}</a><div class="muted small">{shortId(c.Id)}</div></td>
				<td><span class="badge {c.State}">{c.State}</span></td>
				<td><code>{c.Image}</code></td>
				<td class="small">
					{#each c.Ports || [] as p}
						{#if p.PublicPort}<div>{p.PublicPort}→{p.PrivatePort}/{p.Type}</div>{/if}
					{/each}
				</td>
				<td class="muted small">{c.Status}</td>
				<td>
					<div class="row" style="justify-content:flex-end">
						{#if c.State === 'running'}
							<button disabled={!!busy} onclick={() => act(c, 'stop')}>Stop</button>
							<button disabled={!!busy} onclick={() => act(c, 'restart')}>Restart</button>
							<button disabled={!!busy} onclick={() => act(c, 'pause')}>Pause</button>
						{:else if c.State === 'paused'}
							<button disabled={!!busy} onclick={() => act(c, 'unpause')}>Resume</button>
						{:else}
							<button disabled={!!busy} onclick={() => act(c, 'start')}>Start</button>
						{/if}
						<button class="danger" disabled={!!busy} onclick={() => act(c, 'remove')}>Remove</button>
					</div>
				</td>
			</tr>
		{:else}
			<tr><td colspan="6" class="muted">No containers.</td></tr>
		{/each}
	</tbody>
</table>
