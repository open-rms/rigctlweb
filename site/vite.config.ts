import { sveltekit } from "@sveltejs/kit/vite";
import tailwindcss from "@tailwindcss/vite";
import adapter from "@sveltejs/adapter-static";
import { defineConfig } from "vite";

// rigctlweb.openrms.dev is fully prerendered: plain HTML in build/, served by Cloudflare
// Workers Static Assets (wrangler.jsonc). The docs are read from ../docs at build time,
// and the downloads from the latest GitHub release.
export default defineConfig({
  plugins: [tailwindcss(), sveltekit({ adapter: adapter({ fallback: "404.html" }) })],
  server: { port: 5177, fs: { allow: [".."] } },
});
