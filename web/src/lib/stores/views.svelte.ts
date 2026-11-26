import { api, type ViewDef } from '#lib/api';
import { BUILTIN_IDS, BUILTIN_VIEWS, type View } from '#lib/views';

const KEY = 'rigctlweb.view';
const FALLBACK = 'everyday';

/**
 * Views: which controls are visible. Built-in views come from the UI; the server stores user
 * views plus edits to built-ins (an entry with builtin=true replaces that view's control list;
 * removing it restores the default). The active view is chosen per browser.
 */
class Views {
	defs = $state<ViewDef[]>([]);
	activeId = $state(FALLBACK);
	saving = $state(false);
	error = $state('');
	#fallback = FALLBACK;
	#saveTimer: ReturnType<typeof setTimeout> | undefined;

	/** Built-ins (with any user edit applied) followed by custom views. */
	get all(): View[] {
		const overrides = new Map(this.defs.filter((d) => d.builtin).map((d) => [d.id, d]));
		const builtins = BUILTIN_VIEWS.map((b): View => {
			const o = overrides.get(b.id);
			return o && !b.all ? { ...b, controls: [...o.controls], modified: true } : { ...b, controls: [...b.controls] };
		});
		const custom = this.defs.filter((d) => !d.builtin && !BUILTIN_IDS.has(d.id)).map((d): View => ({ ...d, controls: [...d.controls] }));
		return [...builtins, ...custom];
	}

	get active(): View {
		return this.all.find((v) => v.id === this.activeId) ?? this.all.find((v) => v.id === this.#fallback) ?? this.all[0];
	}

	get(id: string): View | undefined {
		return this.all.find((v) => v.id === id);
	}

	/** Whether a control is visible in the active view. */
	shows = (id: string): boolean => {
		const v = this.active;
		return !!v && (!!v.all || v.controls.includes(id));
	};

	async load(defaultView = '') {
		try {
			this.defs = await api.views();
		} catch {
			this.defs = [];
		}
		if (defaultView) this.#fallback = defaultView;
		let saved = '';
		try { saved = localStorage.getItem(KEY) ?? ''; } catch { /* storage unavailable */ }
		this.activeId = this.get(saved) ? saved : this.get(this.#fallback) ? this.#fallback : FALLBACK;
	}

	setActive(id: string) {
		if (!this.get(id)) return;
		this.activeId = id;
		try { localStorage.setItem(KEY, id); } catch { /* ignore */ }
	}

	// ---- editing ----

	#uniqueId(name: string): string {
		const base = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 30) || 'view';
		const taken = new Set(this.all.map((v) => v.id));
		let id = base, n = 2;
		while (taken.has(id) || BUILTIN_IDS.has(id)) id = `${base}-${n++}`;
		return id;
	}

	#commit(next: ViewDef[]) {
		this.defs = next;
		this.error = '';
		clearTimeout(this.#saveTimer);
		this.#saveTimer = setTimeout(() => this.flush(), 400);
	}

	/** Save now (also called automatically a moment after each edit). */
	async flush() {
		clearTimeout(this.#saveTimer);
		this.saving = true;
		try {
			this.defs = await api.saveViews(this.defs);
			this.error = '';
		} catch (e) {
			this.error = (e as Error).message;
		} finally {
			this.saving = false;
		}
	}

	/** New view with its own list, copied from `fromId` (or blank). Returns the new ID. */
	create(name: string, fromId?: string): string {
		const src = fromId ? this.get(fromId) : undefined;
		const id = this.#uniqueId(name);
		const def: ViewDef = { id, name: name.trim().slice(0, 40) || 'New view', controls: src?.all ? [] : [...(src?.controls ?? [])], all: src?.all || undefined, description: fromId ? `Copy of ${src?.name}` : undefined };
		this.#commit([...this.defs, def]);
		this.setActive(id);
		return id;
	}

	rename(id: string, name: string) {
		const n = name.trim().slice(0, 40);
		if (!n || BUILTIN_IDS.has(id)) return;
		this.#commit(this.defs.map((d) => (d.id === id && !d.builtin ? { ...d, name: n } : d)));
	}

	remove(id: string) {
		if (BUILTIN_IDS.has(id)) return;
		this.#commit(this.defs.filter((d) => d.id !== id));
		if (this.activeId === id) this.setActive(this.#fallback);
	}

	/** Restore a built-in view to its default control list. */
	reset(id: string) {
		this.#commit(this.defs.filter((d) => !(d.builtin && d.id === id)));
	}

	/** Turn controls on or off in a view. Built-ins get an override entry on first edit. */
	setControls(viewId: string, ids: string[], on: boolean) {
		const view = this.get(viewId);
		if (!view || view.all) return;
		const set = new Set(view.controls);
		for (const id of ids) on ? set.add(id) : set.delete(id);
		const controls = [...set];
		const builtin = BUILTIN_IDS.has(viewId);
		const exists = this.defs.some((d) => d.id === viewId && !!d.builtin === builtin);
		if (exists) {
			this.#commit(this.defs.map((d) => (d.id === viewId && !!d.builtin === builtin ? { ...d, controls } : d)));
		} else {
			this.#commit([...this.defs, { id: viewId, name: view.name, controls, builtin: builtin || undefined }]);
		}
	}
}

export const views = new Views();
