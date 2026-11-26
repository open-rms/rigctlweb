import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// A WebSocket stand-in: opens at once, records what is sent, and can be dropped.
class FakeWS {
	static OPEN = 1;
	static all: FakeWS[] = [];
	readyState = 1;
	sent: { id: number; cmd: string }[] = [];
	onopen: (() => void) | null = null;
	onmessage: ((ev: { data: string }) => void) | null = null;
	onclose: (() => void) | null = null;
	constructor(public url: string) {
		FakeWS.all.push(this);
	}
	send(s: string) {
		this.sent.push(JSON.parse(s));
	}
	close() {
		this.readyState = 3;
	}
	drop() {
		this.readyState = 3;
		this.onclose?.();
	}
	reply(id: number, ok = true) {
		this.onmessage?.({ data: JSON.stringify({ type: 'result', id, ok, error: ok ? undefined : 'no' }) });
	}
}

describe('busy indicator ("Applying…")', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		FakeWS.all = [];
		vi.stubGlobal('WebSocket', FakeWS);
		vi.stubGlobal('location', { protocol: 'http:', host: 'localhost' });
		vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {} });
		vi.stubGlobal('fetch', () => Promise.reject(new Error('offline')));
	});
	afterEach(() => {
		vi.useRealTimers();
		vi.unstubAllGlobals();
		vi.resetModules();
	});

	async function fresh() {
		const { app, COMMAND_TIMEOUT_MS } = await import('./rig.svelte');
		app.select('r1');
		const ws = FakeWS.all.at(-1)!;
		return { app, ws, COMMAND_TIMEOUT_MS };
	}

	it('clears after the server answers', async () => {
		const { app, ws } = await fresh();
		const p = app.send('set_freq', { hz: 14074000 });
		await vi.advanceTimersByTimeAsync(200);
		expect(app.busy).toBe(true);
		ws.reply(ws.sent[0].id);
		await p;
		await vi.advanceTimersByTimeAsync(600);
		expect(app.busy).toBe(false);
	});

	it('clears when the connection drops before the answer', async () => {
		const { app, ws } = await fresh();
		const p = app.send('set_freq', { hz: 14074000 });
		const failed = expect(p).rejects.toThrow('connection to rigctlWeb lost');
		await vi.advanceTimersByTimeAsync(200);
		expect(app.busy).toBe(true);
		ws.drop();
		await failed;
		await vi.advanceTimersByTimeAsync(600);
		expect(app.busy).toBe(false);
	});

	it('clears when another radio is selected', async () => {
		const { app } = await fresh();
		const p = app.send('set_mode', { mode: 'USB' });
		const failed = expect(p).rejects.toThrow('closed');
		await vi.advanceTimersByTimeAsync(200);
		app.select('r2');
		await failed;
		await vi.advanceTimersByTimeAsync(600);
		expect(app.busy).toBe(false);
	});

	it('gives up on an answer that never comes', async () => {
		const { app, ws, COMMAND_TIMEOUT_MS } = await fresh();
		const p = app.send('set_freq', { hz: 7074000 });
		const failed = expect(p).rejects.toThrow('no answer');
		await vi.advanceTimersByTimeAsync(COMMAND_TIMEOUT_MS + 600);
		await failed;
		expect(app.busy).toBe(false);
		// a late answer is ignored
		ws.reply(ws.sent[0].id);
		expect(app.busy).toBe(false);
	});
});
