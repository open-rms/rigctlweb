<script lang="ts">
	import { cn } from '#lib/utils';
	import type { HTMLSelectAttributes } from 'svelte/elements';

	let { class: className, children, value = $bindable(), ...rest }: HTMLSelectAttributes = $props();
</script>

<select
	bind:value
	class={cn(
		'h-9 w-full rounded-md border border-input bg-background px-2.5 text-sm shadow-xs outline-none transition-colors focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40 disabled:cursor-not-allowed disabled:opacity-50',
		className
	)}
	{...rest}
>{@render children?.()}</select>
