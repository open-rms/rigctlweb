import { wsUrl } from '#lib/paths';
import { api, type Caps, type RigState, type RigView, type Settings } from '#lib/api';
import { views } from '#lib/stores/views.svelte';

type Pending = { resolve: () => void; reject: (e: Error) => void };

/** App-wide reactive store: session, rig list, selected rig's live state and its WebSocket. */
class App {
	ready = $state(false);
	authed = $state(false);
	version = $state('');
	/** 'hamstack' when running inside hamstack: sign-in and the radio list are hamstack's. */
	mode = $state<'standalone' | 'hamstack'>('standalone');
	get managed() {
		return this.mode === 'hamstack';
	}
	rigs = $state<RigView[]>([]);
	selected = $state<string | null>(null);
	state = $state<RigState | null>(null);
	caps = $state<Caps | null>(null);
	wsUp = $state(false);
	settings = $state<Settings>({ theme: 'system', favoriteModels: [], listen: '', defaultView: '' });
	lastError = $state('');
	/** Debounced link health: flips to false only after ~3 s of continuous trouble. */
	online = $state(true);
	/** True while setting changes are being applied (shown as the pulsing antenna). */
	busy = $state(false);
	#pendingCount = 0;
	#busyShownAt = 0;
	#busyTimer: ReturnType<typeof setTimeout> | undefined;

	// Appear only if a change takes a moment (no flicker for instant ones), and stay long
	// enough to be seen once shown.
	#syncBusy() {
		clearTimeout(this.#busyTimer);
		if (this.#pendingCount > 0) {
			if (!this.busy) this.#busyTimer = setTimeout(() => { if (this.#pendingCount > 0) { this.busy = true; this.#busyShownAt = Date.now(); } }, 150);
		} else if (this.busy) {
			const left = Math.max(0, 500 - (Date.now() - this.#busyShownAt));
			this.#busyTimer = setTimeout(() => { if (this.#pendingCount === 0) this.busy = false; }, left);
		}
	}

	#ws: WebSocket | null = null;
	#retry = 0;
	#timer: ReturnType<typeof setTimeout> | undefined;
	#seq = 0;
	#pending = new Map<number, Pending>();
	#capsTimer: ReturnType<typeof setTimeout> | undefined;
	#offTimer: ReturnType<typeof setTimeout> | undefined;

	#track() {
		const up = !!(this.state?.connected && this.wsUp);
		if (up) {
			clearTimeout(this.#offTimer);
			this.#offTimer = undefined;
			this.online = true;
		} else if (this.online && this.#offTimer === undefined) {
			this.#offTimer = setTimeout(() => {
				this.#offTimer = undefined;
				this.online = false;
			}, 3000);
		}
	}

	get readOnly() {
		return this.rigs.find((r) => r.id === this.selected)?.readOnly ?? false;
	}

	async init() {
		try {
			const s = await api.session();
			this.version = s.version;
			this.mode = s.mode ?? 'standalone';
			this.authed = s.authenticated;
			if (this.authed) await this.load();
		} finally {
			this.ready = true;
		}
	}

	async login(token: string) {
		await api.login(token);
		this.authed = true;
		await this.load();
	}

	async logout() {
		await api.logout().catch(() => {});
		this.#close();
		this.authed = false;
		this.state = this.caps = null;
	}

	async load() {
		const [rigs, settings] = await Promise.all([api.rigs(), api.settings()]);
		this.rigs = rigs;
		this.settings = settings;
		await views.load(settings.defaultView);
		const saved = localStorage.getItem('rigctlweb.rig');
		const pick = rigs.find((r) => r.id === saved) ?? rigs[0];
		if (pick) this.select(pick.id);
	}

	async refreshRigs() {
		this.rigs = await api.rigs();
		if (this.selected && !this.rigs.some((r) => r.id === this.selected)) {
			this.selected = null;
			this.state = this.caps = null;
			this.#close();
		}
		if (!this.selected && this.rigs[0]) this.select(this.rigs[0].id);
	}

	select(id: string) {
		this.selected = id;
		this.caps = null;
		this.state = this.rigs.find((r) => r.id === id)?.state ?? null;
		try {
			localStorage.setItem('rigctlweb.rig', id);
		} catch {
			/* storage unavailable */
		}
		this.#connect();
		this.#loadCaps(id);
	}

	#loadCaps(id: string) {
		clearTimeout(this.#capsTimer);
		api
			.caps(id)
			.then((c) => {
				if (this.selected === id) this.caps = c;
			})
			.catch(() => {
				// Rig not connected yet; retry until it is.
				this.#capsTimer = setTimeout(() => this.selected === id && this.#loadCaps(id), 2000);
			});
	}

	#close() {
		clearTimeout(this.#timer);
		const ws = this.#ws;
		this.#ws = null;
		this.wsUp = false;
		ws?.close();
	}

	#connect() {
		this.#close();
		const id = this.selected;
		if (!id) return;
		const ws = new WebSocket(wsUrl(`/ws?rig=${encodeURIComponent(id)}`));
		this.#ws = ws;
		ws.onopen = () => {
			this.wsUp = true;
			this.#track();
			this.#retry = 0;
			this.#loadCaps(id);
		};
		ws.onmessage = (ev) => {
			const m = JSON.parse(ev.data);
			if (m.type === 'state') {
				const wasConnected = this.state?.connected;
				this.state = m.state;
				this.#track();
				if (m.state.connected && !wasConnected) this.#loadCaps(id);
			} else if (m.type === 'result') {
				const p = this.#pending.get(m.id);
				this.#pending.delete(m.id);
				if (m.ok) p?.resolve();
				else p?.reject(new Error(m.error || 'command failed'));
			}
		};
		ws.onclose = () => {
			if (this.#ws !== ws) return;
			this.wsUp = false;
			this.#track();
			this.#timer = setTimeout(() => this.#connect(), Math.min(1000 * 2 ** this.#retry++, 10000));
		};
	}

	/** Send a command; errors surface in lastError and reject. */
	send(cmd: string, args: Record<string, unknown> = {}): Promise<void> {
		const ws = this.#ws;
		if (!ws || ws.readyState !== WebSocket.OPEN) return Promise.reject(new Error('not connected'));
		const id = ++this.#seq;
		this.#pendingCount++;
		this.#syncBusy();
		return new Promise<void>((resolve, reject) => {
			this.#pending.set(id, { resolve, reject });
			ws.send(JSON.stringify({ id, cmd, args }));
		})
			.catch((e: Error) => {
				this.lastError = e.message;
				throw e;
			})
			.finally(() => {
				this.#pendingCount--;
				this.#syncBusy();
			});
	}

	async saveSettings(patch: Partial<Settings>) {
		this.settings = await api.saveSettings({ ...this.settings, ...patch });
	}
}

export const app = new App();
