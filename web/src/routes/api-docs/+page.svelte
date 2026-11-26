<script lang="ts">
	import { onMount } from 'svelte';
	import { app } from '#lib/stores/rig.svelte';
	import { apiPath } from '#lib/paths';

	// Interactive reference for the published API, rendered by Scalar from the server's own
	// OpenAPI document. Loaded only on this page, bundled into the binary (no CDN, works offline).
	let el: HTMLDivElement;
	let failed = $state('');

	onMount(() => {
		let destroy: (() => void) | undefined;
		let cancelled = false;
		(async () => {
			try {
				const [{ createApiReference }] = await Promise.all([
					import('@scalar/api-reference'),
					import('@scalar/api-reference/style.css')
				]);
				if (cancelled) return;
				const dark = app.settings.theme !== 'light' &&
					(app.settings.theme !== 'system' || matchMedia('(prefers-color-scheme: dark)').matches);
				const ref = createApiReference(el, {
					url: apiPath('/api/v1/openapi.json'),
					// Everything stays on this server: no telemetry, no hosted extras, no web fonts.
					telemetry: false,
					withDefaultFonts: false,
					showDeveloperTools: 'never',
					mcp: { disabled: true },
					agent: { disabled: true }, // hosted "Ask AI": would send data off this server
					hideDarkModeToggle: true,
					forceDarkModeState: dark ? 'dark' : 'light',
					persistAuth: false,
					theme: 'default'
				});
				destroy = () => ref.destroy?.();
			} catch (err) {
				failed = (err as Error).message || 'could not load the API reference';
			}
		})();
		return () => {
			cancelled = true;
			destroy?.();
		};
	});
</script>

<div class="mx-auto max-w-[100rem] space-y-3">
	<div class="max-w-4xl">
		<h1 class="text-2xl font-semibold tracking-tight">API</h1>
		<p class="mt-1 text-sm text-muted-foreground">
			The published, versioned API (v1) for your own programs, scripts and dashboards. Send the access token as
			<code class="rounded bg-muted px-1">Authorization: Bearer &lt;token&gt;</code>. Download the
			<a class="underline" href={apiPath('/api/v1/openapi.json')} download="rigctlweb-openapi.json">OpenAPI document</a>
			to generate a client in any language. v1 never keys the transmitter.
		</p>
	</div>
	{#if failed}<p class="text-sm text-destructive" role="alert">The API reference could not load: {failed}</p>{/if}
	<div bind:this={el} class="api-docs overflow-hidden rounded-xl border"></div>
</div>
