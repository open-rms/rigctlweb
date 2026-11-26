import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vite';

export default defineConfig({
	resolve: { alias: { '#lib': fileURLToPath(new URL('./src/lib', import.meta.url)) } },
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// Static SPA: the Go binary embeds build/ and serves index.html for any route.
			adapter: adapter({ pages: 'dist', assets: 'dist', fallback: 'index.html', strict: false })
		})
	],
	server: {
		port: 5173,
		proxy: {
			'/api': { target: 'http://127.0.0.1:8480', changeOrigin: false },
			'/ws': { target: 'ws://127.0.0.1:8480', ws: true, changeOrigin: false }
		}
	}
});
