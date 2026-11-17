<script>
	// Container state shown as a harbour signal light.
	import { cn } from '$lib/utils.js';

	let { state = 'unknown', class: className = '' } = $props();

	const tone = $derived(
		{
			running: 'text-ok bg-ok/10 ring-ok/25',
			paused: 'text-warn bg-warn/10 ring-warn/30',
			restarting: 'text-warn bg-warn/10 ring-warn/30',
			exited: 'text-destructive bg-destructive/10 ring-destructive/25',
			dead: 'text-destructive bg-destructive/10 ring-destructive/25'
		}[state] ?? 'text-muted-foreground bg-muted ring-border'
	);
</script>

<span
	class={cn(
		'inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 font-mono text-[0.7rem] font-medium tracking-wide uppercase ring-1 ring-inset',
		tone,
		className
	)}
>
	<span class={cn('signal size-1.5', (state === 'running' || state === 'restarting') && 'signal-live')}
	></span>
	{state}
</span>
