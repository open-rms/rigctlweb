# rigctlweb.openrms.dev

The rigctlWeb landing page, downloads and docs. Static SvelteKit + Tailwind v4 + shadcn-svelte,
the same toolchain and theme as the openRMS sites.

- **Docs** come from `../docs/*.md` files that start with front matter (`title`, `description`,
  `order`), rendered at build time. Edit them there; they read fine on GitHub too. Files without
  front matter (ROADMAP, DONATING) stay repository-only.
- **Downloads** come from the newest GitHub release at build time. Rebuild and deploy after
  tagging a release.

```sh
pnpm install
pnpm dev          # http://localhost:5177
pnpm check
pnpm build        # static site in build/
pnpm cf:deploy    # build + wrangler deploy (Worker rigctlweb-site, custom domain rigctlweb.openrms.dev)
```
