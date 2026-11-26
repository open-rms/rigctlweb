<script lang="ts">
	import { untrack } from 'svelte';
	import { toast } from 'svelte-sonner';
	import Panel from '#lib/components/ui/panel.svelte';
	import NativeSelect from '#lib/components/ui/native-select.svelte';
	import { Button } from '#lib/components/ui/button';
	import { api, type MemoryChannel, type MemoryView } from '#lib/api';
	import { app } from '#lib/stores/rig.svelte';

	// Memory channels: read them from the radio (nothing on the radio changes) and tune the
	// current VFO to one. Storing to the radio's memories is not offered yet.
	const MAX_READ = 200;

	let info = $state<MemoryView | null>(null);
	let loadError = $state('');
	let bankIdx = $state(0);
	let showEmpty = $state(false);
	let tuning = $state<number | null>(null);
	const rig = $derived(app.selected);

	async function load(id: string) {
		try {
			info = await api.memories(id);
			loadError = '';
		} catch (e) {
			loadError = (e as Error).message;
		}
	}

	$effect(() => {
		const id = rig;
		info = null;
		loadError = '';
		bankIdx = 0;
		if (!id) return;
		untrack(() => void load(id));
	});

	// Poll while a read is running; it stops on its own.
	$effect(() => {
		const id = rig;
		if (!id || !info?.reading) return;
		const t = setInterval(() => void load(id), 700);
		return () => clearInterval(t);
	});

	const bank = $derived(info?.banks[bankIdx]);
	const readTo = $derived(bank ? Math.min(bank.end, bank.start + MAX_READ - 1) : 0);
	const inBank = $derived((ch: MemoryChannel) => !!bank && ch.number >= bank.start && ch.number <= bank.end);
	const read = $derived((info?.channels ?? []).filter((c) => inBank(c)));
	const shown = $derived(read.filter((c) => showEmpty || !c.empty));
	const emptyCount = $derived(read.filter((c) => c.empty).length);
	const keyed = $derived(!!app.state?.ptt);

	async function start() {
		if (!rig || !bank) return;
		try {
			info = await api.readMemories(rig, bank.start, readTo);
		} catch (e) {
			toast.error((e as Error).message);
		}
	}

	async function stop() {
		if (rig) info = await api.stopMemories(rig).catch(() => info);
	}

	async function tune(ch: MemoryChannel) {
		tuning = ch.number;
		try {
			await app.send('recall_channel', { channel: ch.number });
			toast.success(`Tuned to channel ${ch.number}`);
		} catch (e) {
			toast.error(`Channel ${ch.number}: ${(e as Error).message}`);
		} finally {
			tuning = null;
			if (rig) void load(rig);
		}
	}

	const mhz = (hz: number) => (hz / 1e6).toFixed(hz % 1000 === 0 ? 3 : 6);
	function extras(ch: MemoryChannel): string {
		const out: string[] = [];
		if (ch.split && ch.txFreqHz) out.push(`TX ${mhz(ch.txFreqHz)}`);
		if (ch.rptrShift) out.push(`${ch.rptrShift}${ch.rptrOffsHz ? (ch.rptrOffsHz / 1e3).toFixed(0) + ' kHz' : ''}`);
		if (ch.ctcss) out.push(`T ${(ch.ctcss / 10).toFixed(1)}`);
		if (ch.ctcssSql) out.push(`TSQL ${(ch.ctcssSql / 10).toFixed(1)}`);
		return out.join(' · ');
	}
	const bankLabel = (b: { start: number; end: number; type: string }) =>
		`${b.type === 'MEM' ? 'Memories' : b.type === 'BAND' ? 'Band memories' : b.type === 'EDGE' ? 'Band edges' : b.type === 'CALL' ? 'Call channels' : b.type} ${b.start}–${b.end}`;
</script>

<Panel id="memories" title="Memory channels" defaultOpen="wide">
	<div class="space-y-3" data-control="memories">
		{#if loadError}<p class="text-sm text-destructive" role="alert">{loadError}</p>{/if}
		{#if info && !info.supported}
			<p class="text-sm text-muted-foreground">{info.reason}</p>
		{:else if info}
			<div class="flex flex-wrap items-end gap-2">
				{#if info.banks.length > 1}
					<label class="space-y-1 text-xs text-muted-foreground">Bank
						<NativeSelect value={String(bankIdx)} onchange={(e) => (bankIdx = Number(e.currentTarget.value))} disabled={info.reading}>
							{#each info.banks as b, i}<option value={String(i)}>{bankLabel(b)}</option>{/each}
						</NativeSelect>
					</label>
				{/if}
				{#if info.reading}
					<Button variant="outline" onclick={stop}>Stop</Button>
					<span class="text-sm text-muted-foreground" role="status">Reading {info.done} of {info.total}…</span>
				{:else}
					<Button variant="outline" onclick={start} disabled={keyed || !bank} title={keyed ? 'Not while the radio is transmitting' : ''}>
						{read.length ? 'Read again' : 'Read channels'}
					</Button>
					{#if bank && (info.banks.length === 1 || readTo < bank.end)}<span class="text-xs text-muted-foreground">{info.banks.length === 1 ? bankLabel(bank) : ''}{readTo < bank.end ? ` first ${MAX_READ} channels` : ''}</span>{/if}
				{/if}
			</div>
			{#if info.error}<p class="text-sm text-meter-warn" role="status">{info.error}</p>{/if}

			{#if shown.length}
				<ul class="divide-y rounded-lg border">
					{#each shown as ch (ch.number)}
						<li class="flex items-center gap-3 px-3 py-2">
							<span class="w-9 shrink-0 text-right font-mono text-xs text-muted-foreground">{ch.number}</span>
							{#if ch.empty}
								<span class="flex-1 text-sm text-muted-foreground">empty</span>
							{:else}
								<div class="min-w-0 flex-1">
									<div class="flex flex-wrap items-baseline gap-x-2">
										<span class="font-mono text-sm tabular-nums">{mhz(ch.freqHz)}</span>
										{#if ch.mode}<span class="text-xs font-medium">{ch.mode}</span>{/if}
										{#if ch.name}<span class="truncate text-xs text-muted-foreground">{ch.name}</span>{/if}
									</div>
									{#if extras(ch)}<div class="text-xs text-muted-foreground">{extras(ch)}</div>{/if}
								</div>
								<Button size="sm" variant="secondary" onclick={() => tune(ch)} disabled={app.readOnly || keyed || tuning !== null}
									title={app.readOnly ? 'This radio is read-only' : keyed ? 'Not while the radio is transmitting' : 'Tune the current VFO to this channel'}>
									{tuning === ch.number ? 'Tuning…' : 'Tune'}
								</Button>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
			{#if emptyCount}
				<label class="flex items-center gap-2 text-xs text-muted-foreground"><input type="checkbox" bind:checked={showEmpty} /> Show {emptyCount} empty channel{emptyCount === 1 ? '' : 's'}</label>
			{/if}
			<p class="text-xs text-muted-foreground">
				Reading changes nothing on the radio. Tune copies a channel's frequency, mode and repeater settings to the current VFO; it does not switch the radio to memory mode. Storing to the radio's memories is not available yet.
			</p>
		{/if}
	</div>
</Panel>
