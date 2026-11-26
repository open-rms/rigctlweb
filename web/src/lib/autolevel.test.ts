import { describe, expect, it } from 'vitest';
import { judge, planVolume, responded, TARGET_DB } from './autolevel';
import { FLOOR_DB } from './audio';

describe('auto level', () => {
	it('judges levels', () => {
		expect(judge(FLOOR_DB)).toBe('silent');
		expect(judge(-20)).toBe('ok');
		expect(judge(-3)).toBe('hot');
		expect(judge(0)).toBe('hot');
	});
	it('plans a lower volume that lands on the target', () => {
		// peak 0 dB at 100% -> -6 dB target means about 50%
		expect(planVolume(0, 1)).toBe(0.5);
		// from 80% and -2 dB: need -4 dB more -> 0.8 * 0.63 = 0.5
		expect(planVolume(-2, 0.8)).toBe(0.5);
	});
	it('never raises the volume and never goes below the floor', () => {
		expect(planVolume(-30, 0.6)).toBe(0.6);
		expect(planVolume(60, 1)).toBe(0.05);
		expect(planVolume(TARGET_DB, 0.7)).toBe(0.7);
	});
	it('notices when lowering the volume had no effect', () => {
		// 100% -> 50% should drop ~6 dB
		expect(responded(0, -6, 1, 0.5)).toBe(true);
		expect(responded(0, -2.5, 1, 0.5)).toBe(true); // just over 40% of expected
		expect(responded(0, -0.2, 1, 0.5)).toBe(false); // flat-topped: the volume control doesn't help
		expect(responded(0, 0, 1, 0.97)).toBe(true); // change too small to judge
	});
});
