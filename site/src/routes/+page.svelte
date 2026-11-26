<script lang="ts">
  import { Button } from "#lib/components/ui/button/index.ts";
  import * as Card from "#lib/components/ui/card/index.ts";
  import { archOf, mb, GITHUB_URL } from "#lib/release.ts";
  import { docs } from "#lib/docs.ts";
  import Gauge from "@lucide/svelte/icons/gauge";
  import SlidersHorizontal from "@lucide/svelte/icons/sliders-horizontal";
  import LayoutGrid from "@lucide/svelte/icons/layout-grid";
  import BookOpenCheck from "@lucide/svelte/icons/book-open-check";
  import Headphones from "@lucide/svelte/icons/headphones";
  import ShieldCheck from "@lucide/svelte/icons/shield-check";
  import Cpu from "@lucide/svelte/icons/cpu";
  import Plug from "@lucide/svelte/icons/plug";
  import Code from "@lucide/svelte/icons/code";
  import Download from "@lucide/svelte/icons/download";
  import ArrowRight from "@lucide/svelte/icons/arrow-right";
  import Heart from "@lucide/svelte/icons/heart";

  let { data } = $props();
  const rel = $derived(data.release);

  const features = [
    {
      icon: SlidersHorizontal,
      title: "Every control your radio has",
      body: "The panel is built from what rigctld says your radio supports, so you see its own modes, levels and functions, and nothing it can't do.",
    },
    {
      icon: Gauge,
      title: "Big display, live meters",
      body: "A frequency display you can scroll, click or type into, band buttons that remember where you were, and live S-meter, power, SWR and ALC.",
    },
    {
      icon: LayoutGrid,
      title: "From beginner to expert",
      body: "Start with Essentials (frequency, mode, volume, band) and grow to Everyday, Advanced or Everything, or build your own view.",
    },
    {
      icon: BookOpenCheck,
      title: "Radio presets, read like a manual",
      body: "Settings other operators shared on openRMS Rigs for your model and an operation like FT8 or Winlink: why each value, with sources. Check them against your radio, apply what's safe, restore your old values.",
    },
    {
      icon: Headphones,
      title: "Hear the radio in the browser",
      body: "Optional receive audio from the computer's sound card, off until you turn it on, captured only while someone listens.",
    },
    {
      icon: ShieldCheck,
      title: "Careful with your transmitter",
      body: "A hard transmit timeout, it unkeys only what it keyed, presets never key the radio, and it listens on your own computer until you choose otherwise.",
    },
  ];

  const more = [
    { icon: Cpu, title: "One small program", body: "A single binary with the web page inside. Linux and Raspberry Pi first, macOS and Windows too." },
    { icon: Plug, title: "Shares the radio", body: "rigctld takes several clients, so Pat, WSJT-X, fldigi and rigctlWeb can use one radio together." },
    { icon: Code, title: "An open API", body: "A versioned REST API with an OpenAPI description, for loggers, dashboards and your own scripts. It never keys the transmitter." },
  ];
</script>

<svelte:head>
  <title>rigctlWeb: your radio's control panel, in any browser</title>
  <meta name="description" content="rigctlWeb is a free, open-source web control panel for Hamlib's rigctld: tuning, live meters, every control your radio supports, and radio presets from openRMS Rigs. For Linux, Raspberry Pi, macOS and Windows." />
</svelte:head>

<section class="mx-auto max-w-6xl px-4 pt-16 pb-10">
  <div class="grid items-center gap-10 lg:grid-cols-[1.1fr_1fr]">
    <div>
      <p class="text-primary mb-3 text-sm font-semibold tracking-wide uppercase">rigctlWeb</p>
      <h1 class="max-w-3xl text-4xl leading-tight font-semibold tracking-tight sm:text-5xl">
        Your radio's control panel, in any browser.
      </h1>
      <p class="text-muted-foreground mt-5 max-w-2xl text-lg">
        A free, open-source web panel for <a class="text-primary" href="https://hamlib.github.io/">Hamlib</a>'s rigctld,
        in the spirit of flrig. Tune, watch the meters and reach every control your radio supports from a
        phone, tablet or computer on your network.
      </p>
      <div class="mt-8 flex flex-wrap gap-3">
        <Button href="#download"><Download class="size-4" /> Download</Button>
        <Button variant="outline" href="/docs/install/">Install guide <ArrowRight class="size-4" /></Button>
      </div>
      <p class="text-muted-foreground mt-6 text-sm">
        Early days: the control surface works and the first release is out. Radio reports are very welcome on
        <a class="text-primary" href="{GITHUB_URL}/discussions">GitHub Discussions</a>.
      </p>
    </div>
    <figure class="my-0">
      <img src="/panel.jpg" width="979" height="1150" alt="The rigctlWeb panel: a large frequency display at 14.074 MHz, the S-meter, band and mode buttons, memory channels, the audio monitor and level sliders." class="max-h-[34rem] w-full rounded-lg border object-cover object-top shadow-sm" />
      <figcaption class="text-muted-foreground mt-2 text-center text-xs">The Everyday view, driving Hamlib's dummy radio.</figcaption>
    </figure>
  </div>
</section>

<section class="mx-auto max-w-6xl px-4 py-10" aria-labelledby="what">
  <h2 id="what" class="text-2xl font-semibold tracking-tight">What it does</h2>
  <div class="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
    {#each features as f}
      <Card.Root>
        <Card.Header>
          <div class="bg-primary-soft text-primary mb-2 flex size-9 items-center justify-center rounded-md">
            <f.icon class="size-5" />
          </div>
          <Card.Title class="text-base">{f.title}</Card.Title>
        </Card.Header>
        <Card.Content class="text-muted-foreground text-sm">{f.body}</Card.Content>
      </Card.Root>
    {/each}
  </div>
  <div class="mt-6 grid gap-4 sm:grid-cols-3">
    {#each more as f}
      <div class="flex gap-3">
        <f.icon class="text-primary mt-0.5 size-5 shrink-0" />
        <div>
          <p class="my-0 font-semibold">{f.title}</p>
          <p class="text-muted-foreground my-1 text-sm">{f.body}</p>
        </div>
      </div>
    {/each}
  </div>
</section>

<section class="mx-auto max-w-6xl px-4 py-10" aria-labelledby="download" id="download">
  <h2 class="text-2xl font-semibold tracking-tight">Download</h2>
  {#if rel}
    <p class="text-muted-foreground mt-2">
      {rel.tag}{rel.prerelease ? " (pre-release)" : ""}, published {new Date(rel.published).toISOString().slice(0, 10)}.
      <a class="text-primary" href={rel.url}>Release notes</a>
      {#if rel.sums}· <a class="text-primary" href={rel.sums.url}>SHA-256 checksums</a>{/if}
    </p>
    <div class="bg-card mt-5 divide-y rounded-lg border">
      {#each rel.downloads as d}
        <div class="flex flex-col gap-2 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p class="my-0 font-semibold">{d.label}</p>
            <p class="text-muted-foreground my-0 font-mono text-xs">{d.detail}</p>
          </div>
          <div class="flex flex-wrap gap-2">
            {#each d.assets as a}
              <Button variant="outline" size="sm" href={a.url} title={a.name}>
                {archOf(a.name)} <span class="text-muted-foreground text-xs">{mb(a.size)}</span>
              </Button>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  {:else}
    <p class="text-muted-foreground mt-2">
      See the <a class="text-primary" href="{GITHUB_URL}/releases">releases page</a>, or
      <a class="text-primary" href="/docs/install/#build-from-source">build it from source</a>.
    </p>
  {/if}
  <p class="text-muted-foreground mt-4 text-sm">
    You also need Hamlib's <code>rigctld</code> running for your radio. See
    <a class="text-primary" href="/docs/install/">Install</a> and
    <a class="text-primary" href="/docs/connecting/">Connecting a radio</a>.
  </p>
</section>

<section class="mx-auto max-w-6xl px-4 py-10" aria-labelledby="docs">
  <h2 id="docs" class="text-2xl font-semibold tracking-tight">Documentation</h2>
  <div class="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
    {#each docs as d}
      <a href="/docs/{d.slug}/" class="bg-card hover:border-primary block rounded-lg border p-4 no-underline">
        <p class="text-foreground my-0 font-semibold">{d.title}</p>
        <p class="text-muted-foreground my-1 text-sm">{d.description}</p>
      </a>
    {/each}
  </div>
</section>

<section class="mx-auto max-w-6xl px-4 py-10" aria-labelledby="free">
  <h2 id="free" class="text-2xl font-semibold tracking-tight">Free as in ham radio</h2>
  <p class="text-muted-foreground mt-2 max-w-2xl">
    No license keys, no paywalls, no accounts and no kill switches, ever. Competing and interoperating projects are
    welcome, and so are contributions: radio quirks and <code>dump_caps</code> reports especially. If it's useful to
    you, a tip is appreciated but never required.
  </p>
  <div class="mt-5 flex flex-wrap gap-3">
    <Button variant="outline" href={GITHUB_URL}>Source on GitHub <ArrowRight class="size-4" /></Button>
    <Button variant="outline" href="{GITHUB_URL}/blob/main/CONTRIBUTING.md">Contributing <ArrowRight class="size-4" /></Button>
    <Button variant="outline" href="https://buymeacoffee.com/ATCUSA"><Heart class="size-4" /> Buy me a coffee</Button>
    <Button variant="outline" href="https://ko-fi.com/ATCUSA"><Heart class="size-4" /> Ko-fi</Button>
  </div>
</section>
