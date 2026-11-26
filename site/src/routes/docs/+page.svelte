<script lang="ts">
  import { docs } from "#lib/docs.ts";
</script>

<svelte:head>
  <title>Docs · rigctlWeb</title>
  <meta name="description" content="Install, connecting a radio, the panel, radio presets, security, troubleshooting and the API for rigctlWeb." />
</svelte:head>

<section class="mx-auto max-w-4xl px-4 pt-12">
  <h1 class="text-3xl font-semibold tracking-tight">Documentation</h1>
  <p class="text-muted-foreground mt-2">Everything about running rigctlWeb. The same pages are in the repository's <code>docs/</code> folder.</p>
  <div class="mt-6 divide-y rounded-lg border bg-card">
    {#each docs as d}
      <a href="/docs/{d.slug}/" class="hover:bg-muted block p-4 no-underline">
        <p class="text-foreground my-0 font-semibold">{d.title}</p>
        <p class="text-muted-foreground my-1 text-sm">{d.description}</p>
      </a>
    {/each}
  </div>
</section>
