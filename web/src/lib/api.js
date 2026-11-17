// Tiny client for the locci API. The token lives in localStorage.
import { goto } from '$app/navigation';

const KEY = 'locci_token';

export const auth = {
	get token() {
		return localStorage.getItem(KEY);
	},
	set token(t) {
		if (t) localStorage.setItem(KEY, t);
		else localStorage.removeItem(KEY);
	}
};

/** Calls /api/v1{path}; returns parsed JSON or throws Error(message). */
export async function api(method, path, body) {
	const res = await fetch('/api/v1' + path, {
		method,
		headers: {
			...(auth.token ? { Authorization: 'Bearer ' + auth.token } : {}),
			...(body !== undefined ? { 'Content-Type': 'application/json' } : {})
		},
		body: body !== undefined ? JSON.stringify(body) : undefined
	});

	if (res.status === 401 && path !== '/auth/login') {
		auth.token = null;
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
	const res = await fetch('/api/v1' + path, {
		headers: { Authorization: 'Bearer ' + auth.token },
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
