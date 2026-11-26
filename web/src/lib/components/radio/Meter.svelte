<script lang="ts">
	import { untrack } from 'svelte';
	import Help from '#lib/components/ui/help.svelte';

	let { help, label, value, min = 0, max = 1, unit = '', display, ticks = [], warnAt, hotAt, hold = true, height = 'h-3' }: {
		help?: string;
		label: string;
		value: number | undefined;
		min?: number;
		max?: number;
		unit?: string;
		display?: string;
		ticks?: { at: number; label?: string }[];
		warnAt?: number;
		hotAt?: number;
		hold?: boolean;
		height?: string;
	} = $props();

	const clamp = (v: number) => Math.max(0, Math.min(1, (v - min) / (max - min || 1)));
	const frac = $derived(value === undefined ? 0 : clamp(value));

	let peak = $state(0);
	let peakAt = 0;
	$effect(() => {
		const f = frac;
		const now = performance.now();
		untrack(() => {
			if (f >= peak) {
				peak = f;
				peakAt = now;
			} else if (now - peakAt > 1500) {
				peak = Math.max(f, peak - 0.03);
			}
		});
	});

	const tone = $derived(
		value === undefined ? 'bg-muted-foreground/30'
		: hotAt !== undefined && value >= hotAt ? 'bg-meter-hot'
		: warnAt !== undefined && value >= warnAt ? 'bg-meter-warn'
		: 'bg-meter-ok'
	);
</script>

<div class="space-y-1" role="meter" aria-label={label} aria-valuemin={min} aria-valuemax={max} aria-valuenow={value}>
	<div class="flex items-baseline justify-between text-xs">
		<span class="flex items-center gap-1 font-medium text-muted-foreground">{label}{#if help}<Help kind="ui" name={help} />{/if}</span>
		<span class="font-mono tabular-nums text-foreground">{display ?? (value === undefined ? '—' : value.toFixed(2))}{value !== undefined ? unit : ''}</span>
	</div>
	<div class="relative overflow-hidden rounded-full bg-muted {height}">
		<div class="h-full rounded-full transition-[width] duration-100 ease-out {tone}" style="width:{frac * 100}%"></div>
		{#if hold && value !== undefined}
			<div class="absolute top-0 h-full w-0.5 bg-foreground/70" style="left:calc({peak * 100}% - 1px)"></div>
		{/if}
		{#each ticks as t}
			<div class="absolute top-0 h-full w-px bg-background/70" style="left:{clamp(t.at) * 100}%"></div>
		{/each}
	</div>
	{#if ticks.some((t) => t.label)}
		<div class="relative h-3 text-[10px] text-muted-foreground">
			{#each ticks as t}
				{#if t.label}<span class="absolute -translate-x-1/2" style="left:{clamp(t.at) * 100}%">{t.label}</span>{/if}
			{/each}
		</div>
	{/if}
</div>
