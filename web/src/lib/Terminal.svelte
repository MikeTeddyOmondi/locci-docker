<script>
	// Interactive shell in a container over the exec WebSocket.
	import { onMount } from 'svelte';
	import '@xterm/xterm/css/xterm.css';
	import { auth } from '$lib/api.js';

	let { id, cmd = 'sh' } = $props();
	let el;
	let status = $state('connecting…');

	onMount(() => {
		let term, ws, fit, observer;

		(async () => {
			const { Terminal } = await import('@xterm/xterm');
			const { FitAddon } = await import('@xterm/addon-fit');

			term = new Terminal({ cursorBlink: true, fontSize: 13, theme: { background: '#0b0f17' } });
			fit = new FitAddon();
			term.loadAddon(fit);
			term.open(el);
			fit.fit();

			const proto = location.protocol === 'https:' ? 'wss' : 'ws';
			const q = new URLSearchParams({ cmd, token: auth.token });
			ws = new WebSocket(`${proto}://${location.host}/api/v1/containers/${id}/exec/ws?${q}`);
			ws.binaryType = 'arraybuffer';

			const resize = () => {
				fit.fit();
				if (ws.readyState === WebSocket.OPEN)
					ws.send(JSON.stringify({ resize: [term.cols, term.rows] }));
			};

			ws.onopen = () => {
				status = 'connected';
				resize();
				term.focus();
			};
			ws.onmessage = (e) => term.write(new Uint8Array(e.data));
			ws.onclose = () => (status = 'disconnected');
			ws.onerror = () => (status = 'connection failed');
			term.onData((d) => ws.readyState === WebSocket.OPEN && ws.send(d));

			observer = new ResizeObserver(resize);
			observer.observe(el);
		})();

		return () => {
			observer?.disconnect();
			ws?.close();
			term?.dispose();
		};
	});
</script>

<div class="muted small">{status}</div>
<div class="term" bind:this={el}></div>

<style>
	.term {
		height: 420px;
		background: #0b0f17;
		border-radius: 8px;
		padding: 6px;
	}
</style>
