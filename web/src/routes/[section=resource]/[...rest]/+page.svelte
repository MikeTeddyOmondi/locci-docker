<script>
	import { page } from '$app/state';
	import EnvRedirect from '$lib/components/EnvRedirect.svelte';

	const path = $derived(
		'/' + page.params.section + (page.params.rest ? '/' + page.params.rest : '') + page.url.search
	);
</script>

{#key path}<EnvRedirect {path} />{/key}
