import adapter from '@sveltejs/adapter-static';

/** Builds a single-page app straight into the Go embed directory. */
export default {
	kit: {
		adapter: adapter({
			pages: '../internal/ui/dist',
			assets: '../internal/ui/dist',
			fallback: 'index.html'
		})
	}
};
