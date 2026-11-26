// Decisions for the automatic input-level adjustment, kept pure so they can be tested.

import { FLOOR_DB } from '#lib/audio';

/** Where a too-hot signal is brought to (peak, dBFS): loud and clean with headroom. */
export const TARGET_DB = -6;
/** At or above this the audio is clipping or about to. */
export const HOT_DB = -3;
export const MIN_VOLUME = 0.05;

export type Verdict = 'ok' | 'silent' | 'hot';

/** Is what we heard fine, silent (nothing to judge), or too hot? */
export function judge(peakDb: number): Verdict {
	if (peakDb <= FLOOR_DB + 1) return 'silent';
	return peakDb >= HOT_DB ? 'hot' : 'ok';
}

/** New volume (fraction of 100%) that would bring `peakDb` to the target. Only ever lowers. */
export function planVolume(peakDb: number, volume: number, targetDb = TARGET_DB): number {
	const wanted = volume * 10 ** ((targetDb - peakDb) / 20);
	const v = Math.min(volume, Math.max(MIN_VOLUME, wanted));
	return Math.round(v * 100) / 100;
}

/**
 * Did lowering the volume lower the level by at least 40% of what it should have? If not,
 * the clipping happens before this control (the radio's output or the sound card's input
 * stage) and no amount of lowering it here will fix it.
 */
export function responded(beforeDb: number, afterDb: number, volBefore: number, volAfter: number): boolean {
	const expected = 20 * Math.log10(volBefore / volAfter);
	if (!(expected >= 1)) return true; // too small a change to judge
	return beforeDb - afterDb >= expected * 0.4;
}
