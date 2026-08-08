<script lang="ts">
  import ForgeDemo from "$lib/components/ForgeDemo.svelte";

  let copied = $state(false);
  let copyError = $state(false);

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
      notes: `# v1.4.0 is here

A more resilient, more honest release.

- **Smarter under pressure.** Circuit breaking stops one outage from becoming five.
- **Fresh quotas, fast.** Rate limits reset the instant you change your config.
- **Cleaner numbers.** Healthier histogram labels in your metrics collector.

No breaking changes — just upgrade and ship.`,
    },
  ];

  async function copyDocker(): Promise<void> {
    const command = "docker pull ghcr.io/djdembeck/annalist:latest";
    copyError = false;
    try {
      await navigator.clipboard.writeText(command);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      copied = false;
      copyError = true;
      setTimeout(() => (copyError = false), 4000);
    }
  }
</script>

<div class="release-wall min-h-dvh bg-page font-body text-ink">
  <!--
    THESIS: Make every release legible as a traceable workpiece, not an interchangeable admin dashboard.
    OWN-WORLD: Dark powder-coated panels, thermal-paper proof, copper action rails, and cyan trace signals.
    STORY: The operator sees commits received, resolved, struck into a voice, and ready to publish as a note.
    FIRST VIEWPORT: A release trace fills the primary field; its synthetic note proof and Get started action sit alongside it, then stack on mobile.
    FORM: Release Trace Wall, seed aa4fba49.
    FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, and DESIGN.md
  -->
  <section class="landing-hero mx-auto max-w-6xl px-4 pb-14 pt-10 sm:px-6 sm:pb-20 sm:pt-16 lg:pb-24 lg:pt-20">
    <div class="landing-hero-grid">
      <div class="landing-intro">
        <h1 class="landing-title text-balance">Turn release commits into release notes.</h1>
        <p class="landing-lede text-balance">
          Annalist listens for release webhooks on GitHub and Forgejo, reads your commit history,
          and writes human-sounding notes back to the release — unattended.
        </p>
        <div class="landing-actions">
          <a href="/setup" class="btn btn-primary">
            Get started
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
              <path d="M5 12h14M12 5l7 7-7 7" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </a>
          <a
            href="https://github.com/djdembeck/annalist"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-secondary"
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.419-1.305.762-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12c0-6.627-5.373-12-12-12"/>
            </svg>
            View on GitHub
          </a>
        </div>
        <p class="landing-proof-note"><span class="signal-dot signal-dot-cyan" aria-hidden="true"></span>Self-hosted AI release notes for teams that ship on their own infrastructure.</p>
      </div>

      <ForgeDemo />
    </div>
  </section>

  <section class="wall-section border-y border-line bg-surface-1">
    <div class="mx-auto max-w-6xl px-4 py-14 sm:px-6 sm:py-20">
      <div class="section-head">
        <div>
          <h2>One release, one traceable pass.</h2>
          <p>From webhook to release body, the work stays on your stack and the writing voice stays yours.</p>
        </div>
      </div>
      <div class="console-grid feature-console">
        {#each features as feature}
          <article class="panel-soft feature-panel">
            <h3>{feature.title}</h3>
            <p>{feature.body}</p>
            <span class="chip">{feature.label}</span>
          </article>
        {/each}
      </div>
    </div>
  </section>

  <section class="wall-section mx-auto max-w-6xl px-4 py-14 sm:px-6 sm:py-24">
    <div class="section-head">
      <div>
        <h2>The voice is a setting, not a template.</h2>
        <p>Choose chronicler, engineer, launch, or a custom persona per repository. These note samples are synthetic examples.</p>
      </div>
    </div>
    <div class="console-grid tone-console">
      {#each toneExamples as tone}
        <article class="panel tone-panel">
          <div class="tone-panel-head">
            <div>
              <h3>{tone.tone}</h3>
              <p>{tone.blurb}</p>
            </div>
            <span class="chip">Synthetic</span>
          </div>
          <pre class="note-paper">{tone.notes}</pre>
        </article>
      {/each}
    </div>
  </section>

  <section id="deploy" class="wall-section border-y border-line bg-surface-1">
    <div class="mx-auto max-w-6xl px-4 py-14 sm:px-6 sm:py-24">
      <div class="deploy-grid">
        <div class="deploy-copy">
          <h2>Run it next to the systems that ship.</h2>
          <p>
            Pull the image, set your admin token and LLM endpoint, and run. Then wire the
            examples pack to make every release ship with notes.
          </p>
          <a
            href="https://github.com/djdembeck/annalist/tree/main/examples"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-secondary"
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
              <path d="M14.9 3H6c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V8.3c0-.5-.2-1-.6-1.4l-4.7-4.7c-.4-.4-.9-.6-1.4-.6zM16 4.6l3.4 3.4H16V4.6zM6 19V5h8v5h5v9H6Z" />
            </svg>
            Read the examples pack
          </a>
        </div>

        <div class="panel deploy-console">
          <div class="deploy-console-head">
            <span class="trace-label">Docker command</span>
            <button type="button" onclick={copyDocker} class="btn btn-ghost" aria-live="polite">
              {#if copied}
                <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
                  <path d="M20 6 9 17l-5-5" stroke-linecap="round" stroke-linejoin="round" />
                </svg>
                Copied
              {:else}
                <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
                  <rect x="9" y="9" width="13" height="13" rx="2" />
                  <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" stroke-linecap="round" stroke-linejoin="round" />
                </svg>
                Copy
              {/if}
            </button>
          </div>
          <pre class="command-line"><code>docker pull ghcr.io/djdembeck/annalist:latest</code></pre>
          {#if copyError}
            <p class="mt-2 text-xs text-alert" role="status">Copy unavailable — select the command above.</p>
          {/if}
          <div class="deploy-env">
            <p class="trace-label">Then set these environment variables</p>
            <ul>
              <li><span class="chip">ADMIN_TOKEN</span><span>Gate the UI and API.</span></li>
              <li><span class="chip">LLM_BASE_URL</span><span>Your OpenAI-compatible endpoint.</span></li>
              <li><span class="chip">GITHUB_APP_ID</span><span>or</span><span class="chip">FORGEJO_WEBHOOK_SECRET</span><span>Platform credentials.</span></li>
            </ul>
          </div>
          <p class="deploy-footnote">Full credential setup for both platforms is in the README.</p>
        </div>
      </div>
    </div>
  </section>

  <footer class="wall-footer border-t border-line bg-page">
    <div class="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 px-4 py-8 sm:flex-row sm:px-6">
      <span class="font-display text-lg tracking-tight text-white">ANNALIST</span>
      <div class="flex items-center gap-6 text-sm text-ink-3">
        <a href="https://github.com/djdembeck/annalist" target="_blank" rel="noopener noreferrer" class="focus-ring hover:text-ink-2">GitHub</a>
        <a href="https://github.com/djdembeck/annalist/tree/main/examples" target="_blank" rel="noopener noreferrer" class="focus-ring hover:text-ink-2">Examples</a>
        <a href="/setup" class="focus-ring hover:text-ink-2">Get started</a>
      </div>
      <span class="text-xs text-ink-3">UNLICENSED</span>
    </div>
  </footer>
</div>

<style>
  .release-wall {
    overflow: hidden;
  }

  .landing-hero-grid {
    display: grid;
    gap: 2.5rem;
    align-items: center;
  }

  .landing-intro {
    max-width: 36rem;
  }

  .landing-title {
    max-width: 10ch;
    color: var(--color-ink);
    font-family: var(--font-display);
    font-size: clamp(2.75rem, 7vw, 5.75rem);
    font-weight: 400;
    letter-spacing: -0.035em;
    line-height: 0.95;
  }

  .landing-lede {
    max-width: 36rem;
    margin-top: 1.5rem;
    color: var(--color-ink-2);
    font-size: clamp(1rem, 1.7vw, 1.2rem);
    line-height: 1.55;
  }

  .landing-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    margin-top: 1.75rem;
  }

  .landing-proof-note {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 1.25rem;
    color: var(--color-ink-3);
    font-size: 0.75rem;
    line-height: 1.4;
  }

  .wall-section h2,
  .deploy-copy h2 {
    max-width: 22ch;
    color: var(--color-ink);
    font-size: clamp(1.6rem, 3vw, 2.25rem);
    font-weight: 600;
    letter-spacing: -0.02em;
    line-height: 1.05;
  }

  .section-head {
    display: flex;
    justify-content: space-between;
    gap: 1.5rem;
  }

  .section-head p,
  .deploy-copy p {
    max-width: 58ch;
    margin-top: 0.75rem;
    color: var(--color-ink-2);
    line-height: 1.55;
  }

  .feature-console {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    margin-top: 2.5rem;
  }

  .feature-panel {
    display: flex;
    flex-direction: column;
    min-height: 13rem;
  }

  .feature-panel h3,
  .tone-panel h3 {
    color: var(--color-ink);
    font-size: 1rem;
    font-weight: 600;
    line-height: 1.3;
  }

  .feature-panel p {
    margin-top: 0.65rem;
    color: var(--color-ink-2);
    font-size: 0.9rem;
    line-height: 1.55;
  }

  .feature-panel .chip {
    align-self: flex-start;
    margin-top: auto;
  }

  .tone-console {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    margin-top: 2.5rem;
  }

  .tone-panel {
    display: flex;
    min-width: 0;
    flex-direction: column;
    gap: 1rem;
  }

  .tone-panel-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
  }

  .tone-panel-head h3 {
    text-transform: capitalize;
  }

  .tone-panel-head p {
    margin-top: 0.35rem;
    color: var(--color-ink-3);
    font-size: 0.8rem;
    line-height: 1.4;
  }

  .tone-panel .note-paper {
    min-height: 14rem;
    flex: 1;
    overflow: auto;
  }

  .deploy-grid {
    display: grid;
    gap: 2.5rem;
    align-items: start;
  }

  .deploy-copy {
    max-width: 34rem;
  }

  .deploy-copy .btn {
    margin-top: 1.5rem;
  }

  .deploy-console {
    min-width: 0;
  }

  .deploy-console-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }

  .command-line {
    overflow-x: auto;
    margin-top: 0.75rem;
    padding: 0.9rem 1rem;
    border: 1px solid var(--color-line);
    background: var(--color-page);
    color: var(--color-ink-2);
    font-family: var(--font-mono);
    font-size: 0.8rem;
    line-height: 1.5;
    white-space: nowrap;
  }

  .deploy-env {
    margin-top: 1.25rem;
  }

  .deploy-env ul {
    display: grid;
    gap: 0.6rem;
    margin-top: 0.65rem;
  }

  .deploy-env li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.45rem;
    color: var(--color-ink-2);
    font-size: 0.8rem;
    line-height: 1.4;
  }

  .deploy-footnote {
    margin-top: 1.25rem;
    color: var(--color-ink-3);
    font-size: 0.75rem;
    line-height: 1.45;
  }

  .wall-footer a {
    display: inline-flex;
    min-height: 2.75rem;
    align-items: center;
    padding-inline: var(--trace-space-2);
    transition: color 120ms ease;
  }

  @media (min-width: 768px) {
    .landing-hero-grid {
      grid-template-columns: minmax(15rem, 0.76fr) minmax(0, 1.24fr);
      gap: 3rem;
    }

    .deploy-grid {
      grid-template-columns: minmax(0, 0.8fr) minmax(24rem, 1.2fr);
      gap: 4rem;
    }
  }

  @media (min-width: 1024px) {
    .landing-hero-grid {
      gap: 4.5rem;
    }
  }

  @media (max-width: 639px) {
    .feature-console,
    .tone-console {
      grid-template-columns: minmax(0, 1fr);
    }

    .landing-actions .btn {
      width: 100%;
    }

    .section-head p,
    .deploy-copy p {
      font-size: 0.95rem;
    }
  }
</style>
