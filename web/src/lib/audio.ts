// Pure helpers for the receive-audio monitor (kept apart from the store so they can be tested).

/** Signed 16-bit little-endian PCM to floats in -1..1, plus the frame's peak. */
export function decodePcm(buf: ArrayBuffer): { samples: Float32Array<ArrayBuffer>; peak: number } {
	const n = buf.byteLength >> 1;
	const view = new DataView(buf, 0, n * 2);
	const samples = new Float32Array(n);
	let peak = 0;
	for (let i = 0; i < n; i++) {
		const v = view.getInt16(i * 2, true) / 32768;
		samples[i] = v;
		const a = Math.abs(v);
		if (a > peak) peak = a;
	}
	return { samples, peak };
}

export const FLOOR_DB = -60;

/** Peak (0..1) in dB relative to full scale, floored so a silent input reads FLOOR_DB. */
export function toDb(peak: number): number {
	return peak <= 0 ? FLOOR_DB : Math.max(FLOOR_DB, 20 * Math.log10(peak));
}

/** True when a frame is at or very near full scale, which means the audio is being clipped. */
export function isClipping(peak: number): boolean {
	return peak >= 0.985;
}
