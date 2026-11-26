<script lang="ts">
  import { page } from "$app/state";
  import { Button } from "#lib/components/ui/button/index.ts";
</script>

<svelte:head><title>{page.status === 404 ? "Not found" : "Error"} · rigctlWeb</title></svelte:head>

<section class="mx-auto max-w-2xl px-4 py-24 text-center">
  <p class="text-primary font-mono text-sm font-semibold">{page.status}</p>
  <h1 class="mt-2 text-3xl">{page.status === 404 ? "Nothing on this frequency." : "Something went wrong."}</h1>
  <p class="text-muted-foreground">The page you asked for isn't here.</p>
  <Button href="/" size="lg" class="mt-4">Back to rigctlWeb</Button>
</section>
