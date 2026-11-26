import { wsUrl } from '#lib/paths';
import { decodePcm, toDb, isClipping, FLOOR_DB } from '#lib/audio';

export type AudioStatus = 'idle' | 'connecting' | 'listening' | 'error';

const RATE = 12000;
const PRIME = 0.12; // seconds of audio buffered before playback starts (latency vs. glitches)
const MAX_AHEAD = 0.5; // if we get this far ahead, drop frames to catch up

/** Receive-audio monitor: plays the server's audio stream and measures its level. */
class AudioMonitor {
	status = $state<AudioStatus>('idle');
	error = $state('');
	/** Peak level in dBFS (FLOOR_DB..0), smoothed for display. */
	db = $state(FLOOR_DB);
	clipped = $state(false);
	volume = $state(0.8);

	#ctx: AudioContext | null = null;
	#gain: GainNode | null = null;
	#ws: WebSocket | null = null;
	#rig = '';
	#next = 0;
	#framePeak = 0;
	#winPeak = 0;
	#clipUntil = 0;
	#timer: ReturnType<typeof setInterval> | undefined;
	#wantListening = false;

	constructor() {
		try {
			const v = Number(localStorage.getItem('rigctlweb.audio.volume'));
			if (v > 0 && v <= 1) this.volume = v;
		} catch { /* storage unavailable */ }
	}

	get active() { return this.status === 'connecting' || this.status === 'listening'; }

	/** Must be called from a click: browsers only allow sound after a user gesture. */
	async start(rigId: string) {
		this.stop();
		this.#wantListening = true;
		this.#rig = rigId;
		this.error = '';
		this.status = 'connecting';

		try {
			try { this.#ctx = new AudioContext({ sampleRate: RATE, latencyHint: 'interactive' }); }
			catch { this.#ctx = new AudioContext(); } // some browsers refuse odd rates; they resample for us
			this.#gain = this.#ctx.createGain();
			this.#gain.gain.value = this.volume;
			this.#gain.connect(this.#ctx.destination);
			await this.#ctx.resume();
		} catch {
			return this.#fail('This browser would not start audio playback.');
		}

		const ws = new WebSocket(wsUrl(`/ws/audio?rig=${encodeURIComponent(rigId)}`));
		ws.binaryType = 'arraybuffer';
		this.#ws = ws;
		this.#next = 0;
		ws.onmessage = (e) => {
			if (typeof e.data === 'string') return this.#control(e.data);
			this.#play(e.data as ArrayBuffer);
		};
		ws.onclose = () => {
			if (this.#ws !== ws) return;
			if (this.#wantListening && this.status !== 'error') this.#fail('The audio connection closed.');
		};
		this.#timer = setInterval(() => this.#tick(), 50);
	}

	stop() {
		this.#wantListening = false;
		this.#teardown();
		this.status = 'idle';
		this.error = '';
		this.db = FLOOR_DB;
		this.clipped = false;
	}

	/**
	 * Highest level heard over the next `ms` milliseconds, in dBFS. Resolves early (with what it
	 * has) if listening stops. `onProgress` gets 0..1 as time passes.
	 */
	async measurePeak(ms: number, onProgress?: (f: number) => void): Promise<number> {
		this.#winPeak = 0;
		const t0 = performance.now();
		while (performance.now() - t0 < ms && this.status === 'listening') {
			onProgress?.(Math.min(1, (performance.now() - t0) / ms));
			await new Promise((r) => setTimeout(r, 50));
		}
		onProgress?.(1);
		return toDb(this.#winPeak);
	}

	setVolume(v: number) {
		this.volume = Math.max(0, Math.min(1, v));
		if (this.#gain) this.#gain.gain.value = this.volume;
		try { localStorage.setItem('rigctlweb.audio.volume', String(this.volume)); } catch { /* storage unavailable */ }
	}

	#teardown() {
		clearInterval(this.#timer);
		const ws = this.#ws;
		this.#ws = null;
		if (ws) { ws.onmessage = ws.onclose = null; try { ws.close(); } catch { /* already closed */ } }
		void this.#ctx?.close().catch(() => {});
		this.#ctx = this.#gain = null;
	}

	#fail(msg: string) {
		this.#teardown();
		this.#wantListening = false;
		this.error = msg;
		this.status = 'error';
		this.db = FLOOR_DB;
	}

	#control(text: string) {
		let m: { type?: string; error?: string };
		try { m = JSON.parse(text); } catch { return; }
		if (m.error) {
			// Settings changed on the server: pick the new ones up without bothering the user.
			if (m.error === 'audio settings changed' && this.#wantListening) {
				const rig = this.#rig;
				setTimeout(() => this.#wantListening && void this.start(rig), 300);
				return;
			}
			this.#fail(m.error);
		} else if (m.type === 'audio') {
			this.status = 'listening';
		}
	}

	#play(buf: ArrayBuffer) {
		const ctx = this.#ctx, gain = this.#gain;
		if (!ctx || !gain) return;
		const { samples, peak } = decodePcm(buf);
		if (peak > this.#framePeak) this.#framePeak = peak;
		if (peak > this.#winPeak) this.#winPeak = peak;
		if (isClipping(peak)) this.#clipUntil = performance.now() + 1500;

		const t = ctx.currentTime;
		if (this.#next < t + 0.02) this.#next = t + PRIME; // first frame, or we ran dry: re-prime
		else if (this.#next > t + MAX_AHEAD) return; // too far behind real time: skip to catch up
		const ab = ctx.createBuffer(1, samples.length, RATE);
		ab.copyToChannel(samples, 0);
		const src = ctx.createBufferSource();
		src.buffer = ab;
		src.connect(gain);
		src.start(this.#next);
		this.#next += samples.length / RATE;
	}

	#tick() {
		const target = toDb(this.#framePeak);
		this.#framePeak = 0;
		// Rise instantly, fall slowly, like a real meter.
		this.db = target >= this.db ? target : Math.max(target, this.db - 3);
		this.clipped = performance.now() < this.#clipUntil;
	}
}

export const audio = new AudioMonitor();
