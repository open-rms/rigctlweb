import { resolve } from '$app/paths';

/** The base path the app is served at ('' at the root, or e.g. '/integration/rigctlweb'). */
const base = resolve('/').replace(/\/$/, '');

/** URL of a server route (e.g. `/api/rigs`) under the base path. */
export function apiPath(path: string): string {
	return base + path;
}

/** WebSocket URL of a server route (e.g. `/ws?rig=x`), same host, under the base path. */
export function wsUrl(path: string): string {
	const proto = location.protocol === 'https:' ? 'wss' : 'ws';
	return `${proto}://${location.host}${base}${path}`;
}
