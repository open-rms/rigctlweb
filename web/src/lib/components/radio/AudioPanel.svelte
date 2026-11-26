<script lang="ts">
	import { untrack } from 'svelte';
	import { Headphones, Square, TriangleAlert } from '@lucide/svelte';
	import Panel from '#lib/components/ui/panel.svelte';
	import NativeSelect from '#lib/components/ui/native-select.svelte';
	import Meter from '#lib/components/radio/Meter.svelte';
	import { Slider } from '#lib/components/ui/slider';
	import { Switch } from '#lib/components/ui/switch';
	import { api, type AudioConfig, type AudioView } from '#lib/api';
	import { app } from '#lib/stores/rig.svelte';
	import { audio } from '#lib/stores/audio.svelte';
	import { autolevel } from '#lib/stores/autolevel.svelte';

	let info = $state<AudioView | null>(null);
	let loadError = $state('');
	let saving = $state(false);
	const rig = $derived(app.selected);

	// Load this rig's audio settings, and stop listening when the rig changes.
	$effect(() => {
		const id = rig;
		untrack(() => { audio.stop(); autolevel.reset(); });
		info = null;
		loadError = '';
		if (!id) return;
		let alive = true;
		api.audio(id).then((v) => alive && (info = v)).catch((e) => alive && (loadError = e.message));
		return () => { alive = false; audio.stop(); autolevel.reset(); };
	});

	// The input volume is only relevant once the monitor is on.
	$effect(() => {
		const id = rig;
		if (id && info?.config.enabled) untrack(() => void autolevel.refresh(id));
	});

	async function save(patch: Partial<AudioConfig>) {
		if (!rig || !info) return;
		saving = true;
		loadError = '';
		try {
			info = await api.saveAudio(rig, { ...info.config, ...patch });
		} catch (e) {
			loadError = (e as Error).message;
		} finally {
			saving = false;
		}
	}

	const cfg = $derived(info?.config);
	const none = $derived(info?.available.method === 'none');
	const inp = $derived(autolevel.input);
	const canFix = $derived(audio.status === 'listening' && !!inp?.supported && !autolevel.running);
	// The slider shows the live value; the debounce in the store sends it.
	const dbText = $derived(audio.db <= -59.5 ? 'silent' : `${audio.db.toFixed(0)} dB`);
</script>

<Panel id="audio" title="Audio monitor" defaultOpen="wide">
	<div class="space-y-4" data-control="audio">
		{#if loadError}
			<p class="text-sm text-destructive">{loadError}</p>
		{/if}

		{#if !info && !loadError}
			<p class="text-sm text-muted-foreground">Checking this computer's audio…</p>
		{:else if info && cfg}
			{#if none}
				<p class="text-sm text-muted-foreground">
					This computer has no audio capture tool rigctlWeb can use.
					{#each info.available.notes ?? [] as n}{n} {/each}
				</p>
			{:else}
				<label class="flex items-center justify-between gap-3 text-sm">
					<span>
						<span class="font-medium">Listen to this radio in the browser</span>
						<span class="block text-xs text-muted-foreground">
							Receive only. Audio is captured from this computer's sound card only while someone is listening.
						</span>
					</span>
					<Switch checked={cfg.enabled} disabled={saving} onCheckedChange={(on) => save({ enabled: on })} aria-label="Enable audio monitor" />
				</label>

				{#if cfg.enabled}
					<div class="flex items-center gap-3">
						{#if audio.active}
							<button type="button" class="inline-flex items-center gap-2 rounded-md border px-3 py-2 text-sm font-medium hover:bg-accent" onclick={() => audio.stop()}>
								<Square class="size-4" />Stop listening
							</button>
						{:else}
							<button type="button" class="inline-flex items-center gap-2 rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90" onclick={() => rig && audio.start(rig)}>
								<Headphones class="size-4" />Listen
							</button>
						{/if}
						<span class="text-xs text-muted-foreground" aria-live="polite">
							{audio.status === 'connecting' ? 'Connecting…' : audio.status === 'listening' ? 'Listening' : ''}
						</span>
					</div>

					{#if audio.error}
						<p class="flex items-start gap-2 text-sm text-destructive"><TriangleAlert class="mt-0.5 size-4 shrink-0" />{audio.error}</p>
					{/if}

					<Meter
						label="Audio level"
						value={audio.active ? audio.db : undefined}
						min={-60}
						max={0}
						warnAt={-12}
						hotAt={-3}
						display={audio.active ? dbText : undefined}
						ticks={[{ at: -48, label: '-48' }, { at: -24, label: '-24' }, { at: -12, label: '-12' }, { at: 0, label: '0' }]}
					/>
					{#if audio.clipped}
						<div class="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs font-medium text-destructive">
							<span class="flex items-center gap-2"><TriangleAlert class="size-4" />Audio is clipping. Lower the radio's output or the sound card's input level.</span>
							{#if canFix && rig}
								<button type="button" class="rounded-md border border-destructive/40 px-2 py-1 hover:bg-destructive/10" onclick={() => autolevel.run(rig)}>Fix automatically</button>
							{/if}
						</div>
					{/if}

					<div class="space-y-1.5">
						<div class="flex items-center justify-between text-xs">
							<span class="font-medium text-muted-foreground">Volume (this browser only)</span>
							<span class="font-mono tabular-nums">{Math.round(audio.volume * 100)}%</span>
						</div>
						<Slider type="single" value={audio.volume} min={0} max={1} step={0.01} onValueChange={(v) => audio.setVolume(v)} aria-label="Monitor volume" />
					</div>
				{/if}

				{#if cfg.enabled}
					<div class="space-y-3 rounded-lg border p-3" data-control="audio.input">
						<div class="text-sm font-medium">Input level</div>

						{#if inp && !inp.supported}
							<p class="text-xs text-muted-foreground">
								{inp.reason} If the audio clips, lower the radio's USB or audio output level in its menu.
							</p>
						{:else if inp}
							<div class="space-y-1.5">
								<div class="flex items-center justify-between text-xs">
									<span class="font-medium text-muted-foreground">Sound card input (manual)</span>
									<span class="font-mono tabular-nums">{Math.round(inp.volume * 100)}%</span>
								</div>
								<Slider type="single" value={inp.volume} min={0.05} max={1} step={0.01} disabled={autolevel.running} onValueChange={(v) => rig && autolevel.setVolume(rig, v)} aria-label="Sound card input level" />
								<p class="text-xs text-muted-foreground">Lower it until the meter peaks below 0 dB without touching the right edge. It can only be lowered, never raised above 100%.</p>
							</div>

							<div class="flex flex-wrap items-center gap-2">
								<button type="button" class="rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
									disabled={!canFix} onclick={() => rig && autolevel.run(rig)}>Auto-adjust</button>
								<label class="flex items-center gap-1.5 text-xs text-muted-foreground">
									Listen for
									<NativeSelect class="h-8 w-auto" bind:value={autolevel.seconds} disabled={autolevel.running}>
										{#each [2, 3, 4, 5] as n}<option value={n}>{n} s</option>{/each}
									</NativeSelect>
								</label>
								{#if autolevel.running}
									<button type="button" class="rounded-md border px-2.5 py-1.5 text-xs font-medium hover:bg-accent" onclick={() => autolevel.cancel()}>Cancel</button>
								{/if}
								{#if inp.original !== undefined && Math.abs(inp.original - inp.volume) > 0.005}
									<button type="button" class="rounded-md border px-2.5 py-1.5 text-xs font-medium hover:bg-accent disabled:opacity-50" disabled={autolevel.running}
										onclick={() => rig && autolevel.restore(rig)}>Restore original ({Math.round(inp.original * 100)}%)</button>
								{/if}
							</div>
							{#if audio.status !== 'listening'}
								<p class="text-xs text-muted-foreground">Press Listen first. Auto-adjust measures the audio you are hearing.</p>
							{/if}

							{#if autolevel.running}
								<div class="h-1.5 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(autolevel.progress * 100)}>
									<div class="h-full bg-primary transition-[width] duration-75" style="width:{autolevel.phase === 'adjusting' ? 100 : autolevel.progress * 100}%"></div>
								</div>
							{/if}
							<p class="text-xs {autolevel.phase === 'failed' ? 'text-destructive' : 'text-muted-foreground'}" aria-live="polite">{autolevel.message}</p>

							<p class="text-xs text-muted-foreground">
								This is the computer's own input volume, so other programs that use the same input (pat, WSJT-X, Dire Wolf) hear the change too.
								Auto-adjust uses the loudest peak it hears in those few seconds, so a stronger signal later can still clip.
							</p>
						{:else if !autolevel.error}
							<p class="text-xs text-muted-foreground">Checking the input level…</p>
						{/if}
						{#if autolevel.error}<p class="text-xs text-destructive">{autolevel.error}</p>{/if}
					</div>
				{/if}

				<div class="grid gap-3 sm:grid-cols-2">
					<label class="space-y-1 text-xs font-medium text-muted-foreground">
						Sound input
						<NativeSelect value={cfg.device} disabled={saving} onchange={(e) => save({ device: e.currentTarget.value })}>
							<option value="">Automatic (first found)</option>
							{#each info.available.devices as d (d.id)}
								<option value={d.id}>{d.name}</option>
							{/each}
						</NativeSelect>
					</label>
					<label class="space-y-1 text-xs font-medium text-muted-foreground">
						Channel
						<NativeSelect value={cfg.channel} disabled={saving} onchange={(e) => save({ channel: e.currentTarget.value as AudioConfig['channel'] })}>
							<option value="mix">Mix (left + right)</option>
							<option value="left">Left only</option>
							<option value="right">Right only</option>
						</NativeSelect>
					</label>
				</div>
				<p class="text-xs text-muted-foreground">
					Capture method: {info.available.method}. {#each info.available.notes ?? [] as n}{n} {/each}
				</p>
			{/if}
		{/if}
	</div>
</Panel>
