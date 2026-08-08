<script lang="ts">
  import ForgeDemo from "$lib/components/ForgeDemo.svelte";
  import MarkdownPreview from "$lib/components/MarkdownPreview.svelte";

  let selectedTone = $state("chronicler");

  const setupSteps = [
    {
      title: "Open setup",
      body: "Save the admin token that protects this instance, then connect GitHub or Forgejo.",
    },
    {
      title: "Choose repositories",
      body: "Give Annalist the repositories it can read and publish notes for.",
    },
    {
      title: "Set the tone",
      body: "Pick chronicler, engineer, launch, or your own instructions for the note.",
    },
  ];

  const releaseFlow = [
    { title: "A release ships", detail: "Your normal GitHub or Forgejo release flow continues." },
    { title: "The webhook arrives", detail: "Annalist receives the event and checks it once." },
    { title: "Commits become context", detail: "The configured repository history is read." },
    { title: "A note is written back", detail: "The selected tone shapes the release body." },
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
      notes: `# v1.4.0

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

  const selectedExample = $derived(
    toneExamples.find((tone) => tone.tone === selectedTone) ?? toneExamples[0],
  );

  function handleTabKeydown(event: KeyboardEvent) {
    const tab = (event.target as HTMLElement).closest("[role='tab']");
    if (!tab) return;

    const tabs = Array.from(
      (event.currentTarget as HTMLElement).querySelectorAll("[role='tab']"),
    ) as HTMLButtonElement[];
    const current = tabs.indexOf(tab as HTMLButtonElement);
    let next = current;

    switch (event.key) {
      case "ArrowRight": {
        event.preventDefault();
        next = (current + 1) % tabs.length;
        break;
      }
      case "ArrowLeft": {
        event.preventDefault();
        next = (current - 1 + tabs.length) % tabs.length;
        break;
      }
      case "Home": {
        event.preventDefault();
        next = 0;
        break;
      }
      case "End": {
        event.preventDefault();
        next = tabs.length - 1;
        break;
      }
    }

    if (next !== current) {
      tabs[next].focus();
      tabs[next].click();
    }
  }
</script>

<svelte:head>
  <title>Home · Annalist</title>
  <meta
    name="description"
    content="Configure Annalist once, then let each release arrive with a readable note."
  />
  <meta name="theme-color" content="#0b0f14" />
</svelte:head>

<div class="home-page min-h-dvh bg-page font-body text-ink">
  <!--
    THESIS: Turn the installed home into a calm first-run station, not a marketing landing page.
    OWN-WORLD: Dark powder-coated panels, copper actions, cyan trace signals, and thermal-paper note proofs.
    STORY: The operator knows what to configure, what happens next, and how tone changes the output.
    FIRST VIEWPORT: A setup-first headline and primary action sit beside the four-stage release path; detailed proof follows below.
    FORM: Release Trace Wall, adapted for Operate onboarding, seed aa4fba49.
    FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, and DESIGN.md
  -->
  <section class="home-hero mx-auto max-w-6xl px-4 pb-12 pt-10 sm:px-6 sm:pb-16 sm:pt-14 lg:pb-20 lg:pt-18">
    <div class="home-hero-grid">
      <div class="home-intro">
        <h1 class="home-title text-balance">Get the path from release to note ready.</h1>
        <p class="home-lede text-balance">
          Annalist is running on your infrastructure. Complete three short setup steps, then each
          release can arrive, pick up its configured tone, and leave with a readable note.
        </p>
        <div class="home-actions">
          <a href="/setup" class="btn btn-primary">
            Open setup
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
              <path d="M5 12h14M12 5l7 7-7 7" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </a>
          <a
            href="https://github.com/djdembeck/annalist#platform-setup"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-secondary"
          >
            Read the setup guide
          </a>
        </div>
      </div>

      <aside class="panel home-flow-panel" aria-labelledby="flow-heading">
        <div class="home-panel-head">
          <div>
            <h2 id="flow-heading">After setup</h2>
            <p>Nothing changes in the way your team ships.</p>
          </div>
          <span class="status status--quiet">READY WHEN YOU ARE</span>
        </div>
        <ol class="home-flow">
          {#each releaseFlow as stage, index}
            <li class="home-flow-item">
              <span class="home-flow-marker" aria-hidden="true">{index + 1}</span>
              <div>
                <h3>{stage.title}</h3>
                <p>{stage.detail}</p>
              </div>
            </li>
          {/each}
        </ol>
      </aside>
    </div>
  </section>

  <section class="home-section home-section--band border-y border-line bg-surface-1" aria-labelledby="steps-heading">
    <div class="mx-auto max-w-6xl px-4 py-12 sm:px-6 sm:py-16">
      <div class="home-section-heading">
        <h2 id="steps-heading">Three things to configure</h2>
        <p>Start here if this is the first time you have opened this Annalist instance.</p>
      </div>

      <div class="home-steps panel-soft">
        <ol>
          {#each setupSteps as item, index}
            <li class="home-step">
              <span class="home-step-number" aria-hidden="true">{index + 1}</span>
              <div class="home-step-copy">
                <h3>{item.title}</h3>
                <p>{item.body}</p>
              </div>
              {#if index === 0}
                <a href="/setup" class="btn btn-ghost">Begin <span aria-hidden="true">→</span></a>
              {:else}
                <a href="/setup" class="btn btn-ghost">In setup <span aria-hidden="true">→</span></a>
              {/if}
            </li>
          {/each}
        </ol>
      </div>

      <div class="home-expectations">
        <div>
          <h2>What to have nearby</h2>
          <p>Setup is easier when these are ready before you start.</p>
        </div>
        <ul>
          <li><strong>Admin token</strong><span>the key for this dashboard</span></li>
          <li><strong>Forge access</strong><span>a GitHub App or Forgejo token</span></li>
          <li><strong>LLM endpoint</strong><span>an OpenAI-compatible URL and key</span></li>
        </ul>
      </div>
    </div>
  </section>

  <section class="home-section mx-auto max-w-6xl px-4 py-12 sm:px-6 sm:py-16" aria-labelledby="preview-heading">
    <div class="home-section-heading home-section-heading--wide">
      <h2 id="preview-heading">See the release path before you connect it.</h2>
      <p>
        This preview shows the same sequence Annalist uses for a real release. Your repository history
        and configured tone replace the example content.
      </p>
    </div>
    <ForgeDemo />
  </section>

  <section class="home-section home-section--tone border-t border-line bg-surface-1" aria-labelledby="tone-heading">
    <div class="mx-auto max-w-6xl px-4 py-12 sm:px-6 sm:py-16">
      <div class="home-section-heading">
        <h2 id="tone-heading">Tone control belongs next to the repository.</h2>
        <p>Choose the shape of the note once globally, then override it where a repository needs a different register.</p>
      </div>

      <div class="tone-control">
        <div class="panel tone-picker">
          <div class="tone-picker-head">
            <h3>Choose a preview</h3>
            <span class="chip">Same release</span>
          </div>
          <div class="tone-tabs" role="tablist" tabindex="-1" aria-label="Tone previews" onkeydown={handleTabKeydown}>
            {#each toneExamples as tone}
              <button
                type="button"
                role="tab"
                id="tone-tab-{tone.tone}"
                aria-selected={tone.tone === selectedTone}
                aria-controls="tone-preview"
                tabindex={tone.tone === selectedTone ? 0 : -1}
                class:active={tone.tone === selectedTone}
                onclick={() => (selectedTone = tone.tone)}
              >
                <span>{tone.tone}</span>
                <small>{tone.blurb}</small>
              </button>
            {/each}
          </div>
          <a href="/setup" class="btn btn-secondary tone-picker-action">Set the tone in setup <span aria-hidden="true">→</span></a>
        </div>

        <div id="tone-preview" class="tone-preview" role="tabpanel" tabindex="0" aria-labelledby="tone-tab-{selectedTone}">
          <div class="tone-preview-head">
            <div>
              <span class="trace-label">{selectedExample.tone} tone</span>
              <p>{selectedExample.blurb}</p>
            </div>
            <span class="status status--quiet">PREVIEW</span>
          </div>
          <MarkdownPreview source={selectedExample.notes} class="home-markdown" />
        </div>
      </div>
    </div>
  </section>

  <footer class="home-footer border-t border-line bg-page">
    <div class="mx-auto flex max-w-6xl flex-col items-start justify-between gap-4 px-4 py-8 sm:flex-row sm:items-center sm:px-6">
      <div>
        <span class="font-display text-lg tracking-tight text-ink">ANNALIST</span>
        <p>Self-hosted release notes for GitHub and Forgejo.</p>
      </div>
      <div class="home-footer-links">
        <a href="/setup" class="focus-ring">Setup</a>
        <a href="https://github.com/djdembeck/annalist" target="_blank" rel="noopener noreferrer" class="focus-ring">GitHub</a>
        <a href="https://github.com/djdembeck/annalist/tree/main/examples" target="_blank" rel="noopener noreferrer" class="focus-ring">Examples</a>
      </div>
    </div>
  </footer>
</div>

<style>
  .home-page {
    overflow: hidden;
  }

  .home-hero-grid {
    display: grid;
    gap: 2rem;
    align-items: stretch;
  }

  .home-intro {
    display: flex;
    max-width: 42rem;
    flex-direction: column;
    justify-content: center;
  }

  .home-title {
    max-width: 12ch;
    color: var(--color-ink);
    font-family: var(--font-display);
    font-size: clamp(2.75rem, 7vw, 5.75rem);
    font-weight: 400;
    letter-spacing: -0.035em;
    line-height: 0.95;
  }

  .home-lede {
    max-width: 54ch;
    margin-top: 1.5rem;
    color: var(--color-ink-2);
    font-size: clamp(1rem, 1.7vw, 1.15rem);
    line-height: 1.55;
  }

  .home-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    margin-top: 1.75rem;
  }

  .home-flow-panel {
    min-width: 0;
  }

  .home-panel-head,
  .tone-picker-head,
  .tone-preview-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
  }

  .home-panel-head h2,
  .tone-picker h3 {
    color: var(--color-ink);
    font-size: 1.15rem;
    font-weight: 600;
    line-height: 1.2;
  }

  .home-panel-head p,
  .tone-preview-head p {
    margin-top: 0.4rem;
    color: var(--color-ink-3);
    font-size: 0.8rem;
    line-height: 1.4;
  }

  .home-flow {
    display: grid;
    gap: 0;
    margin-top: 1.5rem;
  }

  .home-flow-item {
    position: relative;
    display: grid;
    grid-template-columns: 2rem minmax(0, 1fr);
    gap: 0.75rem;
    min-width: 0;
    padding-bottom: 1.1rem;
  }

  .home-flow-item:last-child {
    padding-bottom: 0;
  }

  .home-flow-marker {
    position: relative;
    z-index: 1;
    display: grid;
    width: 2rem;
    height: 2rem;
    place-items: center;
    border: 1px solid var(--trace-cyan);
    border-radius: 50%;
    background: var(--trace-panel);
    color: var(--trace-cyan);
    font-family: var(--font-mono);
    font-size: 0.7rem;
  }

  .home-flow-item:not(:last-child) .home-flow-marker::after {
    position: absolute;
    top: 2rem;
    left: calc(50% - 0.5px);
    width: 1px;
    height: calc(100% + 1.1rem);
    background: var(--trace-line-strong);
    content: "";
  }

  .home-flow-item h3 {
    color: var(--color-ink);
    font-size: 0.95rem;
    font-weight: 600;
    line-height: 1.3;
  }

  .home-flow-item p {
    margin-top: 0.25rem;
    color: var(--color-ink-2);
    font-size: 0.82rem;
    line-height: 1.45;
  }

  .home-section-heading {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 1.5rem;
    margin-bottom: 1.5rem;
  }

  .home-section-heading h2,
  .home-expectations h2 {
    max-width: 25ch;
    color: var(--color-ink);
    font-size: clamp(1.55rem, 3vw, 2.15rem);
    font-weight: 600;
    letter-spacing: -0.02em;
    line-height: 1.05;
  }

  .home-section-heading p,
  .home-expectations > div p {
    max-width: 54ch;
    margin-top: 0.65rem;
    color: var(--color-ink-2);
    line-height: 1.55;
  }

  .home-steps {
    padding: 0;
  }

  .home-step {
    display: grid;
    grid-template-columns: 2.25rem minmax(0, 1fr) auto;
    gap: 1rem;
    align-items: center;
    min-width: 0;
    padding: 1.25rem 1.5rem;
  }

  .home-step + .home-step {
    border-top: 1px solid var(--trace-line);
  }

  .home-step-number {
    display: grid;
    width: 2.25rem;
    height: 2.25rem;
    place-items: center;
    border: 1px solid var(--trace-line-strong);
    border-radius: 50%;
    color: var(--trace-cyan);
    font-family: var(--font-mono);
    font-size: 0.75rem;
  }

  .home-step-copy {
    min-width: 0;
  }

  .home-step-copy h3 {
    color: var(--color-ink);
    font-size: 1rem;
    font-weight: 600;
    line-height: 1.3;
  }

  .home-step-copy p {
    max-width: 58ch;
    margin-top: 0.3rem;
    color: var(--color-ink-2);
    font-size: 0.875rem;
    line-height: 1.5;
  }

  .home-step .btn {
    white-space: nowrap;
  }

  .home-expectations {
    display: grid;
    grid-template-columns: minmax(0, 0.8fr) minmax(0, 1.2fr);
    gap: 2rem;
    align-items: start;
    margin-top: 2rem;
    padding-top: 2rem;
    border-top: 1px solid var(--trace-line);
  }

  .home-expectations h2 {
    font-size: 1.25rem;
  }

  .home-expectations ul {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 1rem;
  }

  .home-expectations li {
    display: grid;
    gap: 0.3rem;
    min-width: 0;
    padding-top: 0.75rem;
    border-top: 2px solid var(--trace-line-strong);
  }

  .home-expectations strong {
    color: var(--color-ink);
    font-size: 0.9rem;
    font-weight: 600;
  }

  .home-expectations li span {
    color: var(--color-ink-2);
    font-size: 0.8rem;
    line-height: 1.4;
  }

  .home-section-heading--wide {
    display: block;
  }

  .home-section-heading--wide p {
    max-width: 62ch;
  }

  .home-section--tone {
    padding-bottom: 1rem;
  }

  .tone-control {
    display: grid;
    grid-template-columns: minmax(15rem, 0.72fr) minmax(0, 1.28fr);
    gap: 1rem;
    align-items: stretch;
  }

  .tone-picker {
    min-width: 0;
  }

  .tone-tabs {
    display: grid;
    gap: 0.5rem;
    margin-top: 1.25rem;
  }

  .tone-tabs button {
    display: grid;
    gap: 0.3rem;
    min-height: 2.75rem;
    border: 1px solid var(--trace-line);
    border-radius: 0.25rem;
    background: var(--trace-page);
    padding: 0.75rem;
    color: var(--color-ink-2);
    font-family: inherit;
    text-align: left;
    transition: background-color 140ms ease, border-color 140ms ease, color 140ms ease;
  }

  .tone-tabs button:hover {
    border-color: var(--trace-line-strong);
    background: var(--trace-raised);
    color: var(--color-ink);
  }

  .tone-tabs button:focus-visible {
    outline: 2px solid var(--trace-cyan);
    outline-offset: 3px;
  }

  .tone-tabs button.active {
    border-color: var(--trace-cyan);
    background: color-mix(in srgb, var(--trace-cyan) 8%, var(--trace-panel));
    color: var(--color-ink);
  }

  .tone-tabs button span {
    font-size: 0.9rem;
    font-weight: 600;
    text-transform: capitalize;
  }

  .tone-tabs button small {
    color: var(--color-ink-3);
    font-size: 0.76rem;
    line-height: 1.4;
  }

  .tone-picker-action {
    width: 100%;
    margin-top: 1.25rem;
  }

  .tone-preview {
    min-width: 0;
    border: 1px solid var(--trace-paper-edge);
    border-radius: 0.25rem;
    background: var(--trace-ink);
    padding: 1rem;
  }

  .tone-preview-head {
    align-items: center;
    padding-bottom: 0.85rem;
    border-bottom: 1px solid var(--trace-page);
  }

  .tone-preview-head .trace-label {
    color: var(--trace-page);
  }

  .tone-preview-head p {
    color: color-mix(in srgb, var(--trace-page) 72%, var(--trace-ink));
  }

  .tone-preview-head .status {
    color: var(--trace-page);
  }

  :global(.home-markdown) {
    margin-top: 1rem;
    border-color: var(--trace-page) !important;
    background: transparent !important;
    padding: 0 !important;
  }

  :global(.home-markdown) :global(.prose-forge) {
    max-height: 22rem;
    padding: 0.25rem 0 0;
  }

  :global(.home-markdown) :global(.prose-forge *) {
    color: var(--trace-page);
  }

  :global(.home-markdown) :global(.prose-forge h2) {
    border-color: color-mix(in srgb, var(--trace-page) 30%, transparent);
  }

  :global(.home-markdown) :global(.prose-forge li) {
    color: color-mix(in srgb, var(--trace-page) 78%, var(--trace-ink));
  }

  :global(.home-markdown) :global(.prose-forge li::marker) {
    color: var(--trace-copper);
  }

  :global(.home-markdown) :global(.prose-forge code) {
    background: color-mix(in srgb, var(--trace-page) 12%, var(--trace-ink));
  }

  .home-footer {
    color: var(--color-ink-3);
  }

  .home-footer p {
    margin-top: 0.35rem;
    font-size: 0.75rem;
  }

  .home-footer-links {
    display: flex;
    flex-wrap: wrap;
    gap: 1.25rem;
    font-size: 0.875rem;
  }

  .home-footer-links a {
    display: inline-flex;
    min-height: 2.75rem;
    align-items: center;
    color: var(--color-ink-3);
  }

  .home-footer-links a:hover {
    color: var(--color-ink);
  }

  @media (min-width: 768px) {
    .home-hero-grid {
      grid-template-columns: minmax(0, 1.08fr) minmax(18rem, 0.92fr);
      gap: 3.5rem;
    }
  }

  @media (min-width: 1024px) {
    .home-hero-grid {
      gap: 5rem;
    }
  }

  @media (max-width: 760px) {
    .home-step {
      grid-template-columns: 2.25rem minmax(0, 1fr);
    }

    .home-step .btn {
      grid-column: 2;
      justify-self: start;
    }

    .home-expectations,
    .tone-control {
      grid-template-columns: minmax(0, 1fr);
    }

    .home-expectations ul {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  @media (max-width: 639px) {
    .home-actions .btn {
      width: 100%;
    }

    .home-section-heading {
      display: block;
    }

    .home-expectations ul {
      grid-template-columns: minmax(0, 1fr);
    }

    .home-step {
      padding-inline: 1rem;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .tone-tabs button {
      transition: none;
    }
  }
</style>
