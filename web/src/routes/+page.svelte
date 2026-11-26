<script lang="ts">
	import { resolve } from '$app/paths';
	import { app } from '#lib/stores/rig.svelte';
	import { BANDS, bandOf, sUnits, sliderLevels, sortModes, LEVEL_LABELS, FUNC_LABELS } from '#lib/radio';
	import FrequencyDisplay from '#lib/components/radio/FrequencyDisplay.svelte';
	import StickyBar from '#lib/components/radio/StickyBar.svelte';
	import DynamicPanels from '#lib/components/radio/DynamicPanels.svelte';
	import AudioPanel from '#lib/components/radio/AudioPanel.svelte';
	import MemoryPanel from '#lib/components/radio/MemoryPanel.svelte';
	import { hasMemories } from '#lib/views';
	import Meter from '#lib/components/radio/Meter.svelte';
	import { Button } from '#lib/components/ui/button';
	import { Slider } from '#lib/components/ui/slider';
	import { Switch } from '#lib/components/ui/switch';
	import * as Card from '#lib/components/ui/card';
	import Panel from '#lib/components/ui/panel.svelte';
	import { toast } from 'svelte-sonner';
	import { views } from '#lib/stores/views.svelte';
	import Help from '#lib/components/ui/help.svelte';
	import { helpFor } from '#lib/help';
	import { Mic, ArrowLeftRight, Copy, Waypoints } from '@lucide/svelte';

	let freqDisplay: FrequencyDisplay | undefined = $state();
	let operateEl: HTMLElement | undefined = $state();
	const st = $derived(app.state);
	const caps = $derived(app.caps);
	const live = $derived(app.online && !!st);
	const locked = $derived(!live || app.readOnly);

	async function run(cmd: string, args: Record<string, unknown> = {}) {
		try {
			await app.send(cmd, args);
		} catch (e) {
			toast.error((e as Error).message, { id: 'rig-command-error' });
		}
	}

	// ---- band memory: remember last frequency+mode per band (per browser) ----
	type Mem = Record<string, { hz: number; mode: string }>;
	function loadMem(): Mem {
		try { return JSON.parse(localStorage.getItem('rigctlweb.bands') ?? '{}'); } catch { return {}; }
	}
	let bandMem = $state<Mem>(loadMem());
	$effect(() => {
		if (!st?.connected) return;
		const b = bandOf(st.freq);
		if (!b) return;
		if (bandMem[b.name]?.hz === st.freq && bandMem[b.name]?.mode === st.mode) return;
		bandMem[b.name] = { hz: st.freq, mode: st.mode };
		try { localStorage.setItem('rigctlweb.bands', JSON.stringify(bandMem)); } catch { /* ignore */ }
	});
	const currentBand = $derived(st ? bandOf(st.freq) : undefined);
	async function gotoBand(name: string) {
		const b = BANDS.find((x) => x.name === name)!;
		const m = bandMem[name] ?? { hz: b.def, mode: b.mode };
		await run('set_mode', { mode: caps?.modes.includes(m.mode) ? m.mode : b.mode, passband: 0 });
		await run('set_freq', { hz: m.hz });
	}

	// ---- derived UI model ----
	// ---- antenna tuner ----
	// A tune cycle transmits a carrier, so it takes two deliberate clicks within 5 s.
	const canTune = $derived(!!caps?.vfoOps.includes('TUNE'));
	const hasTunerSwitch = $derived(!!caps?.setFuncs.includes('TUNER'));
	let tuneArmed = $state(false);
	let tuneBusy = $state(false);
	let armTimer: ReturnType<typeof setTimeout> | undefined;
	function armTune() {
		tuneArmed = true;
		clearTimeout(armTimer);
		armTimer = setTimeout(() => (tuneArmed = false), 5000);
	}
	// A tune cycle is watched live: carrier detected from PTT, finished after the radio stops
	// transmitting, and judged by the FINAL SWR (a tuner always shows a high SWR while it searches).
	type TuneNote = { kind: 'ok' | 'warn' | 'bad' | 'info'; text: string };
	let tunePhase = $state<'idle' | 'starting' | 'tuning'>('idle');
	let tuneNote = $state<TuneNote | null>(null);
	let tuneStarted = 0;
	let sawTx = false;
	let quiet = 0;
	let peakSwr = 0;
	let lastSwr = 0;
	let tuneWatchdog: ReturnType<typeof setTimeout> | undefined;

	function finishTune(timedOut = false) {
		clearTimeout(tuneWatchdog);
		const had = sawTx;
		tunePhase = 'idle';
		tuneBusy = false;
		if (timedOut) tuneNote = { kind: 'bad', text: 'Tune ran too long; the carrier was dropped. Check the antenna and tuner.' };
		else if (!had) tuneNote = { kind: 'info', text: "Tune command sent, but the radio never reported transmitting, so the result can't be judged. Check the tuner on the radio." };
		else if (!lastSwr) tuneNote = { kind: 'info', text: 'Tune finished. This radio gave no SWR reading, so check the result on the radio.' };
		else if (lastSwr < 2) tuneNote = { kind: 'ok', text: `Tuned: settled at ${lastSwr.toFixed(1)}:1${peakSwr > lastSwr + 0.5 ? ` (peaked at ${peakSwr.toFixed(1)} while searching, which is normal)` : ''}.` };
		else if (lastSwr < 3) tuneNote = { kind: 'warn', text: `Tuned, but SWR settled at ${lastSwr.toFixed(1)}:1. It works; lower would be better.` };
		else tuneNote = { kind: 'bad', text: `The tuner could not find a match (SWR ${lastSwr.toFixed(1)}:1). Check the antenna and cable.` };
	}

	$effect(() => {
		if (tunePhase === 'idle' || !st) return;
		const swr = st.levels['SWR'] ?? 0;
		if (st.ptt) {
			sawTx = true;
			quiet = 0;
			tunePhase = 'tuning';
			if (swr > 0) { lastSwr = swr; peakSwr = Math.max(peakSwr, swr); }
		} else if (sawTx) {
			if (++quiet >= 2) finishTune();
		} else if (Date.now() - tuneStarted > 5000) {
			finishTune();
		}
	});

	async function startTune() {
		clearTimeout(armTimer);
		tuneArmed = false;
		tuneBusy = true;
		tuneNote = null;
		sawTx = false; quiet = 0; peakSwr = 0; lastSwr = 0;
		tuneStarted = Date.now();
		tunePhase = 'starting';
		clearTimeout(tuneWatchdog);
		tuneWatchdog = setTimeout(() => finishTune(true), 35000);
		try {
			await app.send('vfo_op', { op: 'TUNE' });
		} catch (e) {
			clearTimeout(tuneWatchdog);
			tunePhase = 'idle';
			tuneBusy = false;
			toast.error((e as Error).message, { id: 'rig-command-error' });
		}
	}
	// Dropping PTT ends the carrier immediately.
	const stopTune = () => run('set_ptt', { on: false });

	const modes = $derived(caps ? sortModes(caps.modes) : []);
	const sliders = $derived(caps ? sliderLevels(caps.levels) : []);
	const funcs = $derived(caps ? caps.setFuncs.filter((f) => !['RIT', 'XIT', 'TUNER', 'TRANSCEIVE', 'SPECTRUM', 'SPECTRUM_HOLD', 'SCOPE', 'SEND_MORSE', 'SEND_VOICE_MEM', 'OVF_STATUS', 'SYNC', 'RESUME', 'SATMODE'].includes(f)) : []);
	const has = (n: string) => !!caps?.levels.some((l) => l.name === n && l.get);
	const lv = (n: string) => st?.levels[n];
	const show = (id: string) => views.shows(id);
	const slidersShown = $derived(sliders.filter((l) => show('lvl:' + l.name)));
	const funcsShown = $derived(funcs.filter((f) => show('fn:' + f)));
	const modeCardVisible = $derived(show('mode') || (!!caps && caps.vfos.length > 1 && (show('vfo') || show('split'))) || (!!caps && ((caps.maxRit > 0 && show('rit')) || (caps.maxXit > 0 && show('xit')))));

	const sm = $derived(lv('STRENGTH') === undefined ? undefined : sUnits(lv('STRENGTH')!));

	// Sliders: show the live value unless the user is dragging.
	let drag = $state<Record<string, number>>({});
	let sendTimer: Record<string, ReturnType<typeof setTimeout>> = {};
	function setLevel(name: string, v: number) {
		drag[name] = v;
		clearTimeout(sendTimer[name]);
		sendTimer[name] = setTimeout(async () => {
			await run('set_level', { name, value: v });
			delete drag[name];
		}, 60);
	}
	const shown = (n: string) => drag[n] ?? st?.levels[n] ?? 0;
	const fmtLevel = (l: { max: number }, v: number) => (l.max <= 1 ? `${Math.round(v * 100)}%` : String(Math.round(v)));

</script>

<svelte:window
	onkeydown={(e) => {
		const t = e.target as HTMLElement | null;
		if (e.key.toLowerCase() === 'f' && !e.ctrlKey && !e.metaKey && !e.altKey && !/^(INPUT|TEXTAREA|SELECT)$/.test(t?.tagName ?? '') && !t?.isContentEditable) {
			e.preventDefault();
			freqDisplay?.startEdit();
		}
	}}
/>

{#if !app.selected}
	<Card.Root class="mx-auto mt-10 max-w-lg text-center">
		<Card.Header><Card.Title>No radio configured</Card.Title><Card.Description>Add a rigctld connection to get started.</Card.Description></Card.Header>
		<Card.Content><Button href={resolve('/settings')}>Open settings</Button></Card.Content>
	</Card.Root>
{:else}
	{#if !app.online}
		<div class="mb-4 rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive" role="status">
			Can't reach rigctld{st?.error ? `: ${st.error}` : ''}. Retrying…
		</div>
	{/if}
	{#if app.readOnly}
		<div class="mb-4 rounded-lg border bg-muted p-3 text-sm text-muted-foreground" role="status">This radio is read-only; controls are disabled.</div>
	{/if}

	<StickyBar target={operateEl} onptt={() => run('set_ptt', { on: !st?.ptt })} />

	<div class="space-y-4">
		<div bind:this={operateEl}>
			<Card.Root class={st?.ptt ? "ring-2 ring-tx" : ""}>
				<Card.Content class="space-y-4 pt-6">
					<div class="flex flex-wrap items-center justify-between gap-2">
						<div class="text-sm text-muted-foreground">
							{#if caps}{caps.mfg} {caps.model}{:else}…{/if}
							{#if currentBand}<span class="ml-2 rounded bg-secondary px-1.5 py-0.5 text-xs font-medium text-secondary-foreground">{currentBand.name}</span>{/if}
						</div>
						{#if st?.ptt}<span class="animate-pulse rounded-md bg-tx px-2 py-0.5 text-xs font-bold tracking-widest text-white">TX</span>{:else if live}<span class="text-xs tracking-widest text-muted-foreground">RX</span>{/if}
					</div>

					{#if show('freq')}
					<div data-control="freq">
					<FrequencyDisplay bind:this={freqDisplay} hz={st?.freq ?? 0} label={st?.vfo?.replace('VFO', '') ?? ''} disabled={locked} onchange={(hz) => run('set_freq', { hz })} />
					</div>
					{#if st?.split && st.txFreq}
						<FrequencyDisplay hz={st.txFreq} label="TX" active={false} disabled={locked} onchange={(hz) => run('set_split_freq', { hz })} />
					{/if}
					{/if}

					<!-- Meters -->
					{#if (!st?.ptt && show('smeter') && has('STRENGTH')) || (st?.ptt && (show('meter.power') || show('meter.swr') || show('meter.alc') || show('meter.comp') || show('meter.temp')))}
					<div class="grid gap-4 sm:grid-cols-2" data-control="smeter meter.power meter.swr meter.alc meter.comp meter.temp">
						{#if !st?.ptt}
							{#if show('smeter') && has('STRENGTH')}
							<div class="sm:col-span-2">
								<Meter label="Signal" value={lv('STRENGTH')} min={-54} max={60} display={sm?.label}
									ticks={[{ at: -54, label: 'S1' }, { at: -30, label: 'S5' }, { at: 0, label: 'S9' }, { at: 20, label: '+20' }, { at: 40, label: '+40' }, { at: 60, label: '+60' }]}
									warnAt={0} hotAt={40} height="h-4" help="smeter" />
							</div>
							{/if}
						{:else}
							{#if show('meter.power')}
								{#if has('RFPOWER_METER_WATTS')}
									<Meter label="Power" value={lv('RFPOWER_METER_WATTS')} min={0} max={Math.max(100, lv('RFPOWER_METER_WATTS') ?? 0)} display={lv('RFPOWER_METER_WATTS')?.toFixed(0)} unit=" W" help="meter.power" />
								{:else if has('RFPOWER_METER')}
									<Meter label="Power" value={lv('RFPOWER_METER')} min={0} max={1} display={`${Math.round((lv('RFPOWER_METER') ?? 0) * 100)}`} unit="%" help="meter.power" />
								{/if}
							{/if}
							{#if has('SWR') && show('meter.swr')}<Meter label="SWR" value={lv('SWR')} min={1} max={3} display={lv('SWR')?.toFixed(1)} unit=":1" warnAt={1.8} hotAt={2.5} help="meter.swr" />{/if}
							{#if has('ALC') && show('meter.alc')}<Meter label="ALC" value={lv('ALC')} min={0} max={1} display={`${Math.round((lv('ALC') ?? 0) * 100)}`} unit="%" warnAt={0.7} hotAt={0.9} help="meter.alc" />{/if}
							{#if has('COMP_METER') && show('meter.comp')}<Meter label="Compression" value={lv('COMP_METER')} min={0} max={30} display={lv('COMP_METER')?.toFixed(0)} unit=" dB" help="meter.comp" />{/if}
							{#if has('TEMP_METER') && show('meter.temp')}<Meter label="Temp" value={lv('TEMP_METER')} min={20} max={90} display={lv('TEMP_METER')?.toFixed(0)} unit=" °C" warnAt={65} hotAt={80} help="meter.temp" />{/if}
						{/if}
					</div>
					{/if}

					<!-- PTT -->
					{#if caps?.canPtt && show('ptt')}
						<Button
							size="lg"
							data-control="ptt"
							class="h-14 w-full text-base font-semibold {st?.ptt ? 'bg-tx text-white hover:bg-tx/90' : ''}"
							variant={st?.ptt ? 'default' : 'secondary'}
							disabled={locked}
							onclick={() => run('set_ptt', { on: !st?.ptt })}
						><Mic class="size-5" />{st?.ptt ? 'Transmitting — click to stop' : 'Push to talk (toggle)'}</Button>
					{/if}
				</Card.Content>
			</Card.Root>
		</div>

		<!-- Everything below is a set of collapsible panels that flow into as many columns as fit. -->
		<div class="gap-4 columns-[20rem]">
			<!-- Bands -->
			{#if show('bands')}
			<Panel id="bands" title="Bands">
				<div data-control="bands">
					<div class="flex flex-wrap gap-1.5">
						{#each BANDS as b}
							<Button size="sm" variant={currentBand?.name === b.name ? 'default' : 'outline'} disabled={locked} onclick={() => gotoBand(b.name)} aria-pressed={currentBand?.name === b.name}>{b.name}</Button>
						{/each}
					</div>
					<p class="mt-2 text-xs text-muted-foreground">Remembers your last frequency and mode on each band. Band edges are a guide only; nothing is ever blocked.</p>
				</div>
			</Panel>
			{/if}

			{#if modeCardVisible}
			<Panel id="mode" title="Mode" help={{ kind: 'ui', name: 'mode' }}>
<div class="space-y-4">
					{#if show('mode')}
					<div class="flex flex-wrap gap-1.5" data-control="mode">
						{#each modes as m}
							<Button size="sm" variant={st?.mode === m ? 'default' : 'outline'} disabled={locked} onclick={() => run('set_mode', { mode: m, passband: 0 })} aria-pressed={st?.mode === m} title={helpFor('mode', m)}>{m}</Button>
						{/each}
					</div>
					{/if}
					{#if caps && caps.vfos.length > 1 && (show('vfo') || show('split'))}
						<div class="flex flex-wrap items-center gap-1.5" data-control="vfo split">
							{#if show('vfo')}
							{#each ['VFOA', 'VFOB'].filter((v) => caps.vfos.includes(v)) as v}
								<Button size="sm" variant={st?.vfo === v ? 'default' : 'outline'} disabled={locked} onclick={() => run('set_vfo', { vfo: v })} aria-pressed={st?.vfo === v}>{v.replace('VFO', 'VFO ')}</Button>
							{/each}
							{#if caps.vfoOps.includes('XCHG')}<Button size="sm" variant="ghost" disabled={locked} onclick={() => run('vfo_op', { op: 'XCHG' })}><ArrowLeftRight class="size-4" />Swap</Button>{/if}
							{#if caps.vfoOps.includes('CPY')}<Button size="sm" variant="ghost" disabled={locked} onclick={() => run('vfo_op', { op: 'CPY' })}><Copy class="size-4" />A=B</Button>{/if}
							{/if}
							{#if show('split') && caps.canSplit}
							<label class="ml-auto flex items-center gap-2 text-sm">Split <Help kind="ui" name="split" />
								<Switch checked={st?.split ?? false} disabled={locked} onCheckedChange={(on) => run('set_split', { on })} aria-label="Split" />
							</label>
							{/if}
						</div>
					{/if}
					{#if caps && ((caps.maxRit > 0 && show('rit')) || (caps.maxXit > 0 && show('xit')))}
						<div class="space-y-3">
							{#each [{ k: 'rit', on: caps.maxRit, v: st?.rit ?? 0, lim: caps.maxRit }, { k: 'xit', on: caps.maxXit, v: st?.xit ?? 0, lim: caps.maxXit }].filter((x) => x.on > 0 && show(x.k)) as r}
								<div class="space-y-1.5" data-control={r.k}>
									<div class="flex items-center justify-between text-xs">
										<span class="flex items-center gap-1 font-medium uppercase text-muted-foreground">{r.k} <Help kind="ui" name={r.k} /></span>
										<span class="font-mono tabular-nums">{r.v > 0 ? '+' : ''}{r.v} Hz</span>
									</div>
									<div class="flex items-center gap-2">
										<Slider type="single" value={r.v} min={-r.lim} max={r.lim} step={10} disabled={locked} onValueChange={(v) => run(`set_${r.k}`, { hz: v })} aria-label={r.k.toUpperCase()} />
										<Button size="xs" variant="ghost" disabled={locked} onclick={() => run(`set_${r.k}`, { hz: 0 })}>Clear</Button>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>
</Panel>

			{/if}

			{#if (canTune || hasTunerSwitch) && show('tuner')}
				<Panel id="tuner" title="Antenna tuner" help={{ kind: 'ui', name: 'tuner' }}>
<div class="space-y-3">
						{#if hasTunerSwitch}
							<label class="flex items-center justify-between gap-2 text-sm">
								<span>Tuner in line</span>
								<Switch checked={st?.funcs['TUNER'] ?? false} disabled={locked} onCheckedChange={(on) => run('set_func', { name: 'TUNER', on })} aria-label="Tuner in line" />
							</label>
						{/if}
						{#if canTune}
							{#if tuneArmed}
								<div class="flex gap-2">
									<Button class="h-11 flex-1 bg-tx font-semibold text-white hover:bg-tx/90" onclick={startTune} disabled={locked}><Waypoints class="size-4" />Confirm: transmit tune carrier</Button>
									<Button variant="outline" class="h-11" onclick={() => { tuneArmed = false; clearTimeout(armTimer); }}>Cancel</Button>
								</div>
							{:else}
								{#if tunePhase !== 'idle'}
									<div class="flex gap-2">
										<Button variant="secondary" class="h-11 flex-1 font-semibold" disabled><Waypoints class="size-4 animate-pulse" />{tunePhase === 'starting' ? 'Starting tune…' : 'Tuning… watch SWR'}</Button>
										<Button class="h-11 bg-tx text-white hover:bg-tx/90" onclick={stopTune}>Stop</Button>
									</div>
								{:else}
									<Button variant="secondary" class="h-11 w-full font-semibold" onclick={armTune} disabled={locked || st?.ptt}>
										<Waypoints class="size-4" />Tune
									</Button>
								{/if}
							{/if}
							{#if tuneNote}
								<p class="rounded-md border px-3 py-2 text-sm {tuneNote.kind === 'ok' ? 'border-meter-ok/50 bg-meter-ok/10' : tuneNote.kind === 'warn' ? 'border-meter-warn/60 bg-meter-warn/10' : tuneNote.kind === 'bad' ? 'border-destructive/50 bg-destructive/10 text-destructive' : 'bg-muted text-muted-foreground'}" role="status">{tuneNote.text}</p>
							{/if}
							<p class="text-xs text-muted-foreground">A tune cycle transmits a carrier. Check that the frequency is clear and an antenna is connected. Watch SWR above while it runs.</p>
						{/if}
					</div>
</Panel>
			{/if}

			<DynamicPanels />

			{#if show('memories') && app.caps && hasMemories(app.caps)}<MemoryPanel />{/if}

			{#if show('audio')}<AudioPanel />{/if}

			{#if slidersShown.length}
				<Panel id="levels" title="Levels">
<div class="space-y-4">
						{#each slidersShown as l (l.name)}
							<div class="space-y-1.5" data-control={'lvl:' + l.name}>
								<div class="flex items-center justify-between text-xs">
									<span class="flex items-center gap-1 font-medium text-muted-foreground">{LEVEL_LABELS[l.name] ?? l.name} <Help kind="level" name={l.name} /></span>
									<span class="font-mono tabular-nums">{fmtLevel(l, shown(l.name))}</span>
								</div>
								<Slider type="single" value={shown(l.name)} min={l.min} max={l.max} step={l.step || (l.max <= 1 ? 0.01 : 1)} disabled={locked} onValueChange={(v) => setLevel(l.name, v)} aria-label={LEVEL_LABELS[l.name] ?? l.name} />
							</div>
						{/each}
					</div>
</Panel>
			{/if}

			{#if funcsShown.length}
				<Panel id="functions" title="Functions" defaultOpen="wide">
<div>
						<div class="grid grid-cols-2 gap-x-4 gap-y-2.5">
							{#each funcsShown as f (f)}
								<label class="flex items-center justify-between gap-2 text-sm" data-control={'fn:' + f}>
									<span class="flex min-w-0 items-center gap-1"><span class="truncate">{FUNC_LABELS[f] ?? f}</span> <Help kind="func" name={f} /></span>
									<Switch checked={st?.funcs[f] ?? false} disabled={locked} onCheckedChange={(on) => run('set_func', { name: f, on })} aria-label={FUNC_LABELS[f] ?? f} />
								</label>
							{/each}
						</div>
					</div>
</Panel>
			{/if}
		</div>
	</div>
{/if}
