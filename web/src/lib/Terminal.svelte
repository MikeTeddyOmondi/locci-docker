<script>
	// Interactive shell in a container over the exec WebSocket.
	import { onMount } from 'svelte';
	import '@xterm/xterm/css/xterm.css';
	import { auth } from '$lib/api.js';
	import { scope } from '$lib/env.svelte.js';

	let { id, cmd = 'sh' } = $props();
	let el;
	let status = $state('connecting');

	const BG = '#111827';

	onMount(() => {
		let term, ws, fit, observer;

		(async () => {
			const { Terminal } = await import('@xterm/xterm');
			const { FitAddon } = await import('@xterm/addon-fit');

			term = new Terminal({
				cursorBlink: true,
				fontSize: 13,
				fontFamily: "'JetBrains Mono Variable', ui-monospace, monospace",
				theme: { background: BG, cursor: '#f97316', selectionBackground: '#f9731655' }
			});
			fit = new FitAddon();
			term.loadAddon(fit);
			term.open(el);
			fit.fit();

			const proto = location.protocol === 'https:' ? 'wss' : 'ws';
			const q = new URLSearchParams({ cmd, token: auth.token });
			ws = new WebSocket(`${proto}://${location.host}/api/v1${scope(`/containers/${id}/exec/ws`)}?${q}`);
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

	const tone = $derived(
		status === 'connected' ? 'text-ok signal-live' : status === 'connecting' ? 'text-warn signal-live' : 'text-destructive'
	);
</script>

<div class="overflow-hidden rounded-xl border" style="background: {BG}">
	<div class="flex items-center gap-2 border-b border-white/10 px-3 py-2 font-mono text-xs text-white/60">
		<span class="flex gap-1.5" aria-hidden="true">
			<span class="size-2.5 rounded-full bg-white/15"></span><span class="size-2.5 rounded-full bg-white/15"></span><span class="size-2.5 rounded-full bg-white/15"></span>
		</span>
		<span class="ml-2 truncate">{cmd}</span>
		<span class="ml-auto flex items-center gap-2"><span class="signal size-1.5 {tone}"></span>{status}</span>
	</div>
	<div class="h-[60dvh] p-2 md:h-[440px]" bind:this={el}></div>
</div>
