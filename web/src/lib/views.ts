// Views choose which controls are visible. A control has a stable ID; a view is a list of IDs
// (or "everything"). The catalog is the set of controls the connected radio can actually show,
// so a view never causes an unsupported control to appear.

import type { Caps } from '#lib/api';
import { funcList, isFM, LEVEL_LABELS, FUNC_LABELS, sliderLevels, METER_LEVELS } from '#lib/radio';

export type Control = { id: string; label: string; group: string };
export type View = {
	id: string;
	name: string;
	description?: string;
	all?: boolean;
	controls: string[];
	builtin?: boolean;
	/** A built-in view whose control list the user has changed. */
	modified?: boolean;
};

// Display order of groups in the editor.
export const GROUPS = ['Tuning', 'Memories', 'Meters', 'Transmit', 'Audio', 'Receive', 'CW', 'Tones & repeater', 'Functions', 'Radio-specific', 'Parameters'];

const LEVEL_GROUP: Record<string, string> = {
	AF: 'Audio', SQL: 'Audio', MONITOR_GAIN: 'Audio',
	RF: 'Receive', NR: 'Receive', NB: 'Receive', IF: 'Receive', PBT_IN: 'Receive', PBT_OUT: 'Receive',
	NOTCHF: 'Receive', APF: 'Receive', SLOPE_LOW: 'Receive', SLOPE_HIGH: 'Receive',
	RFPOWER: 'Transmit', MICGAIN: 'Transmit', COMP: 'Transmit', VOXGAIN: 'Transmit', VOXDELAY: 'Transmit', ANTIVOX: 'Transmit',
	CWPITCH: 'CW', KEYSPD: 'CW', BKINDL: 'CW'
};
const FUNC_GROUP: Record<string, string> = {
	NB: 'Receive', NR: 'Receive', ANF: 'Receive', MN: 'Receive', APF: 'Receive', AIP: 'Receive', AFLT: 'Receive', ANL: 'Receive', NB2: 'Receive', FAGC: 'Receive',
	COMP: 'Transmit', VOX: 'Transmit', MON: 'Transmit',
	TONE: 'Tones & repeater', TSQL: 'Tones & repeater', CSQL: 'Tones & repeater', DSQL: 'Tones & repeater',
	FBKIN: 'CW', SBKIN: 'CW'
};

/** Everything the connected radio can show, each with a stable ID, label and group. */
export function catalog(caps: Caps | null): Control[] {
	if (!caps) return [];
	const out: Control[] = [];
	const add = (id: string, label: string, group: string) => out.push({ id, label, group });
	const hasLevel = (n: string) => caps.levels.some((l) => l.name === n && l.get);
	const settable = (n: string) => caps.levels.some((l) => l.name === n && l.set);

	add('freq', 'Frequency display', 'Tuning');
	add('bands', 'Band buttons', 'Tuning');
	add('mode', 'Mode buttons', 'Tuning');
	if (caps.vfos.length > 1) add('vfo', 'VFO A/B, swap, copy', 'Tuning');
	if (caps.canSplit) add('split', 'Split', 'Tuning');
	if (caps.maxRit > 0) add('rit', 'RIT', 'Tuning');
	if (caps.maxXit > 0) add('xit', 'XIT', 'Tuning');
	if (Object.keys(caps.bandwidths ?? {}).length || (caps.filters ?? []).length) add('passband', 'Passband width', 'Receive');
	if ((caps.tuningSteps ?? []).length) add('step', 'Tuning step', 'Tuning');

	if (hasLevel('STRENGTH')) add('smeter', 'S-meter', 'Meters');
	if (hasLevel('RFPOWER_METER_WATTS') || hasLevel('RFPOWER_METER')) add('meter.power', 'Power meter', 'Meters');
	if (hasLevel('SWR')) add('meter.swr', 'SWR meter', 'Meters');
	if (hasLevel('ALC')) add('meter.alc', 'ALC meter', 'Meters');
	if (hasLevel('COMP_METER')) add('meter.comp', 'Compression meter', 'Meters');
	if (hasLevel('TEMP_METER')) add('meter.temp', 'Temperature meter', 'Meters');

	if (hasMemories(caps)) add('memories', 'Memory channels', 'Memories');
	add('audio', 'Audio monitor (listen in browser)', 'Audio');
	if (caps.canPtt) add('ptt', 'Push to talk', 'Transmit');
	if (caps.vfoOps.includes('TUNE') || caps.setFuncs.includes('TUNER')) add('tuner', 'Antenna tuner and Tune', 'Transmit');

	if (settable('AGC') && caps.agcLevels.length) add('agc', 'AGC', 'Receive');
	if (settable('PREAMP') && caps.preamps.length) add('preamp', 'Preamp', 'Receive');
	if (settable('ATT') && caps.attenuators.length) add('att', 'Attenuator', 'Receive');

	for (const l of sliderLevels(caps.levels)) add(`lvl:${l.name}`, LEVEL_LABELS[l.name] ?? l.name, LEVEL_GROUP[l.name] ?? 'Audio');
	for (const f of funcList(caps)) add(`fn:${f}`, FUNC_LABELS[f] ?? f, FUNC_GROUP[f] ?? 'Functions');

	if (caps.can['set CTCSS'] && caps.ctcss.length) add('fm.tone', 'Transmit tone (CTCSS)', 'Tones & repeater');
	if (caps.can['set CTCSS Squelch'] && caps.ctcss.length) add('fm.sql', 'Tone squelch', 'Tones & repeater');
	if (caps.can['set Repeater Duplex']) add('fm.shift', 'Repeater shift', 'Tones & repeater');
	if (caps.can['set Repeater Offset']) add('fm.offset', 'Repeater offset', 'Tones & repeater');

	for (const l of caps.extLevels ?? []) {
		if (['NUMERIC', 'INT', 'COMBO', 'CHECKBUTTON'].includes(l.type)) add(`ext:${l.name}`, l.label || l.name, 'Radio-specific');
	}
	for (const f of caps.extFuncs ?? []) if (f.type !== 'BUTTON') add(`ext:${f.name}`, f.label || f.name, 'Radio-specific');

	if (caps.can['set Param']) {
		for (const p of caps.parms ?? []) if (p.set) add(`parm:${p.name}`, p.name, 'Parameters');
		for (const e of caps.extParms ?? []) {
			if (['NUMERIC', 'INT', 'COMBO', 'CHECKBUTTON'].includes(e.type)) add(`parm:${e.name}`, e.label || e.name, 'Parameters');
		}
	}
	return out;
}

export { isFM, METER_LEVELS };

/** Memory kinds that hold frequencies (VOICE and MORSE slots transmit, so they never count). */
export const MEMORY_BANK_TYPES = ['MEM', 'CALL', 'EDGE', 'BAND', 'PRIO', 'MEMOPAD'];

export function hasMemories(caps: Caps): boolean {
	return !!caps.can?.['get Channel'] && (caps.memories ?? []).some((m) => MEMORY_BANK_TYPES.includes(m.type) && m.end >= m.start);
}

// ---- built-in views ----
// Chosen by how these controls are normally taught and used: first what you need to hear and
// make a contact, then the tools for weak or crowded signals, then transmit-audio, CW and
// interference tools, then everything. Controls a radio doesn't have are simply not shown.

const ESSENTIALS = [
	'freq', 'smeter', 'ptt', 'bands', 'mode',
	'lvl:AF', 'lvl:SQL', 'lvl:RFPOWER',
	'meter.power', 'meter.swr', 'tuner'
];
const EVERYDAY = [
	...ESSENTIALS,
	'vfo', 'split', 'rit', 'step', 'passband',
	'lvl:RF', 'lvl:MICGAIN', 'lvl:NR', 'lvl:NB', 'fn:NB', 'fn:NR', 'fn:ANF',
	'agc', 'preamp', 'att',
	'fm.tone', 'fm.sql', 'fm.shift', 'fm.offset',
	'meter.alc', 'fn:LOCK', 'audio', 'memories'
];
const ADVANCED = [
	...EVERYDAY,
	'xit',
	'lvl:COMP', 'fn:COMP', 'meter.comp', 'meter.temp',
	'lvl:NOTCHF', 'fn:MN', 'lvl:IF', 'lvl:PBT_IN', 'lvl:PBT_OUT', 'lvl:APF', 'fn:APF', 'fn:ANL', 'fn:NB2',
	'lvl:VOXGAIN', 'lvl:VOXDELAY', 'lvl:ANTIVOX', 'fn:VOX', 'lvl:MONITOR_GAIN', 'fn:MON',
	'lvl:KEYSPD', 'lvl:CWPITCH', 'lvl:BKINDL', 'fn:FBKIN', 'fn:SBKIN',
	'fn:TONE', 'fn:TSQL', 'fn:CSQL', 'lvl:SLOPE_LOW', 'lvl:SLOPE_HIGH'
];

export const BUILTIN_VIEWS: View[] = [
	{ id: 'essentials', name: 'Essentials', builtin: true, controls: ESSENTIALS,
		description: 'Just what you need to listen and make a contact: frequency, band, mode, volume, squelch, power, S-meter and SWR.' },
	{ id: 'everyday', name: 'Everyday', builtin: true, controls: EVERYDAY,
		description: 'Adds VFO and split, tuning step, filter width, RF gain, noise reduction and blanker, AGC, preamp, attenuator, mic gain and FM tones.' },
	{ id: 'advanced', name: 'Advanced', builtin: true, controls: ADVANCED,
		description: 'Adds RIT/XIT, speech compression, notch and passband shift, VOX, monitor, CW keyer and break-in.' },
	{ id: 'everything', name: 'Everything', builtin: true, all: true, controls: [],
		description: 'Every control your radio offers, including its own settings and parameters.' }
];

export const BUILTIN_IDS = new Set(BUILTIN_VIEWS.map((v) => v.id));

/** Which panel holds a control; the search uses this to open it before scrolling to the control. */
export function panelOf(id: string): string | undefined {
	if (id === 'bands') return 'bands';
	if (['mode', 'vfo', 'split', 'rit', 'xit'].includes(id)) return 'mode';
	if (id === 'tuner') return 'tuner';
	if (id === 'audio') return 'audio';
	if (id === 'memories') return 'memories';
	if (id === 'passband' || id === 'step') return 'filter';
	if (['agc', 'preamp', 'att'].includes(id)) return 'receiver';
	if (id.startsWith('fm.')) return 'fm';
	if (id.startsWith('lvl:')) return 'levels';
	if (id.startsWith('fn:')) return 'functions';
	if (id.startsWith('ext:')) return 'radio';
	if (id.startsWith('parm:')) return 'parms';
	return undefined; // freq, smeter, ptt and meters live in the operating card, always visible
}
