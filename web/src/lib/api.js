// Tiny client for the locci API. The sign-in is an HttpOnly session cookie
// the browser sends by itself; page scripts never see the token. Every request
// carries X-Locci-CSRF, which the server requires (with the cookie) for
// anything that changes state.
import { goto } from '$app/navigation';
import { scope } from '$lib/env.svelte.js';

const CSRF = { 'X-Locci-CSRF': '1' };

// Before the cookie existed the token lived in local storage. Move it into
// the cookie once, so nobody is signed out by the upgrade.
const LEGACY_KEY = 'locci_token';
export async function adoptLegacyToken() {
	let old = null;
	try {
		old = localStorage.getItem(LEGACY_KEY);
		localStorage.removeItem(LEGACY_KEY);
	} catch {
		return;
	}
	if (!old) return;
	await fetch('/api/v1/auth/session', {
		method: 'POST',
		headers: { Authorization: 'Bearer ' + old, ...CSRF }
	}).catch(() => {});
}

/** Calls /api/v1{path} (scoped to the current environment); returns JSON or throws Error(message). */
export async function api(method, path, body) {
	const res = await fetch('/api/v1' + scope(path), {
		method,
		headers: {
			...CSRF,
			...(body !== undefined ? { 'Content-Type': 'application/json' } : {})
		},
		body: body !== undefined ? JSON.stringify(body) : undefined
	});

	if (res.status === 401 && path !== '/auth/login') {
		goto('/login');
		throw new Error('Session expired, please log in again');
	}

	const data = await res.json().catch(() => ({}));
	if (!res.ok) throw new Error(data.error || res.statusText);
	return data;
}

export const get = (path) => api('GET', path);
export const post = (path, body) => api('POST', path, body ?? {});
export const del = (path) => api('DELETE', path);

/** Streams a text endpoint line by line until the signal aborts. */
export async function stream(path, onLine, signal) {
	const res = await fetch('/api/v1' + scope(path), {
		headers: CSRF,
		signal
	});
	if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || res.statusText);

	const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
	let buf = '';
	for (;;) {
		const { value, done } = await reader.read();
		if (done) break;
		buf += value;
		const lines = buf.split('\n');
		buf = lines.pop();
		lines.forEach(onLine);
	}
	if (buf) onLine(buf);
}
