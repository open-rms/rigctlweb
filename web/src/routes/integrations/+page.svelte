<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { app } from '#lib/stores/rig.svelte';
	import { INTEGRATIONS, render, type Integration } from '#lib/integrations';
	import { Button } from '#lib/components/ui/button';
	import { Input } from '#lib/components/ui/input';
	import { Badge } from '#lib/components/ui/badge';
	import NativeSelect from '#lib/components/ui/native-select.svelte';
	import { toast } from 'svelte-sonner';
	import { Copy, ExternalLink, CircleCheck, CircleAlert, CircleHelp } from '@lucide/svelte';

	// The address other programs should use to reach rigctld. Defaults to the selected rig's
	// own rigctld address; edit it when the program runs on a different machine.
	const rig = $derived(app.rigs.find((r) => r.id === app.selected));
	let host = $state('127.0.0.1');
	let port = $state(4532);
	let seeded = '';
	$effect(() => {
		if (!rig || seeded === rig.id) return;
		seeded = rig.id;
		const i = rig.addr.lastIndexOf(':');
		host = rig.addr.slice(0, i) || '127.0.0.1';
		port = Number(rig.addr.slice(i + 1)) || 4532;
	});

	let category = $state<'all' | Integration['category']>('all');
	let q = $state('');
	let openId = $state<string | null>(null);

	const cats: { id: 'all' | Integration['category']; label: string }[] = [
		{ id: 'all', label: 'All' }, { id: 'digital', label: 'Digital modes' }, { id: 'winlink', label: 'Winlink' },
		{ id: 'packet', label: 'Packet' }, { id: 'modem', label: 'Modems' }, { id: 'logging', label: 'Logging' }, { id: 'other', label: 'Other' }
	];
	const shown = $derived(
		INTEGRATIONS.filter((i) => (category === 'all' || i.category === category) &&
			(!q.trim() || `${i.name} ${i.summary} ${i.category}`.toLowerCase().includes(q.trim().toLowerCase())))
	);
	const rendered = (i: Integration) => render(i, { host, port, rigId: rig?.id ?? 'my_rig' });

	const conf = {
		verified: { label: 'Verified', tip: 'Confirmed in the program\'s own documentation or source code.', icon: CircleCheck, cls: 'text-meter-ok' },
		partial: { label: 'Partly verified', tip: 'Pieced together from related documentation or source; check it against your version.', icon: CircleAlert, cls: 'text-meter-warn' },
		unverified: { label: 'Unverified', tip: 'Could not be confirmed. Treat as a starting point.', icon: CircleHelp, cls: 'text-muted-foreground' }
	} as const;
	const pttText = { cat: 'Keys over CAT (the program\'s own PTT setting)', rigctld: 'Keys through rigctld', separate: 'Keys the radio itself, not through rigctld', unknown: 'PTT method not confirmed' } as const;

	async function copy(text: string, what: string) {
		try {
			await navigator.clipboard.writeText(text);
			toast.success(`${what} copied`);
		} catch {
			toast.error('Copy failed. Select the text and copy it by hand.');
		}
	}

	onMount(() => {
		const id = location.hash.slice(1);
		if (!id) return;
		openId = id;
		setTimeout(() => document.getElementById(`app-${id}`)?.scrollIntoView({ block: 'start' }), 60);
	});
</script>

<div class="mx-auto max-w-4xl space-y-5">
	<div>
		<h1 class="text-2xl font-semibold tracking-tight">Integrations</h1>
		<p class="mt-1 text-sm text-muted-foreground">
			Point your other ham programs at the same radio. They all talk to rigctld, so they can share it with rigctlWeb. Pick a program for exact steps and a ready-to-paste snippet.
		</p>
		<p class="mt-2 text-sm text-muted-foreground">Writing your own program? See the <a class="underline" href={resolve('/api-docs')}>API reference</a>.</p>
	</div>

	<div class="grid gap-3 rounded-xl border bg-card p-4 sm:grid-cols-[1fr_8rem_auto] sm:items-end">
		<label class="space-y-1 text-sm">rigctld host
			<Input bind:value={host} placeholder="127.0.0.1" class="font-mono" autocomplete="off" />
		</label>
		<label class="space-y-1 text-sm">Port
			<Input type="number" bind:value={port} min={1} max={65535} class="font-mono" />
		</label>
		<div class="pb-1.5 text-sm" role="status">
			{#if !rig}<span class="text-muted-foreground">No radio added yet.</span>
			{:else if app.online && app.state?.connected}<span class="inline-flex items-center gap-1.5"><span class="size-2 rounded-full bg-meter-ok"></span>rigctld answering{app.caps ? `: ${app.caps.mfg} ${app.caps.model}` : ''}</span>
			{:else}<span class="inline-flex items-center gap-1.5 text-destructive"><span class="size-2 rounded-full bg-destructive"></span>rigctld not reachable</span>{/if}
		</div>
		<p class="text-xs text-muted-foreground sm:col-span-3">
			Use <code class="rounded bg-muted px-1">127.0.0.1</code> for programs on the same computer as rigctld, or this machine's LAN address for programs elsewhere (start rigctld with <code class="rounded bg-muted px-1">-T 0.0.0.0</code> to allow that, and only on a trusted network).
			{#if app.rigs.length > 1}
				<span class="mt-2 block max-w-xs"><NativeSelect value={app.selected ?? ''} onchange={(e) => app.select(e.currentTarget.value)} aria-label="Radio">{#each app.rigs as r}<option value={r.id}>{r.name}</option>{/each}</NativeSelect></span>
			{/if}
		</p>
	</div>

	<div class="rounded-lg border border-meter-warn/50 bg-meter-warn/10 p-3 text-sm">
		<strong class="font-medium">Sharing one radio:</strong> several programs polling the same serial port slow everything down. Give only rigctld the serial port, keep each program's polling interval modest, and let one program own PTT.
	</div>

	<div class="flex flex-wrap items-center gap-2">
		<div role="group" aria-label="Filter by type" class="flex flex-wrap gap-1.5">
			{#each cats as c}
				<Button size="sm" variant={category === c.id ? 'default' : 'outline'} aria-pressed={category === c.id} onclick={() => (category = c.id)}>{c.label}</Button>
			{/each}
		</div>
		<Input bind:value={q} placeholder="Filter programs" aria-label="Filter programs" class="ml-auto w-full sm:w-56" />
	</div>

	<ul class="space-y-3">
		{#each shown as i (i.id)}
			{@const r = rendered(i)}
			{@const c = conf[i.confidence]}
			{@const CIcon = c.icon}
			<li id="app-{i.id}" class="rounded-xl border bg-card">
				<button type="button" class="flex w-full items-start gap-3 px-4 py-3 text-left" aria-expanded={openId === i.id} onclick={() => (openId = openId === i.id ? null : i.id)}>
					<span class="min-w-0 flex-1">
						<span class="flex flex-wrap items-center gap-2">
							<span class="font-medium">{i.name}</span>
							<Badge variant="secondary" class="capitalize">{i.category}</Badge>
							<span class="inline-flex items-center gap-1 text-xs {c.cls}" title={c.tip}><CIcon class="size-3.5" />{c.label}</span>
						</span>
						<span class="mt-0.5 block text-sm text-muted-foreground">{i.summary}</span>
					</span>
				</button>
				{#if openId === i.id}
					<div class="space-y-4 border-t px-4 py-4">
						<p class="text-xs text-muted-foreground">{pttText[i.ptt]}</p>
						<ol class="list-decimal space-y-1.5 pl-5 text-sm">
							{#each r.steps as step}<li>{step}</li>{/each}
						</ol>
						{#if r.snippet}
							<div class="overflow-hidden rounded-lg border">
								<div class="flex items-center justify-between bg-muted px-3 py-1.5 text-xs text-muted-foreground">
									<span class="font-mono">{i.snippetFile ?? 'snippet'}</span>
									<Button size="xs" variant="ghost" onclick={() => copy(r.snippet!, 'Snippet')}><Copy />Copy</Button>
								</div>
								<pre class="overflow-x-auto p-3 font-mono text-xs leading-relaxed"><code>{r.snippet}</code></pre>
							</div>
						{/if}
						{#if i.notes.length}
							<div>
								<div class="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Notes</div>
								<ul class="list-disc space-y-1 pl-5 text-sm text-muted-foreground">{#each i.notes as n}<li>{n}</li>{/each}</ul>
							</div>
						{/if}
						{#if i.sources.length}
							<div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
								<span>Sources:</span>
								{#each i.sources as s}<a class="inline-flex items-center gap-1 underline hover:text-foreground" href={s} target="_blank" rel="noreferrer noopener">{new URL(s).hostname}<ExternalLink class="size-3" /></a>{/each}
							</div>
						{/if}
					</div>
				{/if}
			</li>
		{:else}
			<li class="rounded-xl border p-6 text-center text-sm text-muted-foreground">No programs match.</li>
		{/each}
	</ul>
	<p class="text-xs text-muted-foreground">Not covered yet: GridTracker, Log4OM and N1MM+ (no clear primary documentation of rigctld networking was found). Details for each program also live in <code class="rounded bg-muted px-1">docs/integrations.md</code>.</p>
</div>
