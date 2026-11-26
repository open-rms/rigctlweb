// Quick search over every control and setting. Pure functions so it can be tested.

import type { Caps } from '#lib/api';
import { catalog, type Control, type View } from '#lib/views';
import { helpFor, type HelpKind } from '#lib/help';
import { INTEGRATIONS } from '#lib/integrations';

export type SearchItem =
	| { kind: 'control'; id: string; label: string; group: string; keywords: string; inView: boolean }
	| { kind: 'section'; id: string; label: string; group: string; keywords: string; href: string }
	| { kind: 'view'; id: string; label: string; group: string; keywords: string }
	| { kind: 'theme'; id: string; label: string; group: string; keywords: string };

export const SETTINGS_SECTIONS = [
	{ id: 'radios', label: 'Radios & connection', words: 'rigctld host port add remove radio rig address read-only' },
	{ id: 'views', label: 'Views', words: 'view essentials everyday advanced everything custom copy customize layout show hide' },
	{ id: 'appearance', label: 'Appearance', words: 'theme dark light night red color' },
	{ id: 'access', label: 'Access & security', words: 'token sign in login password lan tls security listen' },
	{ id: 'about', label: 'About & support', words: 'version license donate coffee support' }
];

const THEMES = [
	{ id: 'system', label: 'Theme: follow system' },
	{ id: 'light', label: 'Theme: light' },
	{ id: 'dark', label: 'Theme: dark' },
	{ id: 'red', label: 'Theme: night (red)' }
];

/** Help text for a control id, if there is any (used so searching "squelch" or "notch" finds things). */
export function helpText(id: string): string {
	const [prefix, name] = id.includes(':') ? [id.slice(0, id.indexOf(':')), id.slice(id.indexOf(':') + 1)] : ['ui', id];
	const kind: HelpKind | undefined = prefix === 'lvl' ? 'level' : prefix === 'fn' ? 'func' : prefix === 'parm' ? 'parm' : prefix === 'ui' ? 'ui' : undefined;
	return (kind && helpFor(kind, name)) || '';
}

export function buildIndex(caps: Caps | null, views: View[], activeView: View | undefined): SearchItem[] {
	const items: SearchItem[] = [];
	const shown = (c: Control) => !!activeView && (!!activeView.all || activeView.controls.includes(c.id));
	for (const c of catalog(caps)) {
		items.push({ kind: 'control', id: c.id, label: c.label, group: c.group, inView: shown(c), keywords: `${c.label} ${c.id.replace(/^[a-z]+[.:]/, '')} ${c.group} ${helpText(c.id)}`.toLowerCase() });
	}
	for (const s of SETTINGS_SECTIONS) {
		items.push({ kind: 'section', id: s.id, label: s.label, group: 'Settings', href: `/settings#${s.id}`, keywords: `${s.label} ${s.words} settings`.toLowerCase() });
	}
	items.push({ kind: 'section', id: 'api-docs', label: 'API reference', group: 'Pages', href: '/api-docs', keywords: 'api openapi swagger scalar rest developer docs reference sse events token script program' });
	items.push({ kind: 'section', id: 'presets', label: 'Radio presets', group: 'Pages', href: '/presets', keywords: 'presets profiles openrms rigs settings setup ft8 winlink ardop vara ssb cw rtty psk data mode usb audio apply restore manual' });
	items.push({ kind: 'section', id: 'integrations', label: 'Integrations', group: 'Pages', href: '/integrations', keywords: 'integrations connect programs pat wsjtx fldigi direwolf js8call rigctld config snippet setup' });
	for (const i of INTEGRATIONS) {
		items.push({ kind: 'section', id: `app-${i.id}`, label: `Set up ${i.name}`, group: 'Integrations', href: `/integrations#${i.id}`, keywords: `${i.name} ${i.summary} ${i.category} integration setup configure rigctld`.toLowerCase() });
	}
	for (const v of views) {
		items.push({ kind: 'view', id: v.id, label: `View: ${v.name}`, group: 'Views', keywords: `view ${v.name} ${v.description ?? ''}`.toLowerCase() });
	}
	for (const t of THEMES) items.push({ kind: 'theme', id: t.id, label: t.label, group: 'Appearance', keywords: `${t.label} theme appearance`.toLowerCase() });
	return items;
}

/** Every word typed must appear somewhere; label matches rank above help-text matches. */
export function search(items: SearchItem[], query: string, limit = 40): SearchItem[] {
	const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
	if (!terms.length) return [];
	const scored: [number, SearchItem][] = [];
	for (const it of items) {
		const label = it.label.toLowerCase();
		let score = 0;
		let ok = true;
		for (const t of terms) {
			if (label === t) score += 100;
			else if (label.startsWith(t)) score += 60;
			else if (label.split(/[\s:&/,-]+/).some((w) => w.startsWith(t))) score += 40;
			else if (label.includes(t)) score += 25;
			else if (it.keywords.split(/[\s:&/,.()-]+/).some((w) => w.startsWith(t))) score += 10;
			else if (it.keywords.includes(t)) score += 4;
			else { ok = false; break; }
		}
		if (!ok) continue;
		if (it.kind === 'control' && it.inView) score += 3; // prefer what's already on screen
		scored.push([score, it]);
	}
	scored.sort((a, b) => b[0] - a[0] || a[1].label.localeCompare(b[1].label));
	return scored.slice(0, limit).map((s) => s[1]);
}
