import { describe, expect, it } from 'vitest';
import type { Caps } from './api';
import { BUILTIN_VIEWS, catalog } from './views';
import { buildIndex, search } from './search';
import { parseFreq, sUnits, freqGroups } from './radio';

const radios = import.meta.glob('./testdata/*.json', { eager: true, import: 'default' }) as Record<string, Caps>;
const byName = Object.fromEntries(Object.entries(radios).map(([k, v]) => [k.replace('./testdata/', '').replace('.json', ''), v]));

describe('control catalog', () => {
	it('has real radios to test against', () => {
		expect(Object.keys(byName).sort()).toEqual(expect.arrayContaining(['ftdx10', 'ic7300', 'g90', 'k3', 'ts590s', 'ft991', 'ic7610']));
	});

	it('gives every control a unique id, label and known group', () => {
		for (const [name, caps] of Object.entries(byName)) {
			const cat = catalog(caps);
			const ids = cat.map((c) => c.id);
			expect(new Set(ids).size, `${name}: duplicate ids`).toBe(ids.length);
			for (const c of cat) {
				expect(c.label, `${name}: ${c.id}`).toBeTruthy();
				expect(c.group, `${name}: ${c.id}`).toBeTruthy();
			}
		}
	});

	it('only offers what the radio supports', () => {
		const ids = (n: string) => new Set(catalog(byName[n]).map((c) => c.id));
		expect(ids('ftdx10').has('tuner')).toBe(true);
		expect(ids('ftdx10').has('ptt')).toBe(true);
		expect(ids('ftdx10').has('ext:ROOFINGFILTER')).toBe(true);
		// The dummy rig has no antenna tuner op; the FTDX-10 has no radio parameters.
		expect(ids('ftdx10').has('parm:BACKLIGHT')).toBe(false);
		expect(ids('dummy').has('parm:BACKLIGHT')).toBe(true);
		// The K3 has no CTCSS list, so FM tone controls must not appear.
		expect(ids('k3').has('fm.tone')).toBe(false);
		expect(ids('ft991').has('fm.tone')).toBe(true);
	});

	it('shows sliders for radios whose dump lists levels without ranges (Xiegu G90)', () => {
		const ids = new Set(catalog(byName.g90).map((c) => c.id));
		for (const n of ['AF', 'RF', 'SQL', 'RFPOWER', 'MICGAIN', 'KEYSPD']) expect(ids.has(`lvl:${n}`), n).toBe(true);
	});
});

describe('built-in views', () => {
	const anyCatalog = new Set(Object.values(byName).flatMap((c) => catalog(c).map((x) => x.id)));

	it('only reference control ids that exist on at least one real radio', () => {
		for (const v of BUILTIN_VIEWS.filter((v) => !v.all)) {
			for (const id of v.controls) expect(anyCatalog.has(id), `${v.id} lists unknown control "${id}"`).toBe(true);
		}
	});

	it('grow from Essentials to Advanced, and never repeat a control', () => {
		const [e, d, a] = ['essentials', 'everyday', 'advanced'].map((id) => BUILTIN_VIEWS.find((v) => v.id === id)!);
		expect(d.controls.length).toBeGreaterThan(e.controls.length);
		expect(a.controls.length).toBeGreaterThan(d.controls.length);
		for (const v of [e, d, a]) expect(new Set(v.controls).size, v.id).toBe(v.controls.length);
		for (const id of e.controls) expect(d.controls).toContain(id);
		for (const id of d.controls) expect(a.controls).toContain(id);
	});

	it('Essentials is enough to operate on every radio: frequency, mode, volume, band', () => {
		const e = BUILTIN_VIEWS.find((v) => v.id === 'essentials')!;
		for (const [name, caps] of Object.entries(byName)) {
			const shown = catalog(caps).filter((c) => e.controls.includes(c.id)).map((c) => c.id);
			for (const need of ['freq', 'mode', 'bands']) expect(shown, `${name} missing ${need}`).toContain(need);
		}
	});

	it('Everything shows all', () => {
		expect(BUILTIN_VIEWS.find((v) => v.id === 'everything')!.all).toBe(true);
	});
});

describe('frequency helpers', () => {
	it('parses typed frequencies', () => {
		expect(parseFreq('14.074')).toBe(14_074_000);
		expect(parseFreq('7074k')).toBe(7_074_000);
		expect(parseFreq('146520000')).toBe(146_520_000);
		expect(parseFreq('14.074.000')).toBe(14_074_000);
		expect(parseFreq('7.1005 MHz')).toBe(7_100_500);
		expect(parseFreq('nonsense')).toBeNull();
		expect(parseFreq('')).toBeNull();
	});
	it('groups digits and maps S-units', () => {
		expect(freqGroups(14074000)).toEqual(['14', '074', '000']);
		expect(sUnits(0).label).toBe('S9');
		expect(sUnits(-54).label).toBe('S0');
		expect(sUnits(20).label).toBe('S9+20');
	});
});


describe('search', () => {
	const essentials = BUILTIN_VIEWS.find((v) => v.id === 'essentials')!;
	const idx = (name: string) => buildIndex(byName[name], BUILTIN_VIEWS, essentials);
	const top = (name: string, q: string) => search(idx(name), q).map((i) => i.id);

	it('finds a control by its name, id fragment or what it does', () => {
		expect(top('ftdx10', 'squelch')[0]).toBe('lvl:SQL');
		expect(top('ftdx10', 'noise')).toEqual(expect.arrayContaining(['lvl:NR', 'fn:NR']));
		expect(top('ftdx10', 'notch')).toEqual(expect.arrayContaining(['fn:MN']));
		expect(top('ftdx10', 'roofing')[0]).toBe('ext:ROOFINGFILTER');
		expect(top('ftdx10', 'swr')).toContain('meter.swr');
	});

	it('requires every word to match', () => {
		expect(top('ftdx10', 'squelch zzzz')).toEqual([]);
		expect(top('ftdx10', '')).toEqual([]);
	});

	it('only offers what the connected radio has', () => {
		expect(top('k3', 'tone')).not.toContain('fm.tone');
		expect(top('ft991', 'tone')).toContain('fm.tone');
	});

	it('flags controls that are not in the current view, and finds settings and views', () => {
		const att = search(idx('ftdx10'), 'attenuator').find((i) => i.id === 'att');
		expect(att && att.kind === 'control' && att.inView).toBe(false);
		expect(top('ftdx10', 'afgain').length).toBeGreaterThanOrEqual(0);
		expect(top('ftdx10', 'rigctld')).toContain('radios');
		expect(top('ftdx10', 'essentials')).toContain('essentials');
		expect(top('ftdx10', 'dark')).toContain('dark');
	});
});

import { INTEGRATIONS, render } from './integrations';
describe('integrations', () => {
	it('every entry has steps, sources and a known confidence', () => {
		expect(INTEGRATIONS.length).toBeGreaterThanOrEqual(8);
		for (const i of INTEGRATIONS) {
			expect(i.steps.length, i.id).toBeGreaterThan(0);
			expect(['verified', 'partial', 'unverified']).toContain(i.confidence);
			for (const s of i.sources) expect(() => new URL(s), `${i.id}: ${s}`).not.toThrow();
		}
	});
	it('fills in host, port and rig id and leaves no placeholders', () => {
		for (const i of INTEGRATIONS) {
			const r = render(i, { host: '10.0.0.9', port: 4533, rigId: 'ftdx10' });
			const text = [...r.steps, r.snippet ?? ''].join('\n');
			expect(text, i.id).not.toMatch(/\{(HOST|PORT|RIGID)\}/);
		}
		const pat = INTEGRATIONS.find((i) => i.id === 'pat')!;
		expect(render(pat, { host: '10.0.0.9', port: 4533, rigId: 'ftdx10' }).snippet).toContain('10.0.0.9:4533');
	});
	it('can be found by the search', () => {
		const idx = buildIndex(null, BUILTIN_VIEWS, BUILTIN_VIEWS[1]);
		expect(search(idx, 'direwolf').map((i) => i.id)).toContain('app-direwolf');
	});
});

describe('memory channels', () => {
	const base = { can: { 'get Channel': true }, memories: [] } as unknown as Caps;
	it('counts only frequency memories, never voice or Morse slots', async () => {
		const { hasMemories } = await import('./views');
		const voiceOnly = { ...base, memories: [{ start: 1, end: 5, type: 'VOICE', caps: [] }, { start: 1, end: 5, type: 'MORSE', caps: [] }] } as Caps;
		expect(hasMemories(voiceOnly)).toBe(false);
		const ftdx10 = { ...base, memories: [{ start: 1, end: 99, type: 'MEM', caps: [] }, ...voiceOnly.memories] } as Caps;
		expect(hasMemories(ftdx10)).toBe(true);
		expect(hasMemories({ ...ftdx10, can: {} } as Caps)).toBe(false);
		// The real FT-DX10 capture (memories 1-99 plus band, voice and Morse slots).
		expect(hasMemories(byName.ftdx10)).toBe(true);
		expect(catalog(byName.ftdx10).some((c) => c.id === 'memories')).toBe(true);
	});
});
