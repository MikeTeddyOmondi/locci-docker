// The environment (Docker host) the UI is looking at.
//
// Pages under /e/[env]/ set `envs.current`; api.js then sends Docker resource
// calls to /api/v1/environments/<current>/..., so page code stays the same for
// every host. The last choice is remembered per browser.
import { api } from '$lib/api.js';

const KEY = 'locci_env';
const SCOPED = ['containers', 'images', 'networks', 'volumes', 'system'];

export const envs = $state({ current: '', list: [], loaded: false });

/** Rewrites a Docker resource path for the current environment. */
export function scope(path) {
	if (!envs.current) return path;
	const head = path.split(/[/?]/)[1];
	return SCOPED.includes(head) ? `/environments/${encodeURIComponent(envs.current)}${path}` : path;
}

/** The environment to land on: the remembered one if it still exists, else the server default. */
export function preferred() {
	let saved = '';
	try {
		saved = localStorage.getItem(KEY) || '';
	} catch {}
	if (envs.list.some((e) => e.name === saved)) return saved;
	return envs.list.find((e) => e.is_default)?.name || envs.list[0]?.name || 'local';
}

/** A UI link inside the current environment, e.g. href('/containers'). */
export function href(path = '/') {
	const base = `/e/${encodeURIComponent(envs.current || preferred())}`;
	return path === '/' ? base : base + path;
}

export function remember(name) {
	envs.current = name;
	try {
		localStorage.setItem(KEY, name);
	} catch {}
}

export const currentEnv = () => envs.list.find((e) => e.name === envs.current);

/** Loads the environment list (status dots refresh with it). */
export async function loadEnvs() {
	const res = await api('GET', '/environments');
	envs.list = res.environments;
	envs.loaded = true;
	return envs.list;
}

/** Status -> classes for a signal dot. */
export function statusTone(status) {
	if (status === 'up') return 'text-ok signal-live';
	if (status === 'down') return 'text-destructive';
	return 'text-muted-foreground';
}
