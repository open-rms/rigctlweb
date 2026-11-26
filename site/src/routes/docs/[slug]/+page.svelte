<script lang="ts">
  import { docs } from "#lib/docs.ts";
  import { GITHUB_URL } from "#lib/release.ts";
  let { data } = $props();
  const d = $derived(data.doc);
  const i = $derived(docs.findIndex((x) => x.slug === d.slug));
</script>

<svelte:head>
  <title>{d.title} · rigctlWeb</title>
  <meta name="description" content={d.description} />
</svelte:head>

<div class="mx-auto grid max-w-6xl gap-10 px-4 pt-10 lg:grid-cols-[14rem_1fr]">
  <aside class="text-sm lg:sticky lg:top-20 lg:self-start">
    <p class="text-muted-foreground mb-2 font-semibold tracking-wide uppercase">Docs</p>
    <nav class="flex flex-col gap-1">
      {#each docs as x}
        <a href="/docs/{x.slug}/" class="rounded px-2 py-1 no-underline {x.slug === d.slug ? 'bg-primary-soft text-primary font-semibold' : 'text-muted-foreground hover:text-foreground'}">{x.title}</a>
        {#if x.slug === d.slug && x.headings.length}
          <div class="mb-1 ml-3 flex flex-col gap-0.5 border-l pl-2">
            {#each x.headings as h}
              <a href="#{h.id}" class="text-muted-foreground hover:text-foreground py-0.5 text-[0.8rem] no-underline">{h.text}</a>
            {/each}
          </div>
        {/if}
      {/each}
    </nav>
  </aside>
  <article class="min-w-0 max-w-3xl">
    <h1 class="text-3xl font-semibold tracking-tight">{d.title}</h1>
    <p class="text-muted-foreground mt-2 text-lg">{d.description}</p>
    <div class="prose mt-6">{@html d.html}</div>
    <div class="text-muted-foreground mt-12 flex flex-wrap justify-between gap-4 border-t pt-5 text-sm">
      <span>{#if i > 0}<a class="text-primary" href="/docs/{docs[i - 1].slug}/">← {docs[i - 1].title}</a>{/if}</span>
      <a class="hover:text-foreground" href="{GITHUB_URL}/edit/main/docs/{d.file}">Edit this page on GitHub</a>
      <span>{#if i < docs.length - 1}<a class="text-primary" href="/docs/{docs[i + 1].slug}/">{docs[i + 1].title} →</a>{/if}</span>
    </div>
  </article>
</div>
