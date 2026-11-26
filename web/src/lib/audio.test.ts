import { describe, expect, it } from 'vitest';
import { decodePcm, toDb, isClipping, FLOOR_DB } from './audio';

describe('audio helpers', () => {
	it('decodes s16le and finds the peak', () => {
		const b = new ArrayBuffer(6);
		const v = new DataView(b);
		v.setInt16(0, 16384, true);
		v.setInt16(2, -32768, true);
		v.setInt16(4, 0, true);
		const { samples, peak } = decodePcm(b);
		expect(Array.from(samples)).toEqual([0.5, -1, 0]);
		expect(peak).toBe(1);
	});
	it('ignores a trailing odd byte', () => {
		expect(decodePcm(new ArrayBuffer(5)).samples.length).toBe(2);
	});
	it('converts to dBFS with a floor', () => {
		expect(toDb(1)).toBeCloseTo(0);
		expect(toDb(0.5)).toBeCloseTo(-6.02, 1);
		expect(toDb(0)).toBe(FLOOR_DB);
		expect(toDb(1e-9)).toBe(FLOOR_DB);
	});
	it('flags clipping', () => {
		expect(isClipping(1)).toBe(true);
		expect(isClipping(0.5)).toBe(false);
	});
});
