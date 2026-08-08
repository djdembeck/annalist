<script lang="ts">
  import { onMount } from "svelte";

  const commits = [
    "feat(gateway): add circuit-breaker middleware",
    "fix(rate-limit): reset counter on config reload",
    "docs: describe webhook auth headers",
    "refactor(metrics): consolidate histogram labels",
  ];

  const stages = [
    {
      label: "Receive",
      detail: "release webhook accepted",
    },
    {
      label: "Resolve",
      detail: "commit subjects gathered",
    },
    {
      label: "Strike",
      detail: "voice applied to the note",
    },
  ];

  const output = `## What's new

- Added circuit-breaker middleware to the gateway so cascading failures trip fast instead of saturating downstream services.
- Rate-limit counters now reset when the configuration reloads, preventing stale quotas after a rules change.

## Fixes

- Corrected histogram label cardinality in the metrics collector.

## Notes

- Webhook authentication headers are now documented in the API reference. No breaking changes to existing integrations.`;

  let activeStage = $state(0);
  let paused = $state(false);
  let reducedMotion = $state(false);

  onMount(() => {
    const media = window.matchMedia("(prefers-reduced-motion: reduce)");
    const syncMotion = () => {
      reducedMotion = media.matches;
      if (reducedMotion) activeStage = stages.length - 1;
    };
    const syncVisibility = () => {
      paused = document.hidden;
    };

    syncMotion();
    syncVisibility();
    media.addEventListener("change", syncMotion);
    document.addEventListener("visibilitychange", syncVisibility);

    const interval = window.setInterval(() => {
      if (paused || reducedMotion) return;
      activeStage = (activeStage + 1) % stages.length;
    }, 1800);

    return () => {
      window.clearInterval(interval);
      media.removeEventListener("change", syncMotion);
      document.removeEventListener("visibilitychange", syncVisibility);
    };
  });
</script>

<section
  class="panel forge-demo"
  class:trace-paused={paused}
  class:trace-reduced={reducedMotion}
  aria-labelledby="trace-title"
>
  <header class="forge-demo-head">
    <div>
      <p class="trace-label">Synthetic release trace / v1.4.0</p>
      <h2 id="trace-title">A release note taking shape.</h2>
    </div>
    <span class="status" data-tone="signal">{reducedMotion ? "Settled" : "Tracing"}</span>
  </header>

  <div class="forge-map">
    <div class="trace-input panel-soft">
      <div class="trace-input-head">
        <h3>Commit subjects</h3>
        <span class="chip">4 received</span>
      </div>
      <ul class="commit-list" aria-label="Synthetic commit subjects">
        {#each commits as commit, index}
          <li class:commit-seen={activeStage >= 1 || reducedMotion}>
            <span class="signal-dot signal-dot-heat" aria-hidden="true"></span>
            <code>{commit}</code>
            <span class="sr-only">Synthetic commit {index + 1}</span>
          </li>
        {/each}
      </ul>
    </div>

    <div class="trace-pipeline">
      <div class="trace-pipeline-head">
        <span class="trace-label">Pipeline</span>
        <span class="trace-pipeline-state">{stages[activeStage].detail}</span>
      </div>
      <ol class="trace-rail" aria-label="Release processing stages">
        {#each stages as stage, index}
          <li class="trace-node" class:active={activeStage >= index} class:current={activeStage === index}>
            <span class="trace-node-index" aria-hidden="true">{index + 1}</span>
            <span class="trace-node-copy">
              <strong>{stage.label}</strong>
              <small>{stage.detail}</small>
            </span>
          </li>
        {/each}
        <span class="signal-dot signal-dot-cyan trace-signal" aria-hidden="true"></span>
      </ol>
    </div>
  </div>

  <article class="note-paper trace-proof" aria-labelledby="proof-title">
    <header class="trace-proof-head">
      <div>
        <p class="trace-label">Shaped note proof</p>
        <h3 id="proof-title">v1.4.0 — circuit breaker &amp; staleness fixes</h3>
      </div>
      <span class="chip">Synthetic output</span>
    </header>
    <pre>{output}</pre>
  </article>

  <p class="trace-caption">
    This demonstration is synthetic. Annalist uses your configured tone and repository history when a real release arrives.
  </p>
</section>

<style>
  .forge-demo {
    min-width: 0;
  }

  .forge-demo-head,
  .trace-input-head,
  .trace-pipeline-head,
  .trace-proof-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
  }

  .forge-demo-head h2 {
    max-width: 18ch;
    margin-top: 0.35rem;
    color: var(--color-ink);
    font-size: clamp(1.35rem, 3vw, 1.9rem);
    font-weight: 600;
    letter-spacing: -0.02em;
    line-height: 1.05;
  }

  .forge-demo .status {
    flex: 0 0 auto;
    color: var(--trace-cyan);
  }

  .signal-dot-heat {
    color: var(--trace-heat);
  }

  .forge-map {
    display: grid;
    gap: 1rem;
    margin-top: 1.5rem;
  }

  .trace-input {
    min-width: 0;
  }

  .trace-input h3 {
    color: var(--color-ink);
    font-size: 0.9rem;
    font-weight: 600;
  }

  .commit-list {
    display: grid;
    gap: 0.65rem;
    margin-top: 1rem;
  }

  .commit-list li {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    align-items: start;
    gap: 0.6rem;
    min-width: 0;
    color: var(--color-ink-2);
    font-size: 0.72rem;
    line-height: 1.45;
  }

  .commit-list code {
    overflow-wrap: anywhere;
    font-family: var(--font-mono);
  }

  .commit-list .signal-dot {
    margin-top: 0.3rem;
    opacity: 0.62;
    transition: opacity 180ms ease;
  }

  .commit-list li.commit-seen .signal-dot {
    opacity: 1;
  }

  .trace-pipeline {
    min-width: 0;
    padding: 0.25rem 0;
  }

  .trace-pipeline-state {
    max-width: 16ch;
    color: var(--color-ink-3);
    font-family: var(--font-mono);
    font-size: 0.68rem;
    line-height: 1.35;
    text-align: right;
  }

  .trace-rail {
    position: relative;
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.65rem;
    margin-top: 1.5rem;
    padding: 0;
    list-style: none;
  }

  .trace-rail::before {
    position: absolute;
    top: 0.85rem;
    right: 1.1rem;
    left: 1.1rem;
    height: 1px;
    background: var(--color-line-strong);
    content: "";
  }

  .trace-node {
    position: relative;
    z-index: 1;
    display: flex;
    width: auto;
    height: auto;
    min-width: 0;
    flex-direction: column;
    gap: 0.55rem;
    border: 0;
    border-radius: 0;
    background: transparent;
    color: var(--color-ink-3);
  }

  .trace-node-index {
    display: grid;
    width: 1.7rem;
    height: 1.7rem;
    place-items: center;
    border: 1px solid var(--color-line-strong);
    border-radius: 9999px;
    background: var(--color-page);
    font-family: var(--font-mono);
    font-size: 0.68rem;
    line-height: 1;
    transition: border-color 180ms ease, color 180ms ease, background-color 180ms ease;
  }

  .trace-node-copy {
    display: grid;
    gap: 0.2rem;
    min-width: 0;
  }

  .trace-node strong {
    color: inherit;
    font-size: 0.82rem;
    font-weight: 600;
  }

  .trace-node small {
    color: var(--color-ink-3);
    font-size: 0.68rem;
    line-height: 1.35;
  }

  .trace-node.active {
    color: var(--color-ink);
  }

  .trace-node.active .trace-node-index {
    border-color: var(--trace-cyan);
    color: var(--trace-cyan);
  }

  .trace-node.current .trace-node-index {
    background: var(--trace-raised);
  }

  .trace-signal {
    position: absolute;
    z-index: 2;
    top: 0.58rem;
    left: 0;
    width: 0.55rem;
    height: 0.55rem;
    transform: translateX(-50%);
    animation: trace-journey 5.4s cubic-bezier(0.16, 1, 0.3, 1) infinite;
  }

  .trace-paused .trace-signal {
    animation-play-state: paused;
  }

  .trace-reduced .trace-signal {
    left: calc(100% - 1.1rem);
    animation: none;
  }

  .trace-proof {
    min-width: 0;
    margin-top: 1.5rem;
  }

  .trace-proof-head {
    align-items: flex-start;
    padding-bottom: 0.85rem;
    border-bottom: 1px solid var(--color-line);
  }

  .trace-proof-head h3 {
    max-width: 30ch;
    margin-top: 0.35rem;
    color: var(--trace-page);
    font-size: 0.9rem;
    font-weight: 600;
    line-height: 1.35;
  }

  .trace-proof .trace-label {
    color: var(--trace-page);
  }

  .trace-proof pre {
    max-height: 14rem;
    overflow: auto;
    margin: 0;
    padding-top: 0.95rem;
    color: var(--trace-page);
    font-family: var(--font-mono);
    font-size: 0.72rem;
    line-height: 1.6;
    white-space: pre-wrap;
  }

  .trace-caption {
    margin-top: 1rem;
    color: var(--color-ink-3);
    font-size: 0.72rem;
    line-height: 1.45;
  }

  @keyframes trace-journey {
    0% {
      left: 0;
    }
    30% {
      left: 33.333%;
    }
    63% {
      left: 66.666%;
    }
    100% {
      left: calc(100% - 1.1rem);
    }
  }

  @media (min-width: 640px) {
    .forge-map {
      grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr);
      gap: 1.25rem;
      align-items: start;
    }

    .trace-input {
      min-height: 100%;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .trace-signal {
      left: calc(100% - 1.1rem);
      animation: none;
    }

    .trace-node-index,
    .commit-list .signal-dot {
      transition: none;
    }
  }
</style>
