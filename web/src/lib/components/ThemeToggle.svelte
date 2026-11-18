<script>
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import { buttonVariants } from '$lib/components/ui/button/index.js';
	import { userPrefersMode, setMode } from 'mode-watcher';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import MonitorIcon from '@lucide/svelte/icons/monitor';

	let { align = 'end', class: className = '' } = $props();

	const modes = [
		['light', 'Day shift', SunIcon],
		['dark', 'Night shift', MoonIcon],
		['system', 'Follow system', MonitorIcon]
	];
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		class={buttonVariants({ variant: 'ghost', size: 'icon' }) + ' relative ' + className}
		aria-label="Change theme"
	>
		<SunIcon class="size-[1.1rem] scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90" />
		<MoonIcon
			class="absolute size-[1.1rem] scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0"
		/>
	</DropdownMenu.Trigger>
	<DropdownMenu.Content {align} class="w-44">
		<DropdownMenu.Label class="kicker">Theme</DropdownMenu.Label>
		<DropdownMenu.RadioGroup value={userPrefersMode.current} onValueChange={(v) => setMode(v)}>
			{#each modes as [value, label, Icon]}
				<DropdownMenu.RadioItem {value}>
					<Icon class="size-4 text-muted-foreground" />
					{label}
				</DropdownMenu.RadioItem>
			{/each}
		</DropdownMenu.RadioGroup>
	</DropdownMenu.Content>
</DropdownMenu.Root>
