<script>
	import { goto } from '$app/navigation';
	import { get, post } from '$lib/api.js';
	import { parsePairs, lines } from '$lib/format.js';
	import { attempt } from '$lib/session.svelte.js';

	let f = $state({
		name: '',
		image: '',
		subdomain: '',
		ports: '',
		env: '',
		volumes: '',
		labels: '',
		command: '',
		networks: [],
		start: true
	});
	let networks = $state([]);
	let busy = $state(false);

	$effect(() => {
		attempt(async () => (networks = (await get('/networks')).networks.map((n) => n.Name)));
	});

	async function submit(e) {
		e.preventDefault();
		busy = true;
		// "80:8080" lines -> { "80": "8080" } (container port -> host port)
		const ports = {};
		for (const l of lines(f.ports)) {
			const [c, h = ''] = l.split(':');
			ports[c] = h;
		}
		const spec = {
			name: f.name,
			image: f.image,
			subdomain: f.subdomain || f.name,
			ports,
			environment: lines(f.env),
			volumes: lines(f.volumes),
			labels: parsePairs(f.labels),
			networks: f.networks,
			command: f.command.trim() ? f.command.trim().split(/\s+/) : []
		};
		const created = await attempt(async () => {
			const c = await post('/containers', spec);
			if (f.start) await post(`/containers/${c.container_id}/start`);
			return c;
		}, `Created ${f.name}`);
		busy = false;
		if (created) goto(`/containers/${created.container_id}`);
	}
</script>

<h1>New container</h1>

<form class="panel form" onsubmit={submit}>
	<label><span>Name *</span><input bind:value={f.name} required pattern="[a-zA-Z0-9][a-zA-Z0-9_.-]*" /></label>
	<label><span>Image *</span><input bind:value={f.image} required placeholder="nginx:alpine" /></label>
	<label><span>Subdomain</span><input bind:value={f.subdomain} placeholder="defaults to the name" /></label>
	<label><span>Ports (CONTAINER:HOST per line, HOST empty or 0 = random)</span><textarea bind:value={f.ports} placeholder="80:8080"></textarea></label>
	<label><span>Environment (KEY=VALUE per line)</span><textarea bind:value={f.env}></textarea></label>
	<label><span>Volumes (name-or-path:/container/path per line)</span><textarea bind:value={f.volumes} placeholder="data:/var/lib/data"></textarea></label>
	<label><span>Labels (KEY=VALUE per line)</span><textarea bind:value={f.labels}></textarea></label>
	<label><span>Command (optional)</span><input bind:value={f.command} placeholder="overrides the image CMD" /></label>
	<label><span>Networks</span>
		<select multiple bind:value={f.networks} size={Math.min(6, networks.length || 1)}>
			{#each networks as n}<option value={n}>{n}</option>{/each}
		</select>
	</label>
	<label class="row" style="font-weight:400"><input type="checkbox" bind:checked={f.start} style="width:auto" /> Start after creating</label>
	<div class="row">
		<button class="primary" disabled={busy}>{busy ? 'Creating… (pulling image)' : 'Create'}</button>
		<a href="/containers">Cancel</a>
	</div>
</form>
