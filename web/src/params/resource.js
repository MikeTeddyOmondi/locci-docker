// Old top-level URLs (/containers/..., /images, ...) from before environments.
export function match(param) {
	return ['containers', 'images', 'networks', 'volumes'].includes(param);
}
