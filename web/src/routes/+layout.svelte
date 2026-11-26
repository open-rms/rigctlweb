<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { asset, resolve } from '$app/paths';
	import { app } from '#lib/stores/rig.svelte';
	import { Button } from '#lib/components/ui/button';
	import { Input } from '#lib/components/ui/input';
	import { Badge } from '#lib/components/ui/badge';
	import { Toaster } from '#lib/components/ui/sonner';
	import { Radio, Settings, LogOut, Antenna, Search, Plug, SlidersHorizontal } from '@lucide/svelte';
	import SearchPalette from '#lib/components/SearchPalette.svelte';
	import { searchUI } from '#lib/stores/search.svelte';
	import { views } from '#lib/stores/views.svelte';
	import * as Tooltip from '#lib/components/ui/tooltip';

	let { children } = $props();
	let token = $state('');
	let loginErr = $state('');
	let busy = $state(false);

	onMount(() => app.init());

	// Apply theme class to <html>; "system" follows the OS.
	$effect(() => {
		const t = app.settings.theme;
		const mq = matchMedia('(prefers-color-scheme: dark)');
		const apply = () => {
			const el = document.documentElement;
			el.classList.remove('dark', 'red');
			if (t === 'dark' || (t === 'system' && mq.matches)) el.classList.add('dark');
			if (t === 'red') el.classList.add('red');
		};
		apply();
		mq.addEventListener('change', apply);
		return () => mq.removeEventListener('change', apply);
	});

	async function login(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		loginErr = '';
		try {
			await app.login(token.trim());
			token = '';
		} catch (err) {
			loginErr = (err as Error).message || 'Sign-in failed';
		} finally {
			busy = false;
		}
	}

	const nav = [
		{ href: resolve('/'), label: 'Radio', icon: Radio },
		{ href: resolve('/presets'), label: 'Presets', icon: SlidersHorizontal },
		{ href: resolve('/integrations'), label: 'Integrations', icon: Plug },
		{ href: resolve('/settings'), label: 'Settings', icon: Settings }
	];
</script>

<svelte:head><title>rigctlWeb</title></svelte:head>
<Toaster position="bottom-right" theme={app.settings.theme === 'light' ? 'light' : app.settings.theme === 'system' ? 'system' : 'dark'} />

{#if !app.ready}
	<div class="grid min-h-dvh place-items-center text-muted-foreground">Loading…</div>
{:else if !app.authed}
	<main class="grid min-h-dvh place-items-center p-4">
		<form onsubmit={login} class="w-full max-w-sm space-y-4 rounded-xl border bg-card p-6 shadow-sm">
			<div>
				<h1 class="text-xl font-semibold">rigctlWeb</h1>
				<p class="text-sm text-muted-foreground">Enter the access token printed when the server started, or stored in its config file.</p>
			</div>
			<Input type="password" bind:value={token} placeholder="Access token" autocomplete="current-password" required aria-label="Access token" />
			{#if loginErr}<p class="text-sm text-destructive" role="alert">{loginErr}</p>{/if}
			<Button type="submit" class="w-full" disabled={busy || !token}>{busy ? 'Signing in…' : 'Sign in'}</Button>
		</form>
	</main>
{:else}
	<Tooltip.Provider delayDuration={150}>
	<div class="mx-auto flex min-h-dvh max-w-[100rem] flex-col px-3 sm:px-6">
		<header class="flex flex-wrap items-center gap-x-2 gap-y-2 py-3">
			<a href={resolve('/')} class="mr-1 flex items-center gap-2 text-lg font-semibold tracking-tight"><img src={asset('favicon.svg')} alt="" class="size-7 rounded-md" width="28" height="28" /><span class="hidden min-[380px]:inline">rigctl<span class="text-primary">Web</span></span></a>
			<nav class="flex gap-1" aria-label="Main">
				{#each nav as n}
					{@const Icon = n.icon}
					<a
						href={n.href}
						aria-current={page.url.pathname === n.href ? 'page' : undefined}
						aria-label={n.label}
						class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-sm font-medium transition-colors hover:bg-accent sm:px-3 {page.url.pathname === n.href ? 'bg-accent text-accent-foreground' : 'text-muted-foreground'}"
					><Icon class="size-4" /><span class="hidden sm:inline">{n.label}</span></a>
				{/each}
			</nav>
			<div class="ml-auto flex items-center gap-1.5">
				<span class="inline-flex h-7 items-center gap-1.5 text-xs text-muted-foreground" role="status" aria-live="polite">
					{#if app.busy}
						<Antenna class="antenna-busy size-5" aria-hidden="true" /><span class="hidden md:inline">Applying…</span><span class="sr-only md:hidden">Applying changes</span>
					{/if}
				</span>
				<button type="button" class="inline-flex h-9 items-center gap-2 rounded-md border bg-card px-2.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground" onclick={() => searchUI.show()} aria-label="Search controls and settings" title="Search (press /)">
					<Search class="size-4" /><span class="hidden lg:inline">Search</span><kbd class="hidden rounded border px-1 text-[10px] lg:inline">/</kbd>
				</button>
				{#if app.selected}
					{#if app.online}
						<Badge variant="secondary" class="gap-1.5" title="Connected"><span class="size-2 rounded-full bg-meter-ok"></span><span class="hidden md:inline">Connected</span></Badge>
					{:else}
						<Badge variant="destructive" class="gap-1.5"><span class="size-2 animate-pulse rounded-full bg-white"></span><span class="hidden md:inline">{app.wsUp ? 'Rig offline' : 'Reconnecting'}</span></Badge>
					{/if}
				{/if}
				{#if !app.managed}<Button variant="ghost" size="icon" onclick={() => app.logout()} aria-label="Sign out"><LogOut class="size-4" /></Button>{/if}
			</div>
			<!-- Pickers drop to their own full-width row on narrow screens. -->
			<div class="order-last flex w-full gap-2 sm:order-none sm:w-auto">
				<select class="min-w-0 flex-1 rounded-md border bg-card px-2 py-1.5 text-sm sm:flex-none" value={views.activeId} onchange={(e) => views.setActive(e.currentTarget.value)} aria-label="Interface view" title="Choose how much of the radio to show">
					{#each views.all as v}<option value={v.id}>{v.name}</option>{/each}
				</select>
				{#if app.rigs.length > 1}
					<select class="min-w-0 flex-1 rounded-md border bg-card px-2 py-1.5 text-sm sm:flex-none" value={app.selected} onchange={(e) => app.select(e.currentTarget.value)} aria-label="Select radio">
						{#each app.rigs as r}<option value={r.id}>{r.name}</option>{/each}
					</select>
				{/if}
			</div>
		</header>
		<main class="flex-1 pb-8">{@render children()}</main>
		<SearchPalette />
		<footer class="flex flex-wrap items-center justify-between gap-2 border-t py-3 text-xs text-muted-foreground">
			<span>rigctlWeb {app.version} · open source (MIT)</span>
			<span>Like it? <a class="underline hover:text-foreground" href="https://buymeacoffee.com/ATCUSA" target="_blank" rel="noreferrer">Buy me a coffee</a> or <a class="underline hover:text-foreground" href="https://ko-fi.com/ATCUSA" target="_blank" rel="noreferrer">Ko-fi</a> — one-time or recurring, never required.</span>
		</footer>
	</div>
	</Tooltip.Provider>
{/if}
