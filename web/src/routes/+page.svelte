<script lang="ts">
  import ForgeDemo from "$lib/components/ForgeDemo.svelte";

  let copied = $state(false);

  const features = [
    {
      label: "CROSS-PLATFORM",
      title: "One pipeline, both forges",
      body: "A single annalist instance listens for GitHub Apps webhooks and Forgejo webhooks. Configure your sources once; every repo flows through the same forge.",
    },
    {
      label: "VOICE CONTROL",
      title: "A tone for every repo",
      body: "Pick chronicler, engineer, or launch — or write a custom persona — and tune temperature and instructions per repository, not globally.",
    },
    {
      label: "SELF-HOSTED",
      title: "Your stack, your data",
      body: "Point annalist at your own OpenAI-compatible LLM endpoint. Commits and generated notes never leave your infrastructure. No SaaS lock-in.",
    },
    {
      label: "ZERO-TOUCH",
      title: "Ship, and the notes follow",
      body: "When a release ships, annalist reads the commit history and writes the notes back — unattended, idempotent, and ready on every publish.",
    },
  ];

  const toneExamples = [
    {
      tone: "chronicler",
      blurb: "Neutral, factual — records what shipped and why.",
      notes: `## v1.4.0 — circuit breaker & staleness fixes

### Added
- Circuit-breaker middleware on the gateway (deduped by event hash).

### Fixed
- Rate-limit counters now reset on configuration reload.

### Notes
- Webhook auth headers documented. No breaking changes.`,
    },
    {
      tone: "engineer",
      blurb: "Terse, technical — the commit log, summarized.",
      notes: `v1.4.0

- gateway: circuit-breaker middleware; cascade-safe.
- rate-limit: reset counter on config reload (#128).
- metrics: squash histogram label cardinality.
- docs: webhook auth headers.`,
    },
    {
      tone: "launch",
      blurb: "Upbeat, outward-facing — built for the changelog.",
      notes: `# v1.4.0 is here 🔥

A more resilient, more honest release.

- **Smarter under pressure.** Circuit breaking stops one outage from becoming five.
- **Fresh quotas, fast.** Rate limits reset the instant you change your config.
- **Cleaner numbers.** Healthier histogram labels in your metrics collector.

No breaking changes — just upgrade and ship.`,
    },
  ];

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

<div class="min-h-dvh bg-page font-body text-ink">
  <!-- Hero -->
  <section class="mx-auto max-w-6xl px-6 pb-16 pt-12 sm:pb-24 sm:pt-20">
    <div class="grid gap-10 lg:grid-cols-2 lg:items-center">
      <div class="max-w-xl">
        <h1 class="font-display text-5xl leading-[0.95] tracking-tight text-white sm:text-6xl lg:text-7xl text-balance">
          RELEASE NOTES,<br />FORGED.
        </h1>
        <p class="mt-6 text-lg leading-relaxed text-ink-2 text-balance">
          Self-hosted AI release notes for GitHub and Forgejo. Annalist listens for
          release webhooks, reads your commit history, and writes human-sounding notes
          back — unattended.
        </p>
        <div class="mt-8 flex flex-wrap gap-4">
          <a
            href="/setup"
            class="inline-flex items-center gap-2 rounded bg-mark bg-gradient-to-r from-cherry via-ember to-heat px-5 py-3 text-sm font-bold text-page hover:brightness-110 focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
          >
            Get started
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
              <path d="M5 12h14M12 5l7 7-7 7" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </a>
          <a
            href="https://github.com/djdembeck/annalist"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex items-center gap-2 rounded border border-line bg-surface-1 px-5 py-3 text-sm font-medium text-ink hover:bg-surface-2 hover:border-line-strong focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.419-1.305.762-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12c0-6.627-5.373-12-12-12"/>
            </svg>
            View on GitHub
          </a>
        </div>
      </div>

      <ForgeDemo />
    </div>
  </section>

  <!-- Feature grid -->
  <section class="border-y border-line bg-surface-1">
    <div class="mx-auto max-w-6xl px-6 py-16 sm:py-20">
      <div class="max-w-2xl">
        <h2 class="text-3xl font-semibold leading-tight text-white text-balance">
          Every release is a heat. The notes are the shaped metal.
        </h2>
      </div>
      <div class="mt-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {#each features as f}
          <div class="rounded border border-line bg-surface-1-warm p-5">
            <span class="font-display text-xs tracking-wide text-ink-3">{f.label}</span>
            <h3 class="mt-3 font-semibold text-white">{f.title}</h3>
            <p class="mt-2 text-sm leading-relaxed text-ink-2">{f.body}</p>
          </div>
        {/each}
      </div>
    </div>
  </section>

  <!-- Tone examples -->
  <section class="mx-auto max-w-6xl px-6 py-16 sm:py-24">
    <div class="max-w-2xl">
      <h2 class="text-3xl font-semibold leading-tight text-white text-balance">
        One release, three voices.
      </h2>
      <p class="mt-4 text-ink-2">
        Set a tone per repo — chronicler, engineer, launch, or a custom persona. These
        are synthetic examples.
      </p>
    </div>
    <div class="mt-10 grid gap-4 lg:grid-cols-3">
      {#each toneExamples as t}
        <div class="flex flex-col rounded border border-line bg-surface-1">
          <div class="border-b border-line p-4">
            <div class="flex items-center justify-between gap-2">
              <span class="font-display text-sm capitalize text-white">{t.tone}</span>
              <span class="rounded bg-control px-2 py-0.5 text-xs font-medium tabular-nums text-ink-2">Synthetic example</span>
            </div>
            <p class="mt-2 text-xs text-ink-3">{t.blurb}</p>
          </div>
          <pre class="flex-1 whitespace-pre-wrap p-4 font-mono text-xs leading-relaxed text-ink-2">{t.notes}</pre>
        </div>
      {/each}
    </div>
  </section>

  <!-- Deploy -->
  <section id="deploy" class="border-y border-line bg-surface-1">
    <div class="mx-auto max-w-6xl px-6 py-16 sm:py-24">
      <div class="grid gap-10 lg:grid-cols-2 lg:items-start">
        <div>
          <p class="text-3xl font-semibold leading-tight text-white text-balance">Start with one command.</p>
          <p class="mt-4 max-w-md text-ink-2">
            Pull the image, set your admin token and LLM endpoint, and run. Then wire the
            examples pack to make every release ship with notes.
          </p>
          <a
            href="https://github.com/djdembeck/annalist/tree/main/examples"
            target="_blank"
            rel="noopener noreferrer"
            class="mt-6 inline-flex items-center gap-2 rounded border border-line bg-surface-2 px-4 py-2 text-sm font-medium text-ink hover:bg-control focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
              <path d="M14.9 3H6c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V8.3c0-.5-.2-1-.6-1.4l-4.7-4.7c-.4-.4-.9-.6-1.4-.6zM16 4.6l3.4 3.4H16V4.6zM6 19V5h8v5h5v9H6z"/>
            </svg>
            Read the examples pack
          </a>
        </div>

        <div class="rounded border border-line bg-surface-1 p-4">
          <div class="mb-3 flex items-center justify-between">
            <span class="font-display text-sm text-ink-3">Docker</span>
            <button
              onclick={copyDocker}
              class="inline-flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium text-ink-2 hover:bg-control hover:text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
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
          <pre class="overflow-x-auto rounded bg-page p-4 font-mono text-sm text-ink-2"><code>docker pull ghcr.io/djdembeck/annalist:latest</code></pre>
          <div class="mt-4">
            <p class="text-xs font-medium text-ink-3">Then set these environment variables:</p>
            <ul class="mt-2 space-y-2 text-sm">
              <li class="flex items-center gap-2">
                <span class="rounded bg-control px-1.5 py-0.5 font-mono text-xs text-ink">ADMIN_TOKEN</span>
                <span class="text-ink-2">— gate the UI and API</span>
              </li>
              <li class="flex items-center gap-2">
                <span class="rounded bg-control px-1.5 py-0.5 font-mono text-xs text-ink">LLM_BASE_URL</span>
                <span class="text-ink-2">— your OpenAI-compatible endpoint</span>
              </li>
              <li class="flex items-center gap-2">
                <span class="rounded bg-control px-1.5 py-0.5 font-mono text-xs text-ink">GITHUB_APP_ID</span>
                <span class="text-ink-2">or</span>
                <span class="rounded bg-control px-1.5 py-0.5 font-mono text-xs text-ink">FORGEJO_WEBHOOK_SECRET</span>
                <span class="text-ink-2">— platform credentials</span>
              </li>
            </ul>
          </div>
          <p class="mt-4 text-xs text-ink-3">
            Full credential setup for both platforms is in the README.
          </p>
        </div>
      </div>
    </div>
  </section>

  <!-- Footer -->
  <footer class="border-t border-line bg-page">
    <div class="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 px-6 py-8 sm:flex-row">
      <span class="font-display text-lg tracking-tight text-white">ANNALIST</span>
      <div class="flex items-center gap-6 text-sm text-ink-3">
        <a href="https://github.com/djdembeck/annalist" target="_blank" rel="noopener noreferrer" class="hover:text-ink-2">GitHub</a>
        <a href="https://github.com/djdembeck/annalist/tree/main/examples" target="_blank" rel="noopener noreferrer" class="hover:text-ink-2">Examples</a>
        <a href="/setup" class="hover:text-ink-2">Get started</a>
      </div>
      <span class="text-xs text-ink-3">UNLICENSED</span>
    </div>
  </footer>
</div>
