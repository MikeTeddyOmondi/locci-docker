<script>
	import { get, post, del } from '$lib/api.js';
	import { bytes, shortId, ago } from '$lib/format.js';
	import { attempt, notify } from '$lib/session.svelte.js';

	let images = $state([]);
	let ref = $state('');
	let pulling = $state(false);

	const load = () => attempt(async () => (images = (await get('/images')).images));
	$effect(() => {
		load();
	});

	async function pull(e) {
		e.preventDefault();
		pulling = true;
		await attempt(() => post('/images/pull', { image: ref }), `Pulled ${ref}`);
		pulling = false;
		ref = '';
		load();
	}

	async function remove(im) {
		if (!confirm(`Remove ${im.RepoTags?.[0] || shortId(im.Id)}?`)) return;
		await attempt(() => del(`/images/${encodeURIComponent(im.Id)}?force=true`), 'Image removed');
		load();
	}

	async function prune() {
		const r = await attempt(() => post('/images/prune'));
		if (r) notify(`Reclaimed ${bytes(r.SpaceReclaimed || 0)}`);
		load();
	}
</script>

<div class="row spread">
	<h1>Images</h1>
	<button onclick={prune}>Prune dangling</button>
</div>

<form class="row panel" onsubmit={pull} style="margin-bottom:1rem">
	<input bind:value={ref} placeholder="image to pull, e.g. nginx:alpine" required style="flex:1" />
	<button class="primary" disabled={pulling}>{pulling ? 'Pulling…' : 'Pull'}</button>
</form>

<table>
	<thead><tr><th>Tags</th><th>ID</th><th>Size</th><th>Created</th><th></th></tr></thead>
	<tbody>
		{#each images as im (im.Id)}
			<tr>
				<td>{#each im.RepoTags?.length ? im.RepoTags : ['<none>'] as t}<div><code>{t}</code></div>{/each}</td>
				<td><code>{shortId(im.Id)}</code></td>
				<td>{bytes(im.Size)}</td>
				<td class="muted">{ago(im.Created)}</td>
				<td style="text-align:right"><button class="danger" onclick={() => remove(im)}>Remove</button></td>
			</tr>
		{:else}
			<tr><td colspan="5" class="muted">No images.</td></tr>
		{/each}
	</tbody>
</table>
