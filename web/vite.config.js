import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// `npm run dev` proxies the API (and the exec WebSocket) to `locci serve`.
export default defineConfig({
	plugins: [sveltekit()],
	server: {
		proxy: {
			'/api': { target: 'http://localhost:8998', ws: true },
			'/health': 'http://localhost:8998'
		}
	}
});
