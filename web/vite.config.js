import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

// `npm run dev` proxies the API (and the exec WebSocket) to `locci serve`.
export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		proxy: {
			'/api': { target: 'http://localhost:8998', ws: true },
			'/health': 'http://localhost:8998'
		}
	}
});
