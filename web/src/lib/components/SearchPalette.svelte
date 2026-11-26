<script lang="ts">
	import { tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { app } from '#lib/stores/rig.svelte';
	import { views } from '#lib/stores/views.svelte';
	import { searchUI } from '#lib/stores/search.svelte';
	import { buildIndex, search, type SearchItem } from '#lib/search';
	import { panelOf } from '#lib/views';
	import { toast } from 'svelte-sonner';
	import { Search, SlidersHorizontal, Settings, Eye, Palette } from '@lucide/svelte';

	let dialog: HTMLDialogElement | undefined = $state();
	let input: HTMLInputElement | undefined = $state();
	let query = $state('');
	let cursor = $state(0);

	const index = $derived(buildIndex(app.caps, views.all, views.active));
	const results = $derived(search(index, query));
	$effect(() => { query; cursor = 0; });

	// Open and close the native <dialog>: it provides the focus trap, Esc and backdrop.
	$effect(() => {
		if (!dialog) return;
		if (searchUI.open && !dialog.open) {
			dialog.showModal();
			query = '';
			queueMicrotask(() => input?.focus());
		} else if (!searchUI.open && dialog.open) dialog.close();
	});

	function onKey(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') { e.preventDefault(); cursor = Math.min(cursor + 1, results.length - 1); scrollToCursor(); }
		else if (e.key === 'ArrowUp') { e.preventDefault(); cursor = Math.max(cursor - 1, 0); scrollToCursor(); }
		else if (e.key === 'Enter' && results[cursor]) { e.preventDefault(); choose(results[cursor]); }
	}
	function scrollToCursor() {
		tick().then(() => document.getElementById(`sr-${cursor}`)?.scrollIntoView({ block: 'nearest' }));
	}

	async function choose(it: SearchItem) {
		searchUI.hide();
		switch (it.kind) {
			case 'view': views.setActive(it.id); toast.success(`Switched to ${views.get(it.id)?.name}`); break;
			case 'theme': await app.saveSettings({ theme: it.id as 'system' | 'light' | 'dark' | 'red' }).catch(() => {}); break;
			case 'section': {
				if (!it.href.startsWith('/settings')) { await goto(resolve(it.href as '/')); break; }
				try {
					const k = 'rigctlweb.settings.open';
					const open = JSON.parse(localStorage.getItem(k) ?? '{}');
					open[it.id] = true;
					localStorage.setItem(k, JSON.stringify(open));
				} catch { /* ignore */ }
				await goto(resolve(it.href as '/'));
				await tick();
				// The settings page reads the same storage when it mounts; scroll once it has.
				setTimeout(() => document.getElementById(`section-${it.id}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' }), 80);
				break;
			}
			case 'control': await jumpTo(it); break;
		}
	}

	async function jumpTo(it: Extract<SearchItem, { kind: 'control' }>) {
		if (page.url.pathname !== resolve('/')) { await goto(resolve('/')); await tick(); }
		if (!it.inView) {
			const v = views.active;
			if (!v.all) {
				views.setControls(v.id, [it.id], true);
				toast.success(`Added "${it.label}" to the ${v.name} view`, { description: 'Remove it any time in Settings > Views.' });
			}
		}
		await tick();
		const panel = panelOf(it.id);
		if (panel) window.dispatchEvent(new CustomEvent('rw:reveal', { detail: panel }));
		await tick();
		requestAnimationFrame(() => {
			const el = document.querySelector<HTMLElement>(`[data-control~="${CSS.escape(it.id)}"]`) ?? (panel ? document.getElementById(`panel-${panel}`) : null);
			if (!el) return;
			el.scrollIntoView({ behavior: 'smooth', block: 'center' });
			el.classList.remove('jump-flash');
			void el.offsetWidth; // restart the animation
			el.classList.add('jump-flash');
			setTimeout(() => el.classList.remove('jump-flash'), 1800);
		});
	}

	const icon = (k: SearchItem['kind']) => ({ control: SlidersHorizontal, section: Settings, view: Eye, theme: Palette })[k];
</script>

<svelte:window
	onkeydown={(e) => {
		const t = e.target as HTMLElement | null;
		const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(t?.tagName ?? '') || t?.isContentEditable;
		if ((e.key === 'k' && (e.ctrlKey || e.metaKey)) || (e.key === '/' && !typing && !e.ctrlKey && !e.metaKey && !e.altKey)) {
			e.preventDefault();
			searchUI.toggle();
		}
	}}
/>

<dialog
	bind:this={dialog}
	class="m-0 mx-auto mt-[8vh] w-[min(36rem,calc(100vw-1.5rem))] rounded-xl border bg-popover p-0 text-popover-foreground shadow-2xl backdrop:bg-black/50 backdrop:backdrop-blur-[2px]"
	aria-label="Search settings and controls"
	onclose={() => searchUI.hide()}
	onclick={(e) => { if (e.target === dialog) searchUI.hide(); }}
>
	<div class="flex items-center gap-2 border-b px-3">
		<Search class="size-4 shrink-0 text-muted-foreground" />
		<input
			bind:this={input}
			bind:value={query}
			onkeydown={onKey}
			class="h-12 w-full bg-transparent text-base outline-none placeholder:text-muted-foreground"
			placeholder="Search controls and settings…  (try: squelch, notch, tuner)"
			role="combobox"
			aria-expanded={results.length > 0}
			aria-controls="search-results"
			aria-activedescendant={results.length ? `sr-${cursor}` : undefined}
			autocomplete="off"
			spellcheck="false"
		/>
		<kbd class="hidden rounded border px-1.5 py-0.5 text-[10px] text-muted-foreground sm:inline">Esc</kbd>
	</div>
	<ul id="search-results" role="listbox" class="max-h-[min(60vh,28rem)] overflow-y-auto p-1.5">
		{#each results as it, i (it.kind + it.id)}
			{@const Icon = icon(it.kind)}
			<li id="sr-{i}" role="option" aria-selected={i === cursor}>
				<button
					type="button"
					class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm {i === cursor ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/60'}"
					onclick={() => choose(it)}
					onmousemove={() => (cursor = i)}
				>
					<Icon class="size-4 shrink-0 text-muted-foreground" />
					<span class="min-w-0 flex-1 truncate">{it.label}</span>
					{#if it.kind === 'control' && !it.inView}
						<span class="shrink-0 rounded bg-secondary px-1.5 py-0.5 text-[10px] font-medium text-secondary-foreground" title="Selecting it adds it to the current view">not in this view</span>
					{/if}
					<span class="hidden shrink-0 text-xs text-muted-foreground sm:inline">{it.group}</span>
				</button>
			</li>
		{:else}
			<li class="px-3 py-8 text-center text-sm text-muted-foreground">
				{query.trim() ? `Nothing matches "${query}".` : app.caps ? 'Type to search every control your radio offers, plus settings and views.' : 'Connect a radio to search its controls. Settings and views are searchable now.'}
			</li>
		{/each}
	</ul>
	<div class="flex items-center justify-between border-t px-3 py-1.5 text-[11px] text-muted-foreground">
		<span><kbd class="rounded border px-1">↑</kbd> <kbd class="rounded border px-1">↓</kbd> to move, <kbd class="rounded border px-1">Enter</kbd> to go</span>
		<span>Press <kbd class="rounded border px-1">/</kbd> anywhere to search</span>
	</div>
</dialog>
