<script>
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { get, post, del, stream } from '$lib/api.js';
	import { bytes, shortId } from '$lib/format.js';
	import { attempt } from '$lib/session.svelte.js';
	import Terminal from '$lib/Terminal.svelte';

	const id = $derived(page.params.id);

	let info = $state(null);
	let tab = $state('overview');
	let stats = $state(null);
	let logLines = $state([]);
	let follow = $state(true);
	let shell = $state('sh');
	let termKey = $state(0);
	let logBox;

	const load = () => attempt(async () => (info = await get(`/containers/${id}`)));
	$effect(() => {
		load();
	});

	// Stats: poll while the tab is open and the container runs.
	$effect(() => {
		if (tab !== 'stats' || !info?.State?.Running) return;
		const tick = () => get(`/containers/${id}/stats`).then((s) => (stats = s)).catch(() => {});
		tick();
		const t = setInterval(tick, 3000);
		return () => clearInterval(t);
	});

	// Logs: stream while the tab is open; restart when "follow" changes.
	$effect(() => {
		if (tab !== 'logs') return;
		const ctrl = new AbortController();
		logLines = [];
		stream(`/containers/${id}/logs?tail=500&follow=${follow}`, (line) => {
			logLines.push(line);
			if (logLines.length > 3000) logLines.splice(0, logLines.length - 3000);
			queueMicrotask(() => logBox && (logBox.scrollTop = logBox.scrollHeight));
		}, ctrl.signal).catch((e) => e.name !== 'AbortError' && logLines.push('[error] ' + e.message));
		return () => ctrl.abort();
	});

	async function act(action) {
		if (action === 'remove') {
			if (!confirm('Remove this container?')) return;
			if (await attempt(() => del(`/containers/${id}?force=true`), 'Removed')) goto('/containers');
			return;
		}
		await attempt(() => post(`/containers/${id}/${action}`), action + ' ok');
		load();
	}

	const state = $derived(info?.State?.Status);
	const name = $derived(info?.Name?.replace(/^\//, ''));
</script>

{#if info}
	<div class="row spread">
		<h1>{name} <span class="badge {state}">{state}</span></h1>
		<div class="row">
			{#if info.State.Running && !info.State.Paused}
				<button onclick={() => act('stop')}>Stop</button>
				<button onclick={() => act('restart')}>Restart</button>
				<button onclick={() => act('pause')}>Pause</button>
				<button onclick={() => act('kill')}>Kill</button>
			{:else if info.State.Paused}
				<button onclick={() => act('unpause')}>Resume</button>
			{:else}
				<button onclick={() => act('start')}>Start</button>
			{/if}
			<button class="danger" onclick={() => act('remove')}>Remove</button>
		</div>
	</div>

	<div class="tabs">
		{#each ['overview', 'logs', 'stats', 'console', 'inspect'] as t}
			<button class:active={tab === t} onclick={() => (tab = t)}>{t}</button>
		{/each}
	</div>

	{#if tab === 'overview'}
		<table>
			<tbody>
				<tr><th>ID</th><td><code>{shortId(info.Id)}</code></td></tr>
				<tr><th>Image</th><td><code>{info.Config.Image}</code></td></tr>
				<tr><th>Created</th><td>{new Date(info.Created).toLocaleString()}</td></tr>
				<tr><th>Started</th><td>{info.State.StartedAt?.startsWith('0001') ? '–' : new Date(info.State.StartedAt).toLocaleString()}</td></tr>
				<tr><th>Command</th><td><code>{[info.Path, ...(info.Args || [])].join(' ')}</code></td></tr>
				<tr><th>Restart policy</th><td>{info.HostConfig.RestartPolicy?.Name || 'no'}</td></tr>
				<tr><th>Ports</th><td>
					{#each Object.entries(info.NetworkSettings.Ports || {}) as [port, binds]}
						<div><code>{port}</code> → {(binds || []).map((b) => `${b.HostIp || '0.0.0.0'}:${b.HostPort}`).join(', ') || 'not published'}</div>
					{/each}
				</td></tr>
				<tr><th>Networks</th><td>
					{#each Object.entries(info.NetworkSettings.Networks || {}) as [net, n]}
						<div>{net} <span class="muted">{n.IPAddress}</span></div>
					{/each}
				</td></tr>
				<tr><th>Mounts</th><td>
					{#each info.Mounts || [] as m}
						<div><code>{m.Name || m.Source}</code> → <code>{m.Destination}</code> <span class="muted">{m.RW ? 'rw' : 'ro'}</span></div>
					{:else}<span class="muted">none</span>{/each}
				</td></tr>
				<tr><th>Environment</th><td>{#each info.Config.Env || [] as e}<div><code>{e}</code></div>{/each}</td></tr>
				<tr><th>Labels</th><td>{#each Object.entries(info.Config.Labels || {}) as [k, v]}<div><code>{k}={v}</code></div>{/each}</td></tr>
			</tbody>
		</table>
	{:else if tab === 'logs'}
		<div class="row" style="margin-bottom:.5rem">
			<label class="row" style="margin:0;font-weight:400"><input type="checkbox" bind:checked={follow} style="width:auto" /> Follow</label>
			<button onclick={() => (logLines = [])}>Clear</button>
		</div>
		<pre class="logs" bind:this={logBox}>{logLines.join('\n')}</pre>
	{:else if tab === 'stats'}
		{#if !info.State.Running}
			<p class="muted">Container is not running.</p>
		{:else if stats}
			<div class="grid">
				<div class="panel"><div class="muted">CPU</div><div class="stat">{stats.cpu_percent.toFixed(1)}%</div></div>
				<div class="panel"><div class="muted">Memory</div><div class="stat">{bytes(stats.mem_usage)}</div>
					<div class="small muted">{stats.mem_percent.toFixed(1)}% of {bytes(stats.mem_limit)}</div></div>
				<div class="panel"><div class="muted">Network</div><div class="stat small">↓ {bytes(stats.net_rx)} · ↑ {bytes(stats.net_tx)}</div></div>
				<div class="panel"><div class="muted">Processes</div><div class="stat">{stats.pids}</div></div>
			</div>
			<p class="muted small">Refreshes every 3 seconds.</p>
		{:else}
			<p class="muted">Loading…</p>
		{/if}
	{:else if tab === 'console'}
		{#if info.State.Running}
			<div class="row" style="margin-bottom:.5rem">
				<select bind:value={shell} style="width:auto">
					<option>sh</option><option>bash</option><option>ash</option>
				</select>
				<button onclick={() => termKey++}>Reconnect</button>
			</div>
			{#key termKey + shell}
				<Terminal {id} cmd={shell} />
			{/key}
		{:else}
			<p class="muted">Start the container to open a console.</p>
		{/if}
	{:else if tab === 'inspect'}
		<pre class="logs" style="color:inherit">{JSON.stringify(info, null, 2)}</pre>
	{/if}
{/if}
