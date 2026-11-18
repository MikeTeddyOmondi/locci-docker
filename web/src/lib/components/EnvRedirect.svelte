<script>
	// Sends a pre-environments URL (/, /containers/abc) to the same page in the
	// remembered or default environment, so old links and bookmarks keep working.
	import { goto } from '$app/navigation';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import { envs, loadEnvs, preferred } from '$lib/env.svelte.js';
	import { attempt } from '$lib/session.svelte.js';

	let { path = '' } = $props();

	$effect(() => {
		(async () => {
			if (!envs.loaded) await attempt(loadEnvs);
			goto(`/e/${encodeURIComponent(preferred())}${path}`, { replaceState: true });
		})();
	});
</script>

<div class="space-y-6" aria-busy="true">
	<Skeleton class="h-9 w-56" />
	<div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
		{#each Array(4) as _}<Skeleton class="h-28 rounded-xl" />{/each}
	</div>
</div>
