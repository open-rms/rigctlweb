import { api, type AudioInput } from '#lib/api';
import { audio } from '#lib/stores/audio.svelte';
import { judge, planVolume, responded, TARGET_DB } from '#lib/autolevel';

export type Phase = 'idle' | 'measuring' | 'adjusting' | 'done' | 'failed';

const pct = (v: number) => `${Math.round(v * 100)}%`;
const db = (v: number) => `${v.toFixed(0)} dB`;
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** The sound input's volume (the manual control) and the automatic fix for clipping. */
class AutoLevel {
	input = $state<AudioInput | null>(null);
	error = $state('');
	phase = $state<Phase>('idle');
	message = $state('');
	progress = $state(0);
	/** How long to listen before deciding, 2 to 5 seconds. */
	seconds = $state(3);

	#cancel = false;
	#debounce: ReturnType<typeof setTimeout> | undefined;

	get running() { return this.phase === 'measuring' || this.phase === 'adjusting'; }

	reset() {
		this.#cancel = true;
		clearTimeout(this.#debounce);
		this.input = null;
		this.error = '';
		this.phase = 'idle';
		this.message = '';
		this.progress = 0;
	}

	async refresh(rig: string) {
		try {
			this.input = await api.audioInput(rig);
			this.error = '';
		} catch (e) {
			this.input = null;
			this.error = (e as Error).message;
		}
	}

	/** Manual control: set the volume (debounced, so dragging the slider sends few requests). */
	setVolume(rig: string, v: number) {
		if (this.input) this.input = { ...this.input, volume: v };
		clearTimeout(this.#debounce);
		this.#debounce = setTimeout(async () => {
			try {
				this.input = await api.setAudioInput(rig, { volume: v });
				this.error = '';
			} catch (e) {
				this.error = (e as Error).message;
				void this.refresh(rig);
			}
		}, 200);
	}

	async restore(rig: string) {
		try {
			this.input = await api.setAudioInput(rig, { restore: true });
			this.error = '';
			this.phase = 'idle';
			this.message = '';
		} catch (e) {
			this.error = (e as Error).message;
		}
	}

	cancel() { this.#cancel = true; }

	/**
	 * Listen for a few seconds, and if the audio is clipping lower the input volume to bring the
	 * peak to the target. Checks that the level really fell; if it didn't, puts the volume back
	 * and says the clipping is upstream of this control.
	 */
	async run(rig: string) {
		if (this.running) return;
		if (audio.status !== 'listening') {
			this.phase = 'failed';
			this.message = 'Start listening first, so there is audio to measure.';
			return;
		}
		this.#cancel = false;
		this.error = '';
		let startVol: number;
		try {
			const cur = await api.audioInput(rig);
			this.input = cur;
			if (!cur.supported) {
				this.phase = 'failed';
				this.message = cur.reason ?? "The input volume can't be changed here.";
				return;
			}
			startVol = cur.volume;
		} catch (e) {
			this.phase = 'failed';
			this.message = (e as Error).message;
			return;
		}

		const measure = async (ms: number, label: string) => {
			this.phase = 'measuring';
			this.message = label;
			this.progress = 0;
			const p = await audio.measurePeak(ms, (f) => (this.progress = f));
			return p;
		};
		const aborted = () => {
			if (this.#cancel) { this.phase = 'idle'; this.message = 'Cancelled.'; return true; }
			if (audio.status !== 'listening') { this.phase = 'failed'; this.message = 'Listening stopped, so the adjustment was abandoned.'; return true; }
			return false;
		};

		let vol = startVol;
		let peak = await measure(this.seconds * 1000, `Listening for ${this.seconds} seconds…`);
		if (aborted()) return;
		const first = peak;
		let prev: { peak: number; vol: number } | null = null;

		for (let step = 0; step < 4; step++) {
			if (prev && !responded(prev.peak, peak, prev.vol, vol) && judge(peak) === 'hot') {
				// The level didn't follow the volume: the clipping happens before this control.
				try { this.input = await api.setAudioInput(rig, { volume: startVol }); } catch { /* shown below */ }
				this.phase = 'failed';
				this.message = `Lowering the input to ${pct(vol)} didn't lower the level, so the clipping happens before this control: ` +
					`the radio's USB audio output level or the sound card's own input. I put the input back to ${pct(startVol)}. ` +
					`Lower the radio's audio output level in its menu, then try again.`;
				return;
			}
			const verdict = judge(peak);
			if (verdict === 'silent') {
				this.phase = 'done';
				this.message = prev ? `Input is now ${pct(vol)}, but I heard nothing to confirm it. Check the level meter.` : 'I heard no audio, so there was nothing to judge. Tune to something and try again.';
				return;
			}
			if (verdict === 'ok') {
				this.phase = 'done';
				this.message = prev
					? `Done. Input lowered from ${pct(startVol)} to ${pct(vol)}; peak is now ${db(peak)}.`
					: `No clipping heard in ${this.seconds} seconds (peak ${db(first)}). Nothing was changed.`;
				return;
			}
			if (step === 3) break; // three adjustments made; only judging the last one
			const next = planVolume(peak, vol);
			if (next >= vol) {
				this.phase = 'failed';
				this.message = `Still too loud at the lowest input volume. Lower the radio's audio output level in its menu.`;
				return;
			}
			this.phase = 'adjusting';
			this.message = `Peak was ${db(peak)}. Lowering the input from ${pct(vol)} to ${pct(next)}…`;
			this.progress = 0;
			try {
				this.input = await api.setAudioInput(rig, { volume: next });
			} catch (e) {
				this.phase = 'failed';
				this.message = (e as Error).message;
				return;
			}
			prev = { peak, vol };
			vol = next;
			await sleep(1200); // let the audio already buffered at the old level play out
			if (aborted()) return;
			peak = await measure(2000, 'Checking the new level…');
			if (aborted()) return;
		}
		this.phase = 'failed';
		this.message = `Peak is still ${db(peak)} (target ${db(TARGET_DB)}). Loud signals may still clip; lower the input further by hand or lower the radio's output level.`;
	}
}

export const autolevel = new AutoLevel();
