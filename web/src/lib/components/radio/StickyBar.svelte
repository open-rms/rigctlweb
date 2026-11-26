<script lang="ts">
	import { app } from '#lib/stores/rig.svelte';
	import { views } from '#lib/stores/views.svelte';
	import { freqGroups, sUnits, bandOf } from '#lib/radio';
	import { Mic } from '@lucide/svelte';

	// A slim always-visible readout (frequency, mode, TX/RX, S-meter, PTT) that slides in once
	// the main operating card scrolls out of view, so a long page never hides what you're on.
	let { target, onptt }: { target: HTMLElement | undefined; onptt: () => void } = $props();

	let visible = $state(false);
	$effect(() => {
		if (!target) return;
		const io = new IntersectionObserver(([e]) => (visible = !e.isIntersecting && e.boundingClientRect.top < 0), { threshold: 0 });
		io.observe(target);
		return () => io.disconnect();
	});

	const st = $derived(app.state);
	const sm = $derived(st?.levels['STRENGTH'] === undefined ? undefined : sUnits(st.levels['STRENGTH']));
	const band = $derived(st ? bandOf(st.freq)?.name : undefined);
</script>

{#if visible && st}
	<div class="fixed inset-x-0 top-0 z-40 border-b bg-background/90 backdrop-blur supports-[backdrop-filter]:bg-background/75" role="region" aria-label="Current frequency">
		<div class="mx-auto flex max-w-[100rem] items-center gap-3 px-3 py-1.5 sm:px-6">
			<button type="button" class="min-w-0 rounded text-left" onclick={() => window.scrollTo({ top: 0, behavior: 'smooth' })} aria-label="Back to the top">
				<span class="font-mono text-xl font-medium tabular-nums text-display-foreground sm:text-2xl">{freqGroups(st.freq).join('.')}</span>
			</button>
			<span class="hidden text-sm text-muted-foreground sm:inline">{st.mode}{band ? ` · ${band}` : ''}</span>
			{#if st.ptt}<span class="animate-pulse rounded bg-tx px-1.5 py-0.5 text-[10px] font-bold tracking-widest text-white">TX</span>{:else}<span class="text-[10px] tracking-widest text-muted-foreground">RX</span>{/if}
			{#if sm && !st.ptt}
				<div class="ml-auto hidden h-2 w-24 overflow-hidden rounded-full bg-muted sm:block" title={sm.label}><div class="h-full bg-meter-ok" style="width:{sm.frac * 100}%"></div></div>
			{/if}
			{#if app.caps?.canPtt && views.shows('ptt') && !app.readOnly}
				<button type="button" class="ml-auto inline-flex h-8 items-center gap-1.5 rounded-md px-3 text-sm font-semibold sm:ml-2 {st.ptt ? 'bg-tx text-white' : 'bg-secondary text-secondary-foreground'}" onclick={onptt} aria-label={st.ptt ? 'Stop transmitting' : 'Push to talk'}>
					<Mic class="size-4" /><span class="hidden sm:inline">{st.ptt ? 'Stop' : 'PTT'}</span>
				</button>
			{/if}
		</div>
	</div>
{/if}
