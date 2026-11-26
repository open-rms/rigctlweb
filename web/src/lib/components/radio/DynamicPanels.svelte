<script lang="ts">
	import { app } from '#lib/stores/rig.svelte';
	import { fmtHz, isFM, stepsFor, widthsFor } from '#lib/radio';
	import * as Card from '#lib/components/ui/card';
	import Panel from '#lib/components/ui/panel.svelte';
	import { Button } from '#lib/components/ui/button';
	import { Slider } from '#lib/components/ui/slider';
	import { Switch } from '#lib/components/ui/switch';
	import NativeSelect from '#lib/components/ui/native-select.svelte';
	import { toast } from 'svelte-sonner';
	import { views } from '#lib/stores/views.svelte';
	import Help from '#lib/components/ui/help.svelte';
	import { helpFor } from '#lib/help';
	import { ChevronLeft, ChevronRight } from '@lucide/svelte';

	// Everything here is driven by what the connected radio advertises (dump_caps). A panel
	// or control that the radio doesn't support is simply not rendered.
	const st = $derived(app.state);
	const caps = $derived(app.caps);
	const locked = $derived(!app.online || !st || app.readOnly);
	const mode = $derived(st?.mode ?? '');

	async function run(cmd: string, args: Record<string, unknown> = {}) {
		try {
			await app.send(cmd, args);
		} catch (e) {
			toast.error((e as Error).message, { id: 'rig-command-error' });
		}
	}

	const show = (id: string) => views.shows(id);
	const settable = (name: string) => !!caps?.levels.some((l) => l.name === name && l.set);

	// ---- filter width + tuning step ----
	const widths = $derived(caps && mode && show('passband') ? widthsFor(caps, mode) : []);
	const steps = $derived(caps && mode && show('step') ? stepsFor(caps, mode) : []);
	let stepHz = $state(0);
	$effect(() => {
		if (!steps.length) return;
		let saved = 0;
		try { saved = Number(localStorage.getItem(`rigctlweb.step.${mode}`)) || 0; } catch { /* ignore */ }
		stepHz = steps.includes(saved) ? saved : (steps.find((s) => s >= 100) ?? steps[0]);
	});
	function pickStep(hz: number) {
		stepHz = hz;
		try { localStorage.setItem(`rigctlweb.step.${mode}`, String(hz)); } catch { /* ignore */ }
	}
	const nudge = (dir: 1 | -1) => st && stepHz && run('set_freq', { hz: st.freq + dir * stepHz });

	// ---- receiver chain: AGC / preamp / attenuator ----
	const agc = $derived(show('agc') && caps && settable('AGC') && caps.agcLevels.length ? caps.agcLevels : []);
	const preamps = $derived(show('preamp') && caps && settable('PREAMP') && caps.preamps.length ? [0, ...caps.preamps] : []);
	const attens = $derived(show('att') && caps && settable('ATT') && caps.attenuators.length ? [0, ...caps.attenuators] : []);

	// ---- FM tones and repeater ----
	const fmTone = $derived(show('fm.tone') && !!caps?.can['set CTCSS'] && !!caps.ctcss.length);
	const fmSql = $derived(show('fm.sql') && !!caps?.can['set CTCSS Squelch'] && !!caps.ctcss.length);
	const fmShift = $derived(show('fm.shift') && !!caps?.can['set Repeater Duplex']);
	const fmOffs = $derived(show('fm.offset') && !!caps?.can['set Repeater Offset']);
	const showFM = $derived(isFM(mode) && (fmTone || fmSql || fmShift || fmOffs));
	const tone = (t: number) => (t === 0 ? 'Off' : `${(t / 10).toFixed(1)} Hz`);
	let offsKHz = $state('');
	$effect(() => { offsKHz = String((st?.rptrOffs ?? 0) / 1000); });

	// ---- radio-specific extras and parameters ----
	const extNumeric = (t: string) => t === 'NUMERIC' || t === 'INT';
	const extLevels = $derived((caps?.extLevels ?? []).filter((l) => ['NUMERIC', 'INT', 'COMBO', 'CHECKBUTTON'].includes(l.type) && show('ext:' + l.name)));
	const extFuncs = $derived((caps?.extFuncs ?? []).filter((f) => f.type !== 'BUTTON' && show('ext:' + f.name)));
	// Standard and radio-specific parameters share one card. A parameter is shown as a choice
	// list, a switch, a slider (known range) or a plain number box (range unknown).
	type ParmView = { name: string; label: string; tip?: string; kind: 'enum' | 'combo' | 'switch' | 'slider' | 'number'; options?: string[]; values?: { value: number; name: string }[]; min?: number; max?: number; step?: number };
	const parms = $derived.by((): ParmView[] => {
		if (!caps?.can['set Param']) return [];
		const out: ParmView[] = [];
		for (const p of caps.parms ?? []) {
			if (!p.set) continue;
			if (p.options?.length) out.push({ name: p.name, label: p.name, kind: 'enum', options: p.options });
			else if (p.range.max > p.range.min) out.push({ name: p.name, label: p.name, kind: 'slider', min: p.range.min, max: p.range.max, step: p.range.step });
			else out.push({ name: p.name, label: p.name, kind: 'number' });
		}
		for (const e of caps.extParms ?? []) {
			if (e.type === 'COMBO' && e.values?.length) out.push({ name: e.name, label: e.label || e.name, tip: e.tooltip, kind: 'combo', values: e.values });
			else if (e.type === 'CHECKBUTTON') out.push({ name: e.name, label: e.label || e.name, tip: e.tooltip, kind: 'switch' });
			else if (extNumeric(e.type) && e.range && e.range.max > e.range.min) out.push({ name: e.name, label: e.label || e.name, tip: e.tooltip, kind: 'slider', min: e.range.min, max: e.range.max, step: e.range.step });
			else if (extNumeric(e.type)) out.push({ name: e.name, label: e.label || e.name, tip: e.tooltip, kind: 'number' });
		}
		return out.filter((p) => show('parm:' + p.name));
	});
	let numberDraft = $state<Record<string, string>>({});
	const lvl = (n: string) => st?.levels[n] ?? 0;
</script>

{#snippet field(label: string, tip: string | undefined, body: import('svelte').Snippet, ui?: string, ctl?: string)}
	<div class="space-y-1.5" data-control={ctl}>
		<div class="flex items-center gap-1 text-xs font-medium text-muted-foreground">{label}{#if ui}<Help kind="ui" name={ui} />{:else if tip}<Help text={tip} />{/if}</div>
		{@render body()}
	</div>
{/snippet}

{#if widths.length || steps.length}
	<Panel id="filter" title="Filter & tuning step">
<div class="space-y-4">
			{#if widths.length}
				{#snippet widthBody()}
					<div class="flex flex-wrap gap-1.5">
						{#each widths as w}
							<Button size="sm" variant={Math.abs((st?.passband ?? 0) - w) < 1 ? 'default' : 'outline'} disabled={locked} aria-pressed={Math.abs((st?.passband ?? 0) - w) < 1} onclick={() => run('set_mode', { mode, passband: w })}>{fmtHz(w)}</Button>
						{/each}
					</div>
				{/snippet}
				{@render field(`Passband (${mode}${st?.passband ? ', now ' + fmtHz(st.passband) : ''})`, undefined, widthBody, 'passband', 'passband')}
			{/if}
			{#if steps.length}
				{#snippet stepBody()}
					<div class="flex flex-wrap items-center gap-1.5">
						<Button size="icon" variant="outline" disabled={locked} onclick={() => nudge(-1)} aria-label="Tune down one step"><ChevronLeft class="size-4" /></Button>
						{#each steps as s}
							<Button size="sm" variant={stepHz === s ? 'default' : 'outline'} aria-pressed={stepHz === s} onclick={() => pickStep(s)}>{fmtHz(s)}</Button>
						{/each}
						<Button size="icon" variant="outline" disabled={locked} onclick={() => nudge(1)} aria-label="Tune up one step"><ChevronRight class="size-4" /></Button>
					</div>
				{/snippet}
				{@render field('Tuning step', undefined, stepBody, 'step', 'step')}
			{/if}
		</div>
</Panel>
{/if}

{#if agc.length || preamps.length || attens.length}
	<Panel id="receiver" title="Receiver">
<div class="grid gap-4 sm:grid-cols-3">
			{#if agc.length}
				{#snippet agcBody()}
					<NativeSelect value={String(lvl('AGC'))} disabled={locked} onchange={(e) => run('set_level', { name: 'AGC', value: Number(e.currentTarget.value) })} aria-label="AGC">
						{#each agc as a}<option value={String(a.value)}>{a.name[0] + a.name.slice(1).toLowerCase()}</option>{/each}
					</NativeSelect>
				{/snippet}
				{@render field('AGC', undefined, agcBody, 'agc', 'agc')}
			{/if}
			{#if preamps.length}
				{#snippet preBody()}
					<NativeSelect value={String(lvl('PREAMP'))} disabled={locked} onchange={(e) => run('set_level', { name: 'PREAMP', value: Number(e.currentTarget.value) })} aria-label="Preamp">
						{#each preamps as p}<option value={String(p)}>{p === 0 ? 'Off' : `${p} dB`}</option>{/each}
					</NativeSelect>
				{/snippet}
				{@render field('Preamp', undefined, preBody, 'preamp', 'preamp')}
			{/if}
			{#if attens.length}
				{#snippet attBody()}
					<NativeSelect value={String(lvl('ATT'))} disabled={locked} onchange={(e) => run('set_level', { name: 'ATT', value: Number(e.currentTarget.value) })} aria-label="Attenuator">
						{#each attens as a}<option value={String(a)}>{a === 0 ? 'Off' : `${a} dB`}</option>{/each}
					</NativeSelect>
				{/snippet}
				{@render field('Attenuator', undefined, attBody, 'attenuator', 'att')}
			{/if}
		</div>
</Panel>
{/if}

{#if showFM && caps}
	<Panel id="fm" title="FM tones & repeater">
<div class="grid gap-4 sm:grid-cols-2">
			{#if fmTone}
				{#snippet toneBody()}
					<NativeSelect value={String(st?.ctcss ?? 0)} disabled={locked} onchange={(e) => run('set_ctcss_tone', { tenths: Number(e.currentTarget.value) })} aria-label="CTCSS transmit tone">
						<option value="0">Off</option>
						{#each caps.ctcss as t}<option value={String(t)}>{tone(t)}</option>{/each}
					</NativeSelect>
				{/snippet}
				{@render field('Transmit tone (CTCSS)', undefined, toneBody, 'fm.tone', 'fm.tone')}
			{/if}
			{#if fmSql}
				{#snippet sqlBody()}
					<NativeSelect value={String(st?.ctcssSql ?? 0)} disabled={locked} onchange={(e) => run('set_ctcss_sql', { tenths: Number(e.currentTarget.value) })} aria-label="CTCSS squelch tone">
						<option value="0">Off</option>
						{#each caps.ctcss as t}<option value={String(t)}>{tone(t)}</option>{/each}
					</NativeSelect>
				{/snippet}
				{@render field('Tone squelch', undefined, sqlBody, 'fm.sql', 'fm.sql')}
			{/if}
			{#if fmShift}
				{#snippet shiftBody()}
					<div class="flex gap-1.5">
						{#each [['None', 'Simplex'], ['-', '−'], ['+', '+']] as [v, label]}
							<Button size="sm" variant={(st?.rptrShift || 'None') === v ? 'default' : 'outline'} disabled={locked} onclick={() => run('set_rptr_shift', { shift: v })}>{label}</Button>
						{/each}
					</div>
				{/snippet}
				{@render field('Repeater shift', undefined, shiftBody, 'fm.shift', 'fm.shift')}
			{/if}
			{#if fmOffs}
				{#snippet offsBody()}
					<form class="flex items-center gap-2" onsubmit={(e) => { e.preventDefault(); const k = parseFloat(offsKHz); if (isFinite(k) && k >= 0) run('set_rptr_offs', { hz: Math.round(k * 1000) }); }}>
						<input class="h-9 w-28 rounded-md border border-input bg-background px-2.5 font-mono text-sm" inputmode="decimal" bind:value={offsKHz} disabled={locked} aria-label="Repeater offset in kHz" />
						<span class="text-sm text-muted-foreground">kHz</span>
						<Button size="sm" type="submit" variant="outline" disabled={locked}>Set</Button>
					</form>
				{/snippet}
				{@render field('Repeater offset', undefined, offsBody, 'fm.offset', 'fm.offset')}
			{/if}
		</div>
</Panel>
{/if}

{#if extLevels.length || extFuncs.length}
	<Panel id="radio" title={`${caps?.mfg ?? ''} ${caps?.model ?? ''} settings`} defaultOpen="wide">
<div class="space-y-4">
			{#each extLevels as l (l.name)}
				{#if l.type === 'COMBO' && l.values?.length}
					{#snippet comboBody()}
						<NativeSelect value={String(Math.round(lvl(l.name)))} disabled={locked} onchange={(e) => run('set_level', { name: l.name, value: Number(e.currentTarget.value) })} aria-label={l.label || l.name}>
							{#each l.values ?? [] as v}<option value={String(v.value)}>{v.name}</option>{/each}
						</NativeSelect>
					{/snippet}
					{@render field(l.label || l.name, l.tooltip, comboBody, undefined, 'ext:' + l.name)}
				{:else if l.type === 'CHECKBUTTON'}
					<label class="flex items-center justify-between gap-2 text-sm" title={l.tooltip} data-control={'ext:' + l.name}>
						<span>{l.label || l.name}</span>
						<Switch checked={lvl(l.name) >= 0.5} disabled={locked} onCheckedChange={(on) => run('set_level', { name: l.name, value: on ? 1 : 0 })} aria-label={l.label || l.name} />
					</label>
				{:else if l.range && l.range.max > l.range.min}
					<div class="space-y-1.5" title={l.tooltip} data-control={'ext:' + l.name}>
						<div class="flex items-center justify-between text-xs">
							<span class="font-medium text-muted-foreground">{l.label || l.name}</span>
							<span class="font-mono tabular-nums">{+lvl(l.name).toFixed(2)}</span>
						</div>
						<Slider type="single" value={lvl(l.name)} min={l.range.min} max={l.range.max} step={l.range.step || (l.range.max <= 1 ? 0.01 : 1)} disabled={locked} onValueChange={(v) => run('set_level', { name: l.name, value: v })} aria-label={l.label || l.name} />
					</div>
				{/if}
			{/each}
			{#each extFuncs as f (f.name)}
				<label class="flex items-center justify-between gap-2 text-sm" title={f.tooltip} data-control={'ext:' + f.name}>
					<span>{f.label || f.name}</span>
					<Switch checked={st?.funcs[f.name] ?? false} disabled={locked} onCheckedChange={(on) => run('set_func', { name: f.name, on })} aria-label={f.label || f.name} />
				</label>
			{/each}
		</div>
</Panel>
{/if}

{#if parms.length}
	<Panel id="parms" title="Radio parameters" defaultOpen="wide">
<div class="space-y-4">
			{#each parms as p (p.name)}
				<div data-control={'parm:' + p.name}>
				{#if p.kind === 'enum'}
					{#snippet enumBody()}
						<NativeSelect value={st?.parms[p.name] ?? ''} disabled={locked} onchange={(e) => run('set_parm', { name: p.name, value: e.currentTarget.value })} aria-label={p.label}>
							{#if !(p.options ?? []).includes(st?.parms[p.name] ?? '')}<option value={st?.parms[p.name] ?? ''} disabled>{st?.parms[p.name] || '…'}</option>{/if}
							{#each p.options ?? [] as o}<option value={o}>{o}</option>{/each}
						</NativeSelect>
					{/snippet}
					{@render field(p.label, p.tip ?? helpFor('parm', p.name), enumBody)}
				{:else if p.kind === 'combo'}
					{#snippet comboBody()}
						<NativeSelect value={String(Math.round(Number(st?.parms[p.name] ?? 0)))} disabled={locked} onchange={(e) => run('set_parm', { name: p.name, value: e.currentTarget.value })} aria-label={p.label}>
							{#each p.values ?? [] as v}<option value={String(v.value)}>{v.name}</option>{/each}
						</NativeSelect>
					{/snippet}
					{@render field(p.label, p.tip, comboBody)}
				{:else if p.kind === 'switch'}
					<label class="flex items-center justify-between gap-2 text-sm" title={p.tip}>
						<span>{p.label}</span>
						<Switch checked={Number(st?.parms[p.name] ?? 0) >= 0.5} disabled={locked} onCheckedChange={(on) => run('set_parm', { name: p.name, value: on ? '1' : '0' })} aria-label={p.label} />
					</label>
				{:else if p.kind === 'slider'}
					<div class="space-y-1.5" title={p.tip}>
						<div class="flex items-center justify-between text-xs">
							<span class="font-medium text-muted-foreground">{p.label}</span>
							<span class="font-mono tabular-nums">{st?.parms[p.name] ?? ''}</span>
						</div>
						<Slider type="single" value={Number(st?.parms[p.name] ?? p.min)} min={p.min!} max={p.max!} step={p.step || (p.max! <= 1 ? 0.01 : 1)} disabled={locked} onValueChange={(v) => run('set_parm', { name: p.name, value: String(v) })} aria-label={p.label} />
					</div>
				{:else}
					<form class="space-y-1.5" title={p.tip} onsubmit={(e) => { e.preventDefault(); const v = (numberDraft[p.name] ?? st?.parms[p.name] ?? '').trim(); if (v !== '') run('set_parm', { name: p.name, value: v }); }}>
						<div class="text-xs font-medium text-muted-foreground">{p.label}</div>
						<div class="flex items-center gap-2">
							<input class="h-9 w-32 rounded-md border border-input bg-background px-2.5 font-mono text-sm" inputmode="decimal" disabled={locked} aria-label={p.label}
								value={numberDraft[p.name] ?? st?.parms[p.name] ?? ''} oninput={(e) => (numberDraft[p.name] = e.currentTarget.value)} />
							<Button size="sm" type="submit" variant="outline" disabled={locked}>Set</Button>
						</div>
					</form>
				{/if}
				</div>
			{/each}
		</div>
</Panel>
{/if}
