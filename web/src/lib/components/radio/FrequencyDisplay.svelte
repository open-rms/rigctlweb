<script lang="ts">
	import { freqGroups, parseFreq } from '#lib/radio';
	import { ChevronUp, ChevronDown, Keyboard } from '@lucide/svelte';

	let { hz, label = '', active = true, disabled = false, onchange }: {
		hz: number;
		label?: string;
		active?: boolean;
		disabled?: boolean;
		onchange: (hz: number) => void;
	} = $props();

	// What's on screen: the user's in-flight value if there is one, otherwise the radio's.
	// Rapid steps (wheel, held arrows) update this instantly and are sent as one command.
	let pending = $state<number | null>(null);
	let sendTimer: ReturnType<typeof setTimeout> | undefined;
	let settleTimer: ReturnType<typeof setTimeout> | undefined;
	const shown = $derived(pending ?? hz);

	$effect(() => {
		// The radio caught up with what we asked for: go back to showing its value.
		if (pending !== null && hz === pending) pending = null;
	});

	function commit(v: number) {
		pending = v;
		clearTimeout(sendTimer);
		sendTimer = setTimeout(() => onchange(v), 90);
		clearTimeout(settleTimer);
		// If the radio never confirms (refused, out of range), fall back to its real value.
		settleTimer = setTimeout(() => (pending = null), 2500);
	}

	// Each digit is worth 10^place Hz.
	function step(place: number, dir: 1 | -1) {
		if (disabled) return;
		const next = shown + 10 ** place * dir;
		if (next > 0) commit(next);
	}

	const groups = $derived(freqGroups(shown));
	const places = $derived.by(() => {
		let offset = 0;
		const total = groups.join('').length;
		return groups.map((g) => {
			const arr = [...g].map((_, i) => total - (offset + i) - 1);
			offset += g.length;
			return arr;
		});
	});

	// ---- typed entry ----
	let editing = $state(false);
	let text = $state('');
	let input: HTMLInputElement | undefined = $state();
	let bad = $state(false);

	export function startEdit(initial?: string) {
		if (disabled) return;
		text = initial ?? (shown / 1e6).toFixed(6).replace(/0+$/, '').replace(/\.$/, '');
		bad = false;
		editing = true;
		queueMicrotask(() => {
			input?.focus();
			if (initial === undefined) input?.select();
		});
	}
	function accept() {
		const v = parseFreq(text);
		if (v === null) {
			bad = true;
			return;
		}
		editing = false;
		if (v !== shown) commit(v);
	}
	function onInputKey(e: KeyboardEvent) {
		if (e.key === 'Enter') accept();
		else if (e.key === 'Escape') editing = false;
		else bad = false;
	}

	// Svelte registers wheel listeners as passive, which makes preventDefault a no-op and
	// scrolls the page instead of tuning. This action registers a non-passive one.
	function wheel(node: HTMLElement, fn: (e: WheelEvent) => void) {
		const h = (e: WheelEvent) => fn(e);
		node.addEventListener('wheel', h, { passive: false });
		return { destroy: () => node.removeEventListener('wheel', h) };
	}
</script>

<div class="flex flex-wrap items-center gap-3">
	{#if label}
		<span class="w-10 text-xs font-semibold uppercase tracking-widest {active ? 'text-display-foreground' : 'text-muted-foreground'}">{label}</span>
	{/if}

	{#if editing}
		<div class="min-w-0 flex-1">
			<input
				bind:this={input}
				bind:value={text}
				onkeydown={onInputKey}
				onblur={() => !bad && (editing = false)}
				inputmode="decimal"
				autocomplete="off"
				spellcheck="false"
				aria-label="Frequency"
				aria-invalid={bad}
				placeholder="14.074"
				class="w-full rounded-lg border bg-display px-4 py-2 font-mono text-[clamp(1.9rem,9.5vw,3.75rem)] tabular-nums text-display-foreground outline-none {bad ? 'border-destructive' : 'border-ring'}"
			/>
			<p class="mt-1.5 text-xs {bad ? 'text-destructive' : 'text-muted-foreground'}">
				{bad ? "Couldn't read that frequency." : 'MHz by default: 14.074, 7074k, 146520000. Enter to tune, Esc to cancel.'}
			</p>
		</div>
	{:else}
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<div
			class="group relative flex select-none items-baseline rounded-lg bg-display px-4 py-5 font-mono text-[clamp(1.9rem,9.5vw,3.75rem)] font-medium tabular-nums tracking-tight text-display-foreground shadow-inner {disabled ? 'opacity-60' : ''}"
			role="group"
			aria-label="Frequency {shown} Hz. Scroll or use arrow keys on a digit to tune; double-click to type."
			ondblclick={() => startEdit()}
		>
			{#each groups as g, gi}
				{#if gi > 0}<span class="px-0.5 opacity-50">.</span>{/if}
				{#each [...g] as d, di}
					{@const place = places[gi][di]}
					<span
						class="relative cursor-ns-resize rounded px-[1px] transition-colors hover:bg-white/10 focus-visible:bg-white/15 focus-visible:outline-none"
						role="spinbutton"
						tabindex="0"
						aria-valuenow={Number(d)}
						aria-valuemin={0}
						aria-valuemax={9}
						aria-label="{10 ** place} Hz digit"
						use:wheel={(e) => { e.preventDefault(); step(place, e.deltaY < 0 ? 1 : -1); }}
						onkeydown={(e) => {
							if (e.key === 'ArrowUp') { e.preventDefault(); step(place, 1); }
							else if (e.key === 'ArrowDown') { e.preventDefault(); step(place, -1); }
							else if (e.key === 'Enter' || /^[0-9.,]$/.test(e.key)) { e.preventDefault(); startEdit(/^[0-9.,]$/.test(e.key) ? e.key : undefined); }
						}}
					>
						{d}
						{#if !disabled}
							<button type="button" tabindex="-1" aria-hidden="true" class="absolute -top-0.5 left-1/2 -translate-x-1/2 rounded p-0.5 opacity-0 transition-opacity hover:bg-white/20 group-hover:opacity-70 hover:!opacity-100" onclick={() => step(place, 1)}><ChevronUp class="size-4" /></button>
							<button type="button" tabindex="-1" aria-hidden="true" class="absolute -bottom-0.5 left-1/2 -translate-x-1/2 rounded p-0.5 opacity-0 transition-opacity hover:bg-white/20 group-hover:opacity-70 hover:!opacity-100" onclick={() => step(place, -1)}><ChevronDown class="size-4" /></button>
						{/if}
					</span>
				{/each}
			{/each}
			<span class="ml-2 font-sans text-sm text-muted-foreground">Hz</span>
		</div>
		<button
			type="button"
			class="inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground disabled:opacity-40"
			onclick={() => startEdit()}
			{disabled}
			title="Type a frequency (press F)"
		><Keyboard class="size-4" />Enter frequency <kbd class="rounded bg-muted px-1 font-mono text-[10px]">F</kbd></button>
	{/if}
</div>
