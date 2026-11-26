<script lang="ts">
	import { onMount } from 'svelte';
	import { app } from '#lib/stores/rig.svelte';
	import { api, type RigView } from '#lib/api';
	import * as Collapsible from '#lib/components/ui/collapsible';
	import { Button } from '#lib/components/ui/button';
	import { Input } from '#lib/components/ui/input';
	import { Switch } from '#lib/components/ui/switch';
	import { toast } from 'svelte-sonner';
	import { ChevronDown, Trash2, Plus } from '@lucide/svelte';
	import ViewsEditor from '#lib/components/settings/ViewsEditor.svelte';

	// Which sections are open persists per browser.
	const KEY = 'rigctlweb.settings.open';
	function load(): Record<string, boolean> {
		try { return JSON.parse(localStorage.getItem(KEY) ?? '{"radios":true}'); } catch { return { radios: true }; }
	}
	let open = $state<Record<string, boolean>>(load());
	function toggle(id: string, v: boolean) {
		open[id] = v;
		try { localStorage.setItem(KEY, JSON.stringify(open)); } catch { /* ignore */ }
	}

	// A link like /settings#views opens that section and scrolls to it.
	onMount(() => {
		const id = location.hash.slice(1);
		if (!id) return;
		open[id] = true;
		setTimeout(() => document.getElementById(`section-${id}`)?.scrollIntoView({ block: 'start' }), 60);
	});

	let name = $state('');
	let addr = $state('127.0.0.1:4532');
	let readOnly = $state(false);
	let adding = $state(false);

	async function add(e: SubmitEvent) {
		e.preventDefault();
		adding = true;
		try {
			const r = await api.addRig({ name, addr, readOnly });
			toast.success(`Added ${r.name}`);
			name = '';
			await app.refreshRigs();
		} catch (err) {
			toast.error((err as Error).message);
		} finally {
			adding = false;
		}
	}
	async function remove(r: RigView) {
		if (!confirm(`Remove "${r.name}"? The rig is released but rigctld itself is not stopped.`)) return;
		try {
			await api.deleteRig(r.id);
			await app.refreshRigs();
		} catch (err) {
			toast.error((err as Error).message);
		}
	}

	const themes = [
		{ id: 'system', label: 'System' },
		{ id: 'light', label: 'Light' },
		{ id: 'dark', label: 'Dark' },
		{ id: 'red', label: 'Night (red)' }
	] as const;
	async function setTheme(t: (typeof themes)[number]['id']) {
		try { await app.saveSettings({ theme: t }); } catch (e) { toast.error((e as Error).message); }
	}
</script>


<div class="mx-auto max-w-3xl space-y-3">
	<h1 class="mb-2 text-2xl font-semibold tracking-tight">Settings</h1>

	{#each [
		{ id: 'radios', title: 'Radios & connection', desc: 'rigctld endpoints this app controls' },
		{ id: 'views', title: 'Views', desc: 'Choose which controls are shown: Essentials to Everything, or your own' },
		{ id: 'appearance', title: 'Appearance', desc: 'Theme and night mode' },
		{ id: 'access', title: 'Access & security', desc: 'Sign-in, LAN exposure, TLS' },
		{ id: 'about', title: 'About & support', desc: 'Version, license, donations' }
	] as s (s.id)}
		<Collapsible.Root id="section-{s.id}" open={!!open[s.id]} onOpenChange={(v) => toggle(s.id, v)} class="rounded-xl border bg-card">
			<Collapsible.Trigger class="flex w-full items-center justify-between gap-3 px-4 py-3 text-left">
				<span>
					<span class="block font-medium">{s.title}</span>
					<span class="block text-sm text-muted-foreground">{s.desc}</span>
				</span>
				<ChevronDown class="size-4 shrink-0 text-muted-foreground transition-transform {open[s.id] ? 'rotate-180' : ''}" />
			</Collapsible.Trigger>
			<Collapsible.Content class="border-t px-4 py-4">
				{#if s.id === 'radios'}
					<ul class="mb-5 divide-y rounded-lg border">
						{#each app.rigs as r (r.id)}
							<li class="flex items-center gap-3 px-3 py-2.5">
								<span class="size-2 shrink-0 rounded-full {r.state.connected ? 'bg-meter-ok' : 'bg-destructive'}" title={r.state.connected ? 'Connected' : 'Offline'}></span>
								<div class="min-w-0 flex-1">
									<div class="truncate text-sm font-medium">{r.name}{r.readOnly ? ' · read-only' : ''}</div>
									<div class="truncate font-mono text-xs text-muted-foreground">{r.addr}</div>
								</div>
								{#if !app.managed}<Button variant="ghost" size="icon" onclick={() => remove(r)} aria-label="Remove {r.name}"><Trash2 class="size-4" /></Button>{/if}
							</li>
						{:else}
							<li class="px-3 py-4 text-sm text-muted-foreground">{app.managed ? 'hamstack has not given this app any radios.' : 'No radios yet. Add one below.'}</li>
						{/each}
					</ul>
					{#if app.managed}
					<p class="text-sm text-muted-foreground">These radios come from hamstack. Add, remove or allow transmitting on them in hamstack's Apps page, then restart rigctlWeb there.</p>
					{:else}
					<form onsubmit={add} class="grid gap-3 sm:grid-cols-[1fr_1fr_auto]">
						<label class="space-y-1 text-sm">Name<Input bind:value={name} placeholder="FTDX10 (shack)" required maxlength={64} /></label>
						<label class="space-y-1 text-sm">rigctld host:port<Input bind:value={addr} placeholder="10.0.0.5:4532" required pattern={'[A-Za-z0-9.\\-\\[\\]:]+:\\d{1,5}'} class="font-mono" /></label>
						<Button type="submit" class="self-end" disabled={adding}><Plus class="size-4" />Add</Button>
						<label class="flex items-center gap-2 text-sm sm:col-span-3"><Switch bind:checked={readOnly} aria-label="Read-only" />Read-only (monitor only, never changes the radio)</label>
					</form>
					<p class="mt-3 text-xs text-muted-foreground">Start rigctld first, e.g. <code class="rounded bg-muted px-1">rigctld -m 1 -t 4532</code> for the dummy rig, or your radio's model number and serial port.</p>
					{/if}
				{:else if s.id === 'views'}
					<ViewsEditor />
				{:else if s.id === 'appearance'}
					<div class="flex flex-wrap gap-2" role="radiogroup" aria-label="Theme">
						{#each themes as t}
							<Button variant={app.settings.theme === t.id ? 'default' : 'outline'} onclick={() => setTheme(t.id)} role="radio" aria-checked={app.settings.theme === t.id}>{t.label}</Button>
						{/each}
					</div>
					<p class="mt-3 text-xs text-muted-foreground">Night mode uses only red tones to protect your dark adaptation.</p>
				{:else if s.id === 'access' && app.managed}
					<p class="text-sm">rigctlWeb is running inside hamstack. hamstack handles sign-in, and this program only answers requests that come through hamstack.</p>
				{:else if s.id === 'access'}
					<div class="space-y-3 text-sm">
						<p>Listening on <code class="rounded bg-muted px-1 font-mono">{app.settings.listen}</code>. Change it with <code class="rounded bg-muted px-1">--listen</code> or in the config file, then restart.</p>
						<ul class="list-disc space-y-1.5 pl-5 text-muted-foreground">
							<li>The access token lives in the config file (mode 0600). Rotate it by deleting the <code>token</code> line and restarting.</li>
							<li>On a LAN, put a TLS reverse proxy (Caddy, nginx) in front, or reach it over WireGuard or Tailscale.</li>
							<li>Never expose this port directly to the internet. It can key your transmitter.</li>
							<li>Use a read-only rig entry for shared displays.</li>
						</ul>
					</div>
				{:else}
					<div class="space-y-3 text-sm">
						<p>rigctlWeb {app.version}. Free and open source under the MIT license.</p>
						<p>If it's useful to you, a one-time or recurring donation helps cover test radios and development time. It's never required and never unlocks anything.</p>
						<div class="flex flex-wrap gap-2">
							<Button href="https://buymeacoffee.com/ATCUSA" target="_blank" rel="noreferrer" variant="outline">Buy me a coffee</Button>
							<Button href="https://ko-fi.com/ATCUSA" target="_blank" rel="noreferrer" variant="outline">Ko-fi</Button>
						</div>
					</div>
				{/if}
			</Collapsible.Content>
		</Collapsible.Root>
	{/each}
</div>
