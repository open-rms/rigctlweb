import type { LevelCap } from '#lib/api';

export type Band = { name: string; lo: number; hi: number; def: number; mode: string };

/** Amateur bands for quick-tune. Frequencies in Hz. Informational only; never used to block TX. */
export const BANDS: Band[] = [
	{ name: '160m', lo: 1_800_000, hi: 2_000_000, def: 1_840_000, mode: 'LSB' },
	{ name: '80m', lo: 3_500_000, hi: 4_000_000, def: 3_573_000, mode: 'LSB' },
	{ name: '60m', lo: 5_330_000, hi: 5_410_000, def: 5_357_000, mode: 'USB' },
	{ name: '40m', lo: 7_000_000, hi: 7_300_000, def: 7_074_000, mode: 'LSB' },
	{ name: '30m', lo: 10_100_000, hi: 10_150_000, def: 10_136_000, mode: 'USB' },
	{ name: '20m', lo: 14_000_000, hi: 14_350_000, def: 14_074_000, mode: 'USB' },
	{ name: '17m', lo: 18_068_000, hi: 18_168_000, def: 18_100_000, mode: 'USB' },
	{ name: '15m', lo: 21_000_000, hi: 21_450_000, def: 21_074_000, mode: 'USB' },
	{ name: '12m', lo: 24_890_000, hi: 24_990_000, def: 24_915_000, mode: 'USB' },
	{ name: '10m', lo: 28_000_000, hi: 29_700_000, def: 28_074_000, mode: 'USB' },
	{ name: '6m', lo: 50_000_000, hi: 54_000_000, def: 50_313_000, mode: 'USB' },
	{ name: '2m', lo: 144_000_000, hi: 148_000_000, def: 144_390_000, mode: 'FM' },
	{ name: '70cm', lo: 420_000_000, hi: 450_000_000, def: 446_000_000, mode: 'FM' }
];

export function bandOf(hz: number): Band | undefined {
	return BANDS.find((b) => hz >= b.lo && hz <= b.hi);
}

/** 14074000 -> ["14","074","000"] */
export function freqGroups(hz: number, minDigits = 7): string[] {
	const s = Math.max(0, Math.round(hz)).toString().padStart(minDigits, '0');
	const out: string[] = [];
	for (let i = s.length; i > 0; i -= 3) out.unshift(s.slice(Math.max(0, i - 3), i));
	return out;
}

/** Parse "14.074", "14074", "14.074.000", "7074khz", "144.39m" into Hz. */
export function parseFreq(input: string): number | null {
	const t = input.trim().toLowerCase().replace(/\s+/g, '');
	const m = t.match(/^([\d.,]+)(hz|khz|mhz|k|m)?$/);
	if (!m) return null;
	let num = m[1].replace(/,/g, '');
	const unit = m[2];
	// "14.074.000" (grouped Hz) -> strip dots when there is more than one
	if ((num.match(/\./g) ?? []).length > 1) num = num.replace(/\./g, '');
	let v = parseFloat(num);
	if (!isFinite(v) || v <= 0) return null;
	if (unit === 'khz' || unit === 'k') v *= 1e3;
	else if (unit === 'mhz' || unit === 'm') v *= 1e6;
	else if (!unit) {
		if (num.includes('.') && v < 10_000) v *= 1e6; // bare "14.074" means MHz
		else if (v < 100_000 && !num.includes('.')) v *= 1e3; // bare "7074" means kHz
	}
	return Math.round(v);
}

/** hamlib STRENGTH is dB relative to S9. Returns {s: 0..9+, over: dB over S9}. */
export function sUnits(db: number): { label: string; frac: number } {
	const clamped = Math.max(-54, Math.min(60, db));
	const frac = (clamped + 54) / 114;
	if (db <= -54) return { label: 'S0', frac };
	if (db < 0) return { label: `S${Math.max(0, Math.round(9 + db / 6))}`, frac };
	return { label: db < 3 ? 'S9' : `S9+${Math.round(db)}`, frac };
}

export const LEVEL_LABELS: Record<string, string> = {
	AF: 'AF gain', RF: 'RF gain', SQL: 'Squelch', RFPOWER: 'TX power', MICGAIN: 'Mic gain',
	COMP: 'Compression', NR: 'Noise reduction', NB: 'Noise blanker', IF: 'IF shift',
	CWPITCH: 'CW pitch', KEYSPD: 'Keyer speed', NOTCHF: 'Notch frequency', VOXGAIN: 'VOX gain',
	VOXDELAY: 'VOX delay', ANTIVOX: 'Anti-VOX', MONITOR_GAIN: 'Monitor level', PBT_IN: 'PBT in',
	PBT_OUT: 'PBT out', APF: 'Audio peak filter', BKINDL: 'Break-in delay', SLOPE_LOW: 'Low slope',
	SLOPE_HIGH: 'High slope', PREAMP: 'Preamp', ATT: 'Attenuator', AGC: 'AGC'
};
export const FUNC_LABELS: Record<string, string> = {
	NB: 'Noise blanker', NR: 'Noise reduction', ANF: 'Auto notch', COMP: 'Compressor', VOX: 'VOX',
	TONE: 'CTCSS encode', TSQL: 'CTCSS squelch', MN: 'Manual notch', LOCK: 'Dial lock', MUTE: 'Mute',
	TUNER: 'Antenna tuner', RIT: 'RIT', XIT: 'XIT', FBKIN: 'Full break-in', SBKIN: 'Semi break-in',
	APF: 'Audio peak', AIP: 'IP+', MON: 'Monitor', ABM: 'Auto band', AFC: 'AFC', RF: 'RF', NB2: 'NB 2'
};

/** Levels that are meters (read-only gauges), never shown as sliders. */
export const METER_LEVELS = new Set([
	'STRENGTH', 'RAWSTR', 'SWR', 'ALC', 'COMP_METER', 'RFPOWER_METER', 'RFPOWER_METER_WATTS',
	'TEMP_METER', 'ID_METER', 'VD_METER', 'METER'
]);
/** Levels that are not simple sliders and are handled elsewhere or skipped for now. */
const SKIP_LEVELS = new Set(['AGC', 'PREAMP', 'ATT', 'BAND_SELECT', 'USB_AF', 'USB_AF_INPUT', 'AGC_TIME',
	'NOTCHF_RAW', 'BKIN_DLYMS', 'METER', 'BAL']);

export function sliderLevels(levels: LevelCap[]): LevelCap[] {
	return levels.filter((l) => l.set && l.max > l.min && !METER_LEVELS.has(l.name) && !SKIP_LEVELS.has(l.name)
		&& !l.name.startsWith('SPECTRUM_'));
}

export const MODE_ORDER = ['LSB', 'USB', 'CW', 'CWR', 'AM', 'FM', 'PKTUSB', 'PKTLSB', 'PKTFM', 'RTTY', 'RTTYR', 'WFM'];
export function sortModes(modes: string[]): string[] {
	const rank = (m: string) => { const i = MODE_ORDER.indexOf(m); return i < 0 ? 99 : i; };
	return [...modes].sort((a, b) => rank(a) - rank(b) || a.localeCompare(b));
}

export function isFM(mode: string | undefined): boolean {
	return !!mode && ['FM', 'WFM', 'FMN', 'FM-D', 'PKTFM', 'PKTFMN', 'DFM'].includes(mode);
}

/** 2400 -> "2.4 kHz", 600 -> "600 Hz" */
export function fmtHz(hz: number): string {
	if (hz >= 1e6) return `${+(hz / 1e6).toFixed(3)} MHz`;
	if (hz >= 1000) return `${+(hz / 1000).toFixed(2)} kHz`;
	return `${+hz.toFixed(1)} Hz`;
}

/** Selectable passband widths for a mode, from the rig's own filter and bandwidth tables. */
export function widthsFor(caps: import('#lib/api').Caps, mode: string): number[] {
	const set = new Set<number>();
	for (const f of caps.filters ?? []) if (f.modes.includes(mode) && f.hz > 0) set.add(f.hz);
	const b = caps.bandwidths?.[mode];
	if (b) for (const v of [b.narrow, b.normal, b.wide]) if (v > 0) set.add(v);
	return [...set].sort((a, b) => a - b);
}

/** Tuning steps the rig defines for a mode. */
export function stepsFor(caps: import('#lib/api').Caps, mode: string): number[] {
	const set = new Set<number>();
	for (const s of caps.tuningSteps ?? []) if (s.modes.includes(mode) && s.hz > 0) set.add(s.hz);
	return [...set].sort((a, b) => a - b);
}

const HIDDEN_FUNCS = new Set(['RIT', 'XIT', 'TUNER', 'TRANSCEIVE', 'SPECTRUM', 'SPECTRUM_HOLD', 'SCOPE', 'SEND_MORSE', 'SEND_VOICE_MEM', 'OVF_STATUS', 'SYNC', 'RESUME', 'SATMODE']);

/** Settable functions shown as plain switches (RIT/XIT and the tuner have their own controls). */
export function funcList(caps: import('#lib/api').Caps): string[] {
	return caps.setFuncs.filter((f) => !HIDDEN_FUNCS.has(f));
}
