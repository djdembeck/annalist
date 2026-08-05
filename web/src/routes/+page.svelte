<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { getToken } from "$lib/api";
  import ForgeDemo from "$lib/components/ForgeDemo.svelte";

  let copied = $state(false);

  onMount(() => {
    if (getToken()) {
      goto("/repos");
      return;
    }
  });

  async function copyDocker(): Promise<void> {
    const command = "docker pull ghcr.io/djdembeck/annalist:latest";
    try {
      await navigator.clipboard.writeText(command);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      // Ignore unsupported contexts.
    }
  }
</script>

<div class="min-h-dvh bg-forge-page font-body text-forge-ink">
  <header class="border-b border-forge-line bg-forge-page/90 backdrop-blur-sm">
    <div class="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
      <a href="/" class="font-display text-xl tracking-tight text-forge-white">ANNALIST</a>
      <a
        href="/setup"
        class="rounded bg-forge-control px-3 py-1.5 text-sm font-medium text-forge-ink hover:bg-forge-control-hover"
      >
        Dashboard
      </a>
    </div>
  </header>

  <section class="mx-auto max-w-6xl px-6 pb-16 pt-12 sm:pb-24 sm:pt-20">
    <div class="grid gap-10 lg:grid-cols-2 lg:items-center">
      <div class="max-w-xl">
        <h1 class="font-display text-5xl leading-[0.95] tracking-tight text-forge-white sm:text-6xl lg:text-7xl text-balance">
          RELEASE NOTES,<br />FORGED.
        </h1>
        <p class="mt-6 text-lg leading-relaxed text-forge-ink-2 text-balance">
          Annalist listens for release webhooks, reads your commit history, and writes
          human-sounding release notes back into GitHub and Forgejo — unattended.
        </p>
        <div class="mt-8 flex flex-wrap gap-4">
          <a
            href="#deploy"
            class="inline-flex items-center gap-2 rounded bg-forge-ember bg-gradient-to-r from-forge-cherry via-forge-ember to-forge-heat px-5 py-3 text-sm font-bold text-forge-page hover:brightness-110"
          >
            Deploy with Docker
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
              <path d="M5 12h14M12 5l7 7-7 7" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </a>
          <a
            href="https://github.com/djdembeck/annalist"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex items-center gap-2 rounded border border-forge-line bg-forge-surface px-5 py-3 text-sm font-medium text-forge-ink hover:bg-forge-surface-2 hover:border-forge-line-strong"
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.419-1.305.762-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/>
            </svg>
            View on GitHub
          </a>
        </div>
      </div>

      <ForgeDemo />
    </div>
  </section>

  <section class="border-y border-forge-line bg-forge-surface">
    <div class="mx-auto max-w-6xl px-6 py-12 sm:py-16">
      <p class="max-w-xl text-2xl font-semibold leading-tight text-forge-white text-balance">
        Every release is a heat. Every commit summary is a blow. The notes are the shaped metal.
      </p>

      <div class="mt-10 divide-y divide-forge-line border-t border-forge-line">
        <div class="grid gap-2 py-6 sm:grid-cols-[140px_1fr_auto] sm:items-center">
          <span class="font-display text-lg text-forge-white">01 — RECEIVE</span>
          <span class="text-forge-ink-2">A release webhook fires from GitHub or Forgejo, or your CI calls annalist directly.</span>
          <span class="text-xs tabular-nums text-forge-ink-3">DEDUP BY EVENT HASH</span>
        </div>
        <div class="grid gap-2 py-6 sm:grid-cols-[140px_1fr_auto] sm:items-center">
          <span class="font-display text-lg text-forge-white">02 — RESOLVE</span>
          <span class="text-forge-ink-2">Per-repo settings merge with global defaults: tone, model, temperature, instructions.</span>
          <span class="text-xs tabular-nums text-forge-ink-3">RESOLVED EFFECTIVE</span>
        </div>
        <div class="grid gap-2 py-6 sm:grid-cols-[140px_1fr_auto] sm:items-center">
          <span class="font-display text-lg text-forge-white">03 — STRIKE</span>
          <span class="text-forge-ink-2">An OpenAI-compatible LLM turns the commit log into release notes, then annalist publishes them back.</span>
          <span class="text-xs tabular-nums text-forge-ink-3">IDEMPOTENT MARKER</span>
        </div>
      </div>
    </div>
  </section>

  <section id="deploy" class="mx-auto max-w-6xl px-6 py-16 sm:py-24">
    <div class="grid gap-10 lg:grid-cols-2 lg:items-start">
      <div>
        <p class="text-3xl font-semibold leading-tight text-forge-white text-balance">Start with one command.</p>
        <p class="mt-4 max-w-md text-forge-ink-2">
          Pull the image, set <code class="rounded bg-forge-surface px-1 py-0.5 text-sm text-forge-ink">ADMIN_TOKEN</code> and your LLM endpoint, and run. Then wire the examples pack to make every release ship with notes.
        </p>
        <a
          href="https://github.com/djdembeck/annalist/tree/main/examples"
          target="_blank"
          rel="noopener noreferrer"
          class="mt-6 inline-flex items-center gap-2 rounded border border-forge-line bg-forge-surface px-4 py-2 text-sm font-medium text-forge-ink hover:bg-forge-surface-2"
        >
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <path d="M14.9 3H6c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V8.3c0-.5-.2-1-.6-1.4l-4.7-4.7c-.4-.4-.9-.6-1.4-.6zM16 4.6l3.4 3.4H16V4.6zM6 19V5h8v5h5v9H6z"/>
          </svg>
          Read the examples pack
        </a>
      </div>

      <div class="rounded border border-forge-line bg-forge-surface p-4">
        <div class="mb-3 flex items-center justify-between">
          <span class="font-display text-sm text-forge-ink-3">Docker</span>
          <button
            onclick={copyDocker}
            class="inline-flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium text-forge-ink-2 hover:bg-forge-control hover:text-forge-ink"
            aria-live="polite"
          >
            {#if copied}
              <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
                <path d="M20 6L9 17l-5-5" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
              Copied
            {:else}
              <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
                <rect x="9" y="9" width="13" height="13" rx="2"/>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
              Copy
            {/if}
          </button>
        </div>
        <pre class="overflow-x-auto rounded bg-forge-page p-4 font-mono text-sm text-forge-ink-2"><code>docker pull ghcr.io/djdembeck/annalist:latest</code></pre>
        <p class="mt-3 text-xs text-forge-ink-3">
          Then run it with <code class="rounded bg-forge-page px-1">ADMIN_TOKEN</code>, <code class="rounded bg-forge-page px-1">LLM_BASE_URL</code>, and the platform credentials listed in the README.
        </p>
      </div>
    </div>
  </section>

  <footer class="border-t border-forge-line bg-forge-surface">
    <div class="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 px-6 py-8 sm:flex-row">
      <span class="font-display text-lg tracking-tight text-forge-white">ANNALIST</span>
      <div class="flex items-center gap-6 text-sm text-forge-ink-3">
        <a href="https://github.com/djdembeck/annalist" target="_blank" rel="noopener noreferrer" class="hover:text-forge-ink-2">GitHub</a>
        <a href="https://github.com/djdembeck/annalist/tree/main/examples" target="_blank" rel="noopener noreferrer" class="hover:text-forge-ink-2">Examples</a>
      </div>
      <span class="text-xs text-forge-ink-3">UNLICENSED</span>
    </div>
  </footer>
</div>
