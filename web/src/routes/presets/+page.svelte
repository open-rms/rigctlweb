<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { app } from '#lib/stores/rig.svelte';
	import {
		api, type PresetCheck, type PresetLibrary, type PresetSnapshot, type PresetStep, type PresetSummary, type PresetView
	} from '#lib/api';
	import { Button } from '#lib/components/ui/button';
	import { Badge } from '#lib/components/ui/badge';
	import NativeSelect from '#lib/components/ui/native-select.svelte';
	import * as Dialog from '#lib/components/ui/dialog';
	import { ArrowLeft, ExternalLink, RefreshCw, TriangleAlert } from '@lucide/svelte';

	// openRMS Rigs presets: crowd-sourced radio settings, shown like a manual and applied through
	// rigctlWeb's own checked commands. Everything in a profile is text written by other people:
	// it is rendered as text, links are http(s) only, and no remote images are loaded.

	let library = $state<PresetLibrary | null>(null);
	let radio = $state<{ mfg: string; model: string } | null>(null);
	let profiles = $state<PresetSummary[]>([]);
	let updating = $state(false);
	let scope = $state<'radio' | 'all'>('radio');
	let operation = $state('');
	let openId = $state<string | null>(null);
	let view = $state<PresetView | null>(null);
	let check = $state<PresetCheck | null>(null);
	let checking = $state(false);
	let chosen = $state<Record<string, boolean>>({});
	let confirmOpen = $state(false);
	let catReviewed = $state(false);
	let applying = $state(false);
	let snapshot = $state<PresetSnapshot | null>(null);
	let restoring = $state(false);
	let loadError = $state('');
	const rig = $derived(app.selected);
	const keyed = $derived(!!app.state?.ptt);

	async function loadList() {
		try {
			const r = await api.presets(rig);
			library = r.library;
			radio = r.radio;
			profiles = r.profiles;
			loadError = '';
		} catch (e) {
			loadError = (e as Error).message;
		}
	}

	async function loadSnapshot() {
		snapshot = rig ? ((await api.presetSnapshot(rig).catch(() => null))?.snapshot ?? null) : null;
	}

	async function open(id: string | null) {
		openId = id;
		view = null;
		check = null;
		chosen = {};
		if (!id) return;
		try {
			view = await api.preset(id, rig);
			// Receive-side settings start ticked; transmit-side ones are the operator's choice.
			chosen = Object.fromEntries(view.steps.filter((s) => s.applicable).map((s) => [s.id, s.safety === 'rx']));
		} catch (e) {
			toast.error((e as Error).message);
			openId = null;
		}
	}

	$effect(() => {
		void rig;
		untrack(() => {
			void loadList();
			void loadSnapshot();
			if (openId) void open(openId);
		});
	});

	onMount(() => {
		const fromHash = () => {
			const id = location.hash.slice(1) || null;
			if (id !== openId) void open(id);
		};
		fromHash();
		addEventListener('hashchange', fromHash);
		return () => removeEventListener('hashchange', fromHash);
	});

	async function update() {
		updating = true;
		try {
			library = await api.updatePresets();
			toast.success(`Library updated: ${library.count} preset${library.count === 1 ? '' : 's'}`);
		} catch (e) {
			toast.error(`Update failed: ${(e as Error).message}`);
		} finally {
			updating = false;
			await loadList();
		}
	}

	async function runCheck() {
		if (!rig || !view) return;
		checking = true;
		try {
			check = await api.checkPreset(rig, view.id);
			// Untick settings that already match.
			for (const s of check.steps) if (s.noop && chosen[s.id]) chosen[s.id] = false;
		} catch (e) {
			toast.error((e as Error).message);
		} finally {
			checking = false;
		}
	}

	async function apply() {
		if (!rig || !view) return;
		applying = true;
		try {
			const r = await api.applyPreset(rig, view.id, toApply.map((s) => s.id), catToSend.map((s) => s.command!));
			if (r.failed) toast.error(`Set ${r.applied.length}, then ${r.failed} failed: ${r.error}. ${r.skipped.length} not tried.`);
			else toast.success(`Set ${r.applied.length} setting${r.applied.length === 1 ? '' : 's'}. Restore puts the old values back.`);
			confirmOpen = false;
			await loadSnapshot();
			await runCheck();
		} catch (e) {
			toast.error((e as Error).message);
		} finally {
			applying = false;
		}
	}

	async function restore() {
		if (!rig) return;
		restoring = true;
		try {
			const r = await api.restorePresets(rig);
			if (r.failed) toast.error(`Restored ${r.restored.length}, then ${r.failed} failed: ${r.error}`);
			else toast.success(`Restored ${r.restored.length} setting${r.restored.length === 1 ? '' : 's'}${r.unknown.length ? `; ${r.unknown.length} could not be read before, so set them by hand` : ''}`);
			await loadSnapshot();
			if (view) await runCheck();
		} catch (e) {
			toast.error((e as Error).message);
		} finally {
			restoring = false;
		}
	}

	const operations = $derived([...new Set(profiles.map((p) => p.operation))].sort());
	const listed = $derived(profiles.filter((p) => (scope === 'all' || p.matches) && (!operation || p.operation === operation)));
	const matching = $derived(profiles.filter((p) => p.matches).length);

	// Steps for the operator to set on the radio: manual items, and raw CAT rigctlWeb can't send.
	function onRadio(x: PresetStep) {
		return x.kind === 'manual' || (x.kind === 'cat' && !x.applicable && !x.checkOnly);
	}
	const groups = $derived.by(() => {
		const s = view?.steps ?? [];
		return {
			apply: s.filter((x) => x.applicable),
			radio: s.filter(onRadio),
			other: s.filter((x) => !x.applicable && !onRadio(x))
		};
	});
	const toApply = $derived(groups.apply.filter((s) => chosen[s.id]));
	const catToSend = $derived(toApply.filter((s) => s.kind === 'cat' && s.command));
	const statusOf = (id: string) => check?.steps.find((s) => s.id === id);
	const statusCls = { ok: 'text-meter-ok', drift: 'text-meter-warn', unknown: 'text-muted-foreground', manual: 'text-muted-foreground' } as const;
	const statusText = { ok: 'matches', drift: 'differs', unknown: 'unknown', manual: 'on the radio' } as const;
	const sourceOf = (id: string) => view?.sources.find((s) => s.id === id);
	const safeUrl = (u?: string) => (u && /^https?:\/\//i.test(u) ? u : undefined);
	const when = (iso?: string) => (iso ? new Date(iso).toLocaleString() : '');
	const opLabel = (o: string) => o.toUpperCase().replace('WINLINK-', 'Winlink ');

	function expectText(s: PresetStep): string {
		const e = s.expect;
		if (!e) return '';
		const parts: string[] = [];
		if (e.min !== undefined || e.max !== undefined) parts.push(`${e.min ?? '…'} to ${e.max ?? '…'}`);
		if (e.one_of?.length) parts.push(`one of ${e.one_of.join(', ')}`);
		if (e.not?.length) parts.push(`not ${e.not.map((v) => (s.choices?.[String(v)] ?? (s.name === 'AGC' && v === 0 ? 'OFF' : v))).join(', ')}`);
		return parts.length ? `accepts ${parts.join('; ')}` : '';
	}
	function catValue(s: PresetStep): string {
		if (s.value === undefined) return '';
		const v = String(s.value);
		return s.choices?.[v] ? `${s.choices[v]} (${v})` : v;
	}
</script>

{#snippet stepDetail(s: PresetStep)}
	{#if s.kind === 'cat' && s.readable && !s.command}<p class="mt-1 text-xs text-muted-foreground">Raw CAT: Check reads <code class="font-mono">{s.catRead}</code></p>{/if}
	{#if s.note}<p class="mt-1 whitespace-pre-wrap text-xs text-muted-foreground">{s.note}</p>{/if}
	{#if s.menu?.length && !s.manual}<p class="mt-1 text-xs text-muted-foreground">On the radio: {s.menu.join(' › ')}</p>{/if}
	<div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
		{#if expectText(s)}<span>{expectText(s)}</span>{/if}
		{#if s.hamlib}<span>Hamlib {s.hamlib}</span>{/if}
		{#if s.firmware}<span>firmware {s.firmware}</span>{/if}
		{#if s.confidence}<span>confidence: <span class="font-medium text-foreground">{s.confidence}</span></span>{/if}
		{#each s.sources as id (id)}
			{@const src = sourceOf(id)}
			<a class="underline decoration-dotted" href={`#src-${id}`} title={src ? `${src.title}${src.author ? ' — ' + src.author : ''}` : id}>{id}</a>
		{/each}
		{#if s.reason && !s.applicable}<span>{s.reason}</span>{/if}
	</div>
{/snippet}

{#snippet status(id: string)}
	{@const st = statusOf(id)}
	{#if st}
		<span class={`text-xs ${statusCls[st.status]}`} title={st.error ?? ''}>
			{statusText[st.status]}{#if st.currentText !== undefined && st.status !== 'manual'}: <span class="font-mono">{st.currentText}</span>{/if}
		</span>
	{/if}
{/snippet}

<div class="mx-auto max-w-4xl space-y-5">
	{#if !openId}
		<div>
			<h1 class="text-2xl font-semibold tracking-tight">Radio presets</h1>
			<p class="mt-1 text-sm text-muted-foreground">
				Settings for your radio model and an operation (FT8, Winlink, SSB…), shared by other operators through
				<a class="underline" href="https://rigs.openrms.dev" target="_blank" rel="noopener noreferrer">openRMS Rigs</a>.
				Read each one like a manual, check it against your radio, and apply the settings rigctlWeb can set. Nothing that keys the transmitter is ever applied.
			</p>
		</div>

		<div class="flex flex-wrap items-center gap-3 rounded-xl border bg-card p-4">
			<div class="min-w-0 flex-1 text-sm">
				{#if library?.available}
					<span class="font-medium">{library.count} preset{library.count === 1 ? '' : 's'}</span>
					<span class="text-muted-foreground"> · updated {when(library.fetchedAt)}</span>
				{:else}
					<span class="text-muted-foreground">No presets yet. Update the library to download them; after that they work offline.</span>
				{/if}
				{#if library?.error}<p class="mt-1 text-xs text-destructive" role="alert">{library.error}</p>{/if}
				<p class="mt-1 text-xs text-muted-foreground">Updating contacts {library?.source ?? 'rigs.openrms.dev'} only when you press the button. It sends nothing about your station.</p>
			</div>
			<Button variant="outline" onclick={update} disabled={updating}>
				<RefreshCw class={`size-4 ${updating ? 'animate-spin' : ''}`} /> {updating ? 'Updating…' : 'Update library'}
			</Button>
		</div>
		{#if loadError}<p class="text-sm text-destructive" role="alert">{loadError}</p>{/if}

		{#if snapshot && rig}
			<div class="flex flex-wrap items-center gap-3 rounded-xl border border-meter-warn/40 bg-card p-4 text-sm">
				<div class="flex-1">Presets changed {snapshot.saved.length} setting{snapshot.saved.length === 1 ? '' : 's'} on this radio ({snapshot.presets.join(', ')}).</div>
				<Button variant="outline" onclick={restore} disabled={restoring || keyed || app.readOnly}>{restoring ? 'Restoring…' : 'Restore previous settings'}</Button>
			</div>
		{/if}

		{#if library?.available}
			<div class="flex flex-wrap items-end gap-3">
				<label class="space-y-1 text-xs text-muted-foreground">Show
					<NativeSelect value={scope} onchange={(e) => (scope = e.currentTarget.value as 'radio' | 'all')}>
						<option value="radio">{radio ? `For this radio (${radio.mfg} ${radio.model}): ${matching}` : 'For this radio'}</option>
						<option value="all">All radios: {profiles.length}</option>
					</NativeSelect>
				</label>
				<label class="space-y-1 text-xs text-muted-foreground">Operation
					<NativeSelect value={operation} onchange={(e) => (operation = e.currentTarget.value)}>
						<option value="">Any</option>
						{#each operations as o (o)}<option value={o}>{opLabel(o)}</option>{/each}
					</NativeSelect>
				</label>
			</div>

			{#if !listed.length}
				<p class="text-sm text-muted-foreground">
					{scope === 'radio' ? (radio ? `No presets for the ${radio.mfg} ${radio.model} yet. Show all radios to read the others.` : 'Connect a radio to see its presets, or show all radios.') : 'No presets match.'}
				</p>
			{/if}
			<ul class="grid gap-3 sm:grid-cols-2">
				{#each listed as p (p.id)}
					<li>
						<button type="button" class="h-full w-full rounded-xl border bg-card p-4 text-left transition-colors hover:bg-accent" onclick={() => (location.hash = p.id)}>
							<div class="flex items-start gap-2">
								<span class="flex-1 font-medium">{p.title}</span>
								<Badge variant="secondary">{opLabel(p.operation)}</Badge>
							</div>
							<div class="mt-1 text-xs text-muted-foreground">{p.rigMfg} {p.rigName}{p.author ? ` · by ${p.author}` : ''}</div>
							<div class="mt-2 flex flex-wrap gap-1.5 text-xs">
								{#if p.refused}<Badge variant="destructive">Refused</Badge>{/if}
								{#each p.tags as t (t)}<Badge variant="outline">{t}</Badge>{/each}
								{#if p.matches && !p.refused}<span class="text-muted-foreground">{p.settings} to set · {p.manual} on the radio</span>{/if}
							</div>
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	{:else}
		<Button variant="ghost" size="sm" onclick={() => (location.hash = '')}><ArrowLeft class="size-4" /> All presets</Button>
		{#if !view}
			<p class="text-sm text-muted-foreground">Loading…</p>
		{:else}
			<header class="space-y-2">
				<div class="flex flex-wrap items-start gap-2">
					<h1 class="flex-1 text-2xl font-semibold tracking-tight">{view.title}</h1>
					<Badge variant="secondary">{opLabel(view.operation)}</Badge>
				</div>
				{#if view.summary}<p class="text-sm text-muted-foreground">{view.summary}</p>{/if}
				<p class="text-sm">
					{view.rigMfg} {view.rigName}{view.connection ? ` over ${view.connection}` : ''}{view.interface ? ` (${view.interface})` : ''}{view.hamlib ? ` · Hamlib ${view.hamlib}` : ''}{view.firmware ? ` · firmware ${view.firmware}` : ''}
				</p>
				<div class="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
					{#each view.tags as t (t)}<Badge variant="outline">{t}</Badge>{/each}
					<span>★ {view.stats.stars} · worked for {view.stats.worked} · didn't for {view.stats.not_worked}</span>
				</div>
				{#if view.refused}
					<p class="flex items-start gap-2 rounded-lg border border-destructive/40 p-3 text-sm text-destructive" role="alert"><TriangleAlert class="mt-0.5 size-4 shrink-0" /> rigctlWeb will not apply any of this preset: {view.refused}</p>
				{:else if !view.matches}
					<p class="rounded-lg border p-3 text-sm text-muted-foreground">This preset is for the {view.rigMfg} {view.rigName}{radio ? `, not your ${radio.mfg} ${radio.model}` : ''}. You can read it, but rigctlWeb won't apply it.</p>
				{/if}
			</header>

			{#if view.matches && !view.refused}
				<div class="flex flex-wrap items-center gap-2 rounded-xl border bg-card p-4">
					<Button variant="outline" onclick={runCheck} disabled={checking || !rig}>{checking ? 'Checking…' : check ? 'Check again' : 'Check against my radio'}</Button>
					<Button onclick={() => ((catReviewed = false), (confirmOpen = true))} disabled={!toApply.length || app.readOnly || keyed}
						title={app.readOnly ? 'This radio is read-only' : keyed ? 'Not while the radio is transmitting' : ''}>
						Apply {toApply.length} setting{toApply.length === 1 ? '' : 's'}…
					</Button>
					{#if snapshot}<Button variant="ghost" onclick={restore} disabled={restoring || keyed || app.readOnly}>{restoring ? 'Restoring…' : 'Restore previous settings'}</Button>{/if}
					{#if check}<span class="text-xs text-muted-foreground">{check.summary.ok} match · {check.summary.drift} differ · {check.summary.unknown} unknown · {check.summary.manual} on the radio</span>{/if}
					<p class="w-full text-xs text-muted-foreground">Checking reads your radio and compares here; nothing is sent anywhere. Applying saves the current values first, so Restore can put them back.</p>
				</div>
			{/if}

			{#if groups.apply.length}
				<section class="space-y-2">
					<h2 class="text-lg font-semibold">Settings rigctlWeb can set</h2>
					<ul class="divide-y rounded-xl border bg-card">
						{#each groups.apply as s (s.id)}
							<li class="flex gap-3 px-4 py-3">
								<input type="checkbox" class="mt-1" bind:checked={chosen[s.id]} aria-label={`Apply ${s.text}`} disabled={app.readOnly} />
								<div class="min-w-0 flex-1">
									<div class="flex flex-wrap items-baseline gap-x-2">
										<span class="text-sm font-medium">{s.text}</span>
										{#if s.safety === 'tx'}<Badge variant="destructive" title="Changes how the radio transmits. Off until you tick it.">transmit</Badge>{/if}
										{#if !s.readable}<span class="text-xs text-meter-warn">can't be read back, so Restore can't undo it</span>{/if}
										{@render status(s.id)}
									</div>
									{#if s.command}<p class="mt-1 text-xs text-muted-foreground">Raw CAT: reads <code class="font-mono">{s.catRead}</code>, sends <code class="font-mono">{s.command}</code></p>{/if}
									{@render stepDetail(s)}
								</div>
							</li>
						{/each}
					</ul>
				</section>
			{/if}

			{#if groups.radio.length}
				<section class="space-y-2">
					<h2 class="text-lg font-semibold">Set these on the radio</h2>
					<p class="text-sm text-muted-foreground">Menu settings rigctlWeb can't change on this radio. Each says why.</p>
					<ul class="divide-y rounded-xl border bg-card">
						{#each groups.radio as s (s.id)}
							<li class="px-4 py-3">
								<div class="flex flex-wrap items-baseline gap-x-2">
									<span class="text-sm font-medium">{s.manual ? s.manual.menu : s.name}</span>
									<span class="font-mono text-sm">{s.manual ? s.manual.value : catValue(s)}</span>
									{#if s.checkOnly}<span class="text-xs text-muted-foreground">(check only)</span>{/if}
									{#if s.safety === 'tx'}<Badge variant="destructive">transmit</Badge>{/if}
								</div>
								{@render stepDetail(s.kind === 'cat' ? s : { ...s, reason: undefined })}
							</li>
						{/each}
					</ul>
				</section>
			{/if}

			{#if groups.other.length}
				<section class="space-y-2">
					<h2 class="text-lg font-semibold">Checked only, or not available here</h2>
					<ul class="divide-y rounded-xl border bg-card">
						{#each groups.other as s (s.id)}
							<li class="px-4 py-3">
								<div class="flex flex-wrap items-baseline gap-x-2">
									<span class="text-sm font-medium">{s.text}</span>
									{#if s.safety === 'tx'}<Badge variant="destructive">transmit</Badge>{/if}
									{@render status(s.id)}
								</div>
								{@render stepDetail(s)}
							</li>
						{/each}
					</ul>
				</section>
			{/if}

			{#if view.audio && (view.audio.rx_target_dbfs !== undefined || view.audio.rx_note || view.audio.tx_note)}
				<section class="space-y-1 text-sm">
					<h2 class="text-lg font-semibold">Audio</h2>
					{#if view.audio.rx_target_dbfs !== undefined}<p>Receive noise floor at the computer: about {view.audio.rx_target_dbfs} dBFS.</p>{/if}
					{#if view.audio.rx_note}<p class="whitespace-pre-wrap text-muted-foreground">{view.audio.rx_note}</p>{/if}
					{#if view.audio.tx_note}<p class="whitespace-pre-wrap text-muted-foreground">{view.audio.tx_note}</p>{/if}
				</section>
			{/if}

			{#if view.software.length}
				<section class="space-y-2">
					<h2 class="text-lg font-semibold">In your software</h2>
					<ul class="divide-y rounded-xl border bg-card text-sm">
						{#each view.software as sw, i (i)}
							<li class="px-4 py-2"><span class="font-medium">{sw.app}</span>: {sw.setting} = <span class="font-mono">{sw.value}</span>{#if sw.note}<p class="whitespace-pre-wrap text-xs text-muted-foreground">{sw.note}</p>{/if}</li>
						{/each}
					</ul>
				</section>
			{/if}

			{#if view.notes}
				<section class="space-y-1">
					<h2 class="text-lg font-semibold">Notes</h2>
					<p class="whitespace-pre-wrap text-sm text-muted-foreground">{view.notes}</p>
				</section>
			{/if}

			{#if view.sources.length}
				<section class="space-y-2">
					<h2 class="text-lg font-semibold">Sources</h2>
					<ol class="space-y-1 text-sm">
						{#each view.sources as src (src.id)}
							<li id={`src-${src.id}`}>
								<span class="font-mono text-xs text-muted-foreground">{src.id}</span>
								{#if safeUrl(src.url)}<a class="underline" href={safeUrl(src.url)} target="_blank" rel="noopener noreferrer nofollow">{src.title}</a>{:else}{src.title}{/if}{src.author ? ` — ${src.author}` : ''}{src.date ? ` (${src.date})` : ''}
								{#if src.note}<span class="text-xs text-muted-foreground"> {src.note}</span>{/if}
							</li>
						{/each}
					</ol>
				</section>
			{/if}

			<footer class="border-t pt-3 text-xs text-muted-foreground">
				{view.author ? `By ${view.author}` : 'Author not given'}, revision {view.revision}{view.updatedAt ? `, ${when(view.updatedAt)}` : ''}{view.forkedFrom ? `, forked from ${view.forkedFrom}` : ''}.
				Shared under {view.license}.
				<a class="inline-flex items-center gap-1 underline" href={`https://rigs.openrms.dev/p/${encodeURIComponent(view.id)}`} target="_blank" rel="noopener noreferrer">View on openRMS Rigs <ExternalLink class="size-3" /></a>
			</footer>
		{/if}
	{/if}
</div>

<Dialog.Root bind:open={confirmOpen}>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>Apply {toApply.length} setting{toApply.length === 1 ? '' : 's'}?</Dialog.Title>
			<Dialog.Description>rigctlWeb saves the current values first. Restore puts them back.</Dialog.Description>
		</Dialog.Header>
		<ul class="space-y-1 text-sm">
			{#each toApply as s (s.id)}
				<li class="flex items-baseline gap-2">{s.text}{#if s.safety === 'tx'}<Badge variant="destructive">transmit</Badge>{/if}{#if !s.readable}<span class="text-xs text-meter-warn">no restore</span>{/if}</li>
			{/each}
		</ul>
		{#if toApply.some((s) => s.safety === 'tx')}
			<p class="text-sm text-meter-warn">Some of these change how the radio transmits. Check them against your station before going on air.</p>
		{/if}
		{#if catToSend.length}
			<div class="space-y-2 rounded-lg border p-3">
				<p class="text-sm font-medium">Raw CAT commands sent to the radio</p>
				<p class="text-xs text-muted-foreground">
					These menu settings go out as raw CAT, exactly as shown: each is read first (so Restore can put it back), set,
					then read back. All are on the openRMS allow-list.
				</p>
				<ul class="space-y-0.5 font-mono text-sm">
					{#each catToSend as s (s.id)}
						<li>
							<span class="font-sans text-xs text-muted-foreground">{s.name}:</span> read {s.catRead}, send {s.command}, read
							{s.catRead} again
						</li>
					{/each}
				</ul>
				<label class="flex items-center gap-2 text-sm">
					<input type="checkbox" bind:checked={catReviewed} />
					I've checked these commands against my radio's manual
				</label>
			</div>
		{/if}
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (confirmOpen = false)}>Cancel</Button>
			<Button onclick={apply} disabled={applying || keyed || (catToSend.length > 0 && !catReviewed)}>{applying ? 'Applying…' : 'Apply'}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
