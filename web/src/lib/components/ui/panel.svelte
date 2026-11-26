<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { ChevronDown } from '@lucide/svelte';
	import Help from '#lib/components/ui/help.svelte';
	import type { HelpKind } from '#lib/help';

	// A titled, collapsible section. Open/closed state is remembered per browser. Panels flow
	// in CSS columns, so the page rearranges itself to whatever width it has.
	let { id, title, help, defaultOpen = true, class: className = '', children }: {
		id: string;
		title: string;
		help?: { kind: HelpKind; name: string };
		/** true, false, or 'wide' (open on wide screens, collapsed on phones to cut clutter) */
		defaultOpen?: boolean | 'wide';
		class?: string;
		children: Snippet;
	} = $props();

	const key = () => `rigctlweb.panel.${id}`;
	let open = $state(true);

	onMount(() => {
		let saved: string | null = null;
		try { saved = localStorage.getItem(key()); } catch { /* storage unavailable */ }
		if (saved === '1' || saved === '0') open = saved === '1';
		else open = defaultOpen === 'wide' ? matchMedia('(min-width: 640px)').matches : defaultOpen;

		// Search results ask a panel to open so the control they point at is visible.
		const reveal = (e: Event) => { if ((e as CustomEvent<string>).detail === id) open = true; };
		window.addEventListener('rw:reveal', reveal);
		return () => window.removeEventListener('rw:reveal', reveal);
	});

	function toggle() {
		open = !open;
		try { localStorage.setItem(key(), open ? '1' : '0'); } catch { /* ignore */ }
	}
</script>

<section id="panel-{id}" class="mb-4 break-inside-avoid rounded-xl border bg-card text-card-foreground shadow-sm {className}" aria-labelledby="panel-{id}-title">
	<div class="flex items-center gap-1 px-4 {open ? 'pt-3 pb-2' : 'py-3'}">
		<h2 id="panel-{id}-title" class="min-w-0 flex-1 text-sm font-medium">
			<button type="button" class="flex w-full items-center gap-1.5 truncate text-left" onclick={toggle} aria-expanded={open} aria-controls="panel-{id}-body">{title}</button>
		</h2>
		{#if help}<Help kind={help.kind} name={help.name} />{/if}
		<button type="button" class="-mr-1 inline-flex size-7 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground" onclick={toggle} aria-label={open ? `Collapse ${title}` : `Expand ${title}`} aria-expanded={open}>
			<ChevronDown class="size-4 transition-transform {open ? '' : '-rotate-90'}" />
		</button>
	</div>
	{#if open}
		<div id="panel-{id}-body" class="space-y-4 px-4 pb-4">{@render children()}</div>
	{/if}
</section>
