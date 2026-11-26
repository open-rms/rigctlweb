import { apiPath } from '#lib/paths';
export type RigState = {
	id: string;
	name: string;
	connected: boolean;
	error?: string;
	freq: number;
	mode: string;
	passband: number;
	vfo: string;
	split: boolean;
	txVfo?: string;
	txFreq?: number;
	ptt: boolean;
	ctcss: number;
	ctcssSql: number;
	rptrShift?: string;
	rptrOffs: number;
	parms: Record<string, string>;
	rit: number;
	xit: number;
	levels: Record<string, number>;
	funcs: Record<string, boolean>;
};

export type LevelCap = { name: string; min: number; max: number; step: number; get: boolean; set: boolean };

export type ExtField = {
	name: string;
	type: 'NUMERIC' | 'CHECKBUTTON' | 'COMBO' | 'BUTTON' | 'STRING' | 'INT';
	label: string;
	tooltip?: string;
	range?: { min: number; max: number; step: number };
	values?: { value: number; name: string }[];
};
export type ParmCap = { name: string; get: boolean; set: boolean; range: { min: number; max: number; step: number }; options?: string[] };
export type ModeSet = { hz: number; modes: string[] };

export type Caps = {
	model: string;
	mfg: string;
	hamlib: string;
	modes: string[];
	vfos: string[];
	vfoOps: string[];
	getFuncs: string[];
	setFuncs: string[];
	levels: LevelCap[];
	agcLevels: { value: number; name: string }[];
	maxRit: number;
	maxXit: number;
	canSplit: boolean;
	canPtt: boolean;
	can: Record<string, boolean>;
	preamps: number[];
	attenuators: number[];
	extLevels: ExtField[];
	extFuncs: ExtField[];
	extParms: ExtField[];
	parms: ParmCap[];
	tuningSteps: ModeSet[];
	filters: ModeSet[];
	bandwidths: Record<string, { normal: number; narrow: number; wide: number }>;
	ctcss: number[];
	dcs: number[];
	memories: { start: number; end: number; type: string; caps: string[] }[];
	scanOps: string[];
};

export type RigView = { id: string; name: string; addr: string; readOnly: boolean; state: RigState };
export type Settings = { theme: 'system' | 'light' | 'dark' | 'red'; favoriteModels: number[]; listen: string; defaultView: string };
export type ViewDef = { id: string; name: string; description?: string; all?: boolean; controls: string[]; builtin?: boolean };

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
	const res = await fetch(apiPath(path), {
		method,
		headers: body === undefined ? undefined : { 'content-type': 'application/json' },
		body: body === undefined ? undefined : JSON.stringify(body),
		credentials: 'same-origin'
	});
	if (!res.ok) throw new ApiError(res.status, (await res.text()).trim() || res.statusText);
	return res.status === 204 ? (undefined as T) : ((await res.json()) as T);
}

export type AudioDevice = { id: string; name: string };
export type AudioConfig = { enabled: boolean; device: string; channel: 'left' | 'right' | 'mix' };
export type AudioView = {
	config: AudioConfig;
	available: { os: string; arch: string; method: string; devices: AudioDevice[]; notes?: string[] };
	listeners: number;
};

export type AudioInput = { supported: boolean; reason?: string; volume: number; original?: number };

export type MemoryChannel = {
	number: number; name?: string; empty: boolean; freqHz: number; mode?: string; widthHz?: number;
	split?: boolean; txFreqHz?: number; rptrShift?: '+' | '-'; rptrOffsHz?: number; ctcss?: number; ctcssSql?: number;
};
export type MemoryBank = { start: number; end: number; type: string };
export type MemoryView = {
	supported: boolean; reason?: string; banks: MemoryBank[]; channels: MemoryChannel[];
	reading: boolean; done: number; total: number; error?: string;
};

// openRMS Rigs presets (see internal/presets). Every string in a profile is crowd-sourced:
// render as text only.
export type PresetLibrary = { available: boolean; fetchedAt?: string; generatedAt?: string; source: string; license?: string; count: number; error?: string };
export type PresetStats = { stars: number; worked: number; not_worked: number };
export type PresetSummary = {
	id: string; revision: number; title: string; summary?: string; operation: string; rigMfg: string; rigName: string;
	author?: string; tags: string[]; stats: PresetStats; updatedAt?: string; matches: boolean; refused?: string;
	settings: number; manual: number; txSettings: number;
};
export type PresetExpect = { min?: number; max?: number; one_of?: (number | boolean | string)[]; not?: (number | boolean | string)[] };
export type PresetStep = {
	id: string; kind: string; name: string; value?: number | boolean | string; passbandHz?: number; expect?: PresetExpect;
	display?: string; unit?: string; note?: string; confidence?: 'high' | 'medium' | 'low'; sources: string[];
	safety: 'rx' | 'tx'; checkOnly: boolean; readable: boolean; applicable: boolean; reason?: string;
	manual?: { menu: string; value: string }; choices?: Record<string, string>; hamlib?: string; firmware?: string; menu?: string[]; tolerance?: number;
	catRead?: string; command?: string; text: string;
};
export type PresetSource = { id: string; title: string; author?: string; url?: string; date?: string; note?: string };
export type PresetView = PresetSummary & {
	connection?: string; interface?: string; hamlib?: string; firmware?: string;
	audio?: { rx_target_dbfs?: number; rx_note?: string; tx_note?: string };
	software: { app: string; setting: string; value: string; note?: string }[];
	sources: PresetSource[]; notes?: string; license: string; links: Record<string, string>; forkedFrom?: string;
	steps: PresetStep[]; connected: boolean;
};
export type PresetCheck = {
	profileId: string; revision: number; summary: Record<'ok' | 'drift' | 'unknown' | 'manual', number>;
	steps: { id: string; status: 'ok' | 'drift' | 'unknown' | 'manual'; current?: number | boolean | string; currentText?: string; noop: boolean; error?: string }[];
};
export type PresetApply = { applied: string[]; failed?: string; error?: string; skipped: string[] };
export type PresetSnapshot = { rig: string; takenAt: string; presets: string[]; saved: { kind: string; name: string; known: boolean; applied: boolean; preset: string }[] };
export type PresetRestore = { restored: string[]; unknown: string[]; failed?: string; error?: string; remaining: number };

export class ApiError extends Error {
	constructor(
		public status: number,
		message: string
	) {
		super(message);
	}
}

export const api = {
	session: () => req<{ authenticated: boolean; version: string; mode?: 'standalone' | 'hamstack' }>('GET', '/api/session'),
	login: (token: string) => req<{ authenticated: boolean }>('POST', '/api/login', { token }),
	logout: () => req<void>('POST', '/api/logout'),
	rigs: () => req<RigView[]>('GET', '/api/rigs'),
	addRig: (r: { name: string; addr: string; readOnly: boolean }) => req<RigView>('POST', '/api/rigs', r),
	deleteRig: (id: string) => req<void>('DELETE', `/api/rigs/${encodeURIComponent(id)}`),
	caps: (id: string) => req<Caps>('GET', `/api/rigs/${encodeURIComponent(id)}/caps`),
	settings: () => req<Settings>('GET', '/api/settings'),
	views: () => req<ViewDef[]>('GET', '/api/views'),
	saveViews: (v: ViewDef[]) => req<ViewDef[]>('PUT', '/api/views', v),
	audio: (id: string) => req<AudioView>('GET', `/api/rigs/${encodeURIComponent(id)}/audio`),
	saveAudio: (id: string, c: AudioConfig) => req<AudioView>('PUT', `/api/rigs/${encodeURIComponent(id)}/audio`, c),
	audioInput: (id: string) => req<AudioInput>('GET', `/api/rigs/${encodeURIComponent(id)}/audio/input`),
	setAudioInput: (id: string, b: { volume: number } | { restore: true }) =>
		req<AudioInput>('PUT', `/api/rigs/${encodeURIComponent(id)}/audio/input`, b),
	memories: (id: string) => req<MemoryView>('GET', `/api/rigs/${encodeURIComponent(id)}/memories`),
	readMemories: (id: string, from: number, to: number) =>
		req<MemoryView>('POST', `/api/rigs/${encodeURIComponent(id)}/memories/read`, { from, to }),
	stopMemories: (id: string) => req<MemoryView>('POST', `/api/rigs/${encodeURIComponent(id)}/memories/stop`),
	saveSettings: (s: Settings) => req<Settings>('PUT', '/api/settings', s),
	presets: (rig?: string | null) =>
		req<{ library: PresetLibrary; radio: { mfg: string; model: string } | null; profiles: PresetSummary[] }>('GET', `/api/presets${rig ? `?rig=${encodeURIComponent(rig)}` : ''}`),
	updatePresets: () => req<PresetLibrary>('POST', '/api/presets/update'),
	preset: (pid: string, rig?: string | null) =>
		req<PresetView>('GET', `/api/presets/${encodeURIComponent(pid)}${rig ? `?rig=${encodeURIComponent(rig)}` : ''}`),
	checkPreset: (rig: string, pid: string) => req<PresetCheck>('POST', `/api/rigs/${encodeURIComponent(rig)}/presets/${encodeURIComponent(pid)}/check`),
	// cat: the exact raw CAT commands the operator confirmed; the server refuses if they differ.
	applyPreset: (rig: string, pid: string, steps: string[], cat: string[]) =>
		req<PresetApply>('POST', `/api/rigs/${encodeURIComponent(rig)}/presets/${encodeURIComponent(pid)}/apply`, { steps, cat }),
	presetSnapshot: (rig: string) => req<{ snapshot: PresetSnapshot | null }>('GET', `/api/rigs/${encodeURIComponent(rig)}/presets-snapshot`),
	restorePresets: (rig: string) => req<PresetRestore>('POST', `/api/rigs/${encodeURIComponent(rig)}/presets-restore`)
};
