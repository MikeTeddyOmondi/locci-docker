<script>
	import { get } from '$lib/api.js';
	import { bytes } from '$lib/format.js';
	import { attempt } from '$lib/session.svelte.js';

	let info = $state(null);
	let counts = $state({});

	$effect(() => {
		attempt(async () => {
			const [i, c, im, n, v] = await Promise.all([
				get('/system/info'),
				get('/containers'),
				get('/images'),
				get('/networks'),
				get('/volumes')
			]);
			info = i;
			const running = c.containers.filter((x) => x.State === 'running').length;
			counts = {
				containers: c.containers.length,
				running,
				stopped: c.containers.length - running,
				images: im.images.length,
				networks: n.networks.length,
				volumes: (v.volumes || []).length
			};
		});
	});
</script>

<h1>Dashboard</h1>

<div class="grid">
	<a class="panel" href="/containers"><div class="muted">Containers</div><div class="stat">{counts.containers ?? '–'}</div>
		<div class="small muted">{counts.running ?? 0} running · {counts.stopped ?? 0} stopped</div></a>
	<a class="panel" href="/images"><div class="muted">Images</div><div class="stat">{counts.images ?? '–'}</div></a>
	<a class="panel" href="/networks"><div class="muted">Networks</div><div class="stat">{counts.networks ?? '–'}</div></a>
	<a class="panel" href="/volumes"><div class="muted">Volumes</div><div class="stat">{counts.volumes ?? '–'}</div></a>
</div>

{#if info}
	<h2>Host</h2>
	<table>
		<tbody>
			<tr><th>Name</th><td>{info.Name}</td></tr>
			<tr><th>Docker</th><td>{info.ServerVersion}</td></tr>
			<tr><th>OS</th><td>{info.OperatingSystem} ({info.Architecture})</td></tr>
			<tr><th>Kernel</th><td>{info.KernelVersion}</td></tr>
			<tr><th>CPUs / Memory</th><td>{info.NCPU} / {bytes(info.MemTotal)}</td></tr>
			<tr><th>Storage driver</th><td>{info.Driver}</td></tr>
			<tr><th>Docker root</th><td><code>{info.DockerRootDir}</code></td></tr>
		</tbody>
	</table>
{/if}
