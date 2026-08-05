<script lang="ts">
  import { onMount } from "svelte";

  const commits = [
    "feat(gateway): add circuit-breaker middleware",
    "fix(rate-limit): reset counter on config reload",
    "docs: describe webhook auth headers",
    "refactor(metrics): consolidate histogram labels",
  ];

  const output = `## What's new

- Added circuit-breaker middleware to the gateway so cascading failures trip fast instead of saturating downstream services.
- Rate-limit counters now reset when the configuration reloads, preventing stale quotas after a rules change.

## Fixes

- Corrected histogram label cardinality in the metrics collector.

## Notes

- Webhook authentication headers are now documented in the API reference. No breaking changes to existing integrations.`;

  let step = $state(0);

  onMount(() => {
    const interval = setInterval(() => {
      step = (step + 1) % (commits.length + 2);
    }, 1400);
    return () => clearInterval(interval);
  });
</script>

<div class="rounded border border-line bg-surface-1 p-4 sm:p-6">
  <div class="mb-4 flex items-center justify-between">
    <span class="font-display text-sm text-ember">Forge stage</span>
    <span class="rounded bg-control px-2 py-0.5 text-xs font-medium tabular-nums text-ink-2" aria-hidden="true">v1.4.0</span>
  </div>

  <div class="relative h-24 overflow-hidden rounded bg-page/60 sm:h-28">
    <div class="absolute inset-0 flex items-center justify-center px-4">
      <div class="w-full max-w-lg">
        <div class="mb-3 h-6 overflow-hidden text-center">
          {#if step < commits.length}
            <p class="animate-strike text-sm text-heat sm:text-base">{commits[step]}</p>
          {:else}
            <span class="inline-block h-1.5 w-1.5 rounded-full bg-ink-3"></span>
          {/if}
        </div>

        <div class="relative h-2 w-full overflow-hidden rounded-sm bg-surface-2">
          <div
            class="absolute inset-0 h-full w-full transition-opacity duration-300"
            class:opacity-60={step < commits.length}
            class:opacity-100={step >= commits.length}
          >
            <div
              class="h-full w-full animate-heat-pulse"
              style="background: linear-gradient(90deg, #ff3d00 0%, #ff7b00 35%, #ffb300 60%, #fff7e8 80%, #ffb300 90%, #ff3d00 100%);"
            ></div>
          </div>
        </div>

        <div class="mt-2 flex justify-between text-xs tabular-nums text-ink-3">
          <span>HEAT {step < commits.length ? "1250" : "1480"}</span>
          <span>BLOWS {Math.min(step, commits.length).toString().padStart(2, "0")}/{commits.length.toString().padStart(2, "0")}</span>
        </div>
      </div>
    </div>
  </div>

  <div class="mt-4">
    <div class="rounded border border-line bg-page/80 p-4">
      <div class="mb-2 flex items-center justify-between">
        <span class="font-display text-sm text-white">Synthetic output</span>
        <span class="text-xs tabular-nums text-ink-3">written by annalist</span>
      </div>
      <pre class="whitespace-pre-wrap font-mono text-xs leading-relaxed text-ink-2">{output}</pre>
    </div>
  </div>
</div>

<style>
  @keyframes strike {
    0% {
      opacity: 0;
      transform: translateY(-0.5rem);
    }
    20% {
      opacity: 1;
      transform: translateY(0);
    }
    80% {
      opacity: 1;
      transform: translateY(0);
    }
    100% {
      opacity: 0;
      transform: translateY(0.5rem);
    }
  }

  @keyframes heatPulse {
    0%,
    100% {
      filter: brightness(1);
    }
    50% {
      filter: brightness(1.4);
    }
  }

  .animate-strike {
    animation: strike 1.4s ease-in-out both;
  }

  .animate-heat-pulse {
    animation: heatPulse 1.4s ease-in-out infinite;
  }

  @media (prefers-reduced-motion: reduce) {
    .animate-strike,
    .animate-heat-pulse {
      animation: none;
    }
  }
</style>
