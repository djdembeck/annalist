<script lang="ts">
  import { marked } from "marked";
  import DOMPurify from "isomorphic-dompurify";

  let {
    source,
    class: className = "",
  }: {
    source: string;
    class?: string;
  } = $props();

  let mode = $state<"rendered" | "raw">("rendered");

  const renderedHtml = $derived(
    DOMPurify.sanitize(marked.parse(source) as string),
  );
</script>

<div class="rounded border border-line bg-surface-1 p-1 {className}">
  <div
    class="flex items-center justify-end gap-1 border-b border-line px-3 py-2"
    aria-label="Preview mode"
    role="group"
  >
    <span class="mr-2 text-xs text-ink-3">View</span>
    <button
      type="button"
      aria-pressed={mode === "rendered"}
      onclick={() => (mode = "rendered")}
      class="rounded px-2 py-[0.625rem] text-xs transition-colors min-h-[2.75rem] {mode === 'rendered'
        ? 'bg-surface-2 text-ink'
        : 'text-ink-3 hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring'}"
    >
      Rendered
    </button>
    <button
      type="button"
      aria-pressed={mode === "raw"}
      onclick={() => (mode = "raw")}
      class="rounded px-2 py-[0.625rem] text-xs transition-colors min-h-[2.75rem] {mode === 'raw'
        ? 'bg-surface-2 text-ink'
        : 'text-ink-3 hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring'}"
    >
      Raw
    </button>
  </div>

  {#if mode === "rendered"}
    <div
      class="max-h-96 overflow-auto p-4 prose-forge"
      aria-live="polite"
    >
      {@html renderedHtml}
    </div>
  {:else}
    <pre
      class="max-h-96 overflow-auto whitespace-pre-wrap rounded bg-surface-2 p-4 font-mono text-xs text-ink"
      aria-live="polite"
    >{source}</pre>
  {/if}
</div>

<style>
  .prose-forge :global(*) {
    color: var(--trace-ink);
  }
  .prose-forge :global(h2) {
    font-family: "Saira", ui-sans-serif, system-ui, sans-serif;
    font-weight: 600;
    font-size: 1rem;
    line-height: 1.25;
    margin-bottom: 0.75rem;
    padding-bottom: 0.375rem;
    border-bottom: 1px solid var(--trace-line);
  }
  .prose-forge :global(p) {
    font-family: "Saira", ui-sans-serif, system-ui, sans-serif;
    font-size: 0.875rem;
    line-height: 1.5;
    margin-bottom: 0.75rem;
    color: var(--trace-ink);
  }
  .prose-forge :global(ul) {
    list-style-type: disc;
    padding-left: 1.25rem;
    margin-bottom: 0.75rem;
  }
  .prose-forge :global(li) {
    font-family: "Saira", ui-sans-serif, system-ui, sans-serif;
    font-size: 0.875rem;
    line-height: 1.5;
    margin-bottom: 0.25rem;
    color: var(--trace-muted);
  }
  .prose-forge :global(li::marker) {
    color: var(--trace-copper);
  }
  .prose-forge :global(strong) {
    color: var(--trace-ink);
    font-weight: 600;
  }
  .prose-forge :global(code) {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono",
      "Courier New", monospace;
    font-size: 0.8125rem;
    background: var(--trace-raised);
    padding: 0.125rem 0.375rem;
    border-radius: 4px;
    color: var(--trace-ink);
  }
</style>
