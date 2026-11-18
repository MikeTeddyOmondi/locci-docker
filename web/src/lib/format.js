export function shortId(id = '') {
	return id.replace('sha256:', '').slice(0, 12);
}

export function bytes(n = 0) {
	if (n < 1024) return n + ' B';
	const units = ['KiB', 'MiB', 'GiB', 'TiB'];
	let i = -1;
	do {
		n /= 1024;
		i++;
	} while (n >= 1024 && i < units.length - 1);
	return n.toFixed(1) + ' ' + units[i];
}

/** Unix seconds or ISO string -> "3h ago" */
export function ago(t) {
	const ms = typeof t === 'number' ? t * 1000 : Date.parse(t);
	const s = Math.max(0, Math.round((Date.now() - ms) / 1000));
	if (s < 60) return s + 's ago';
	if (s < 3600) return Math.floor(s / 60) + 'm ago';
	if (s < 86400) return Math.floor(s / 3600) + 'h ago';
	return Math.floor(s / 86400) + 'd ago';
}

export function containerName(c) {
	return (c.Names?.[0] || '').replace(/^\//, '') || shortId(c.Id);
}

/** "KEY=VALUE" lines -> object */
export function parsePairs(text) {
	const out = {};
	for (const line of text.split('\n')) {
		const t = line.trim();
		if (!t) continue;
		const i = t.indexOf('=');
		if (i > 0) out[t.slice(0, i)] = t.slice(i + 1);
	}
	return out;
}

/** non-empty trimmed lines */
export function lines(text) {
	return text
		.split('\n')
		.map((l) => l.trim())
		.filter(Boolean);
}
