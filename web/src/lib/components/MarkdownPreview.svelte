<script lang="ts">
  import { marked } from "marked";
  import DOMPurify from "isomorphic-dompurify";
  let { source }: { source: string } = $props();
  let mode = $state<"rendered" | "raw">("rendered");
  const renderedHtml = $derived(
    DOMPurify.sanitize(marked.parse(source) as string),
  );
</script>

<div class="rounded-lg border border-line bg-surface-1">
  <div
    class="flex items-center justify-end gap-1 border-b border-line px-3 py-2"
    aria-label="Preview mode"
    role="group"
  >
    <span class="mr-2 text-sm text-ink-3">View</span>
    <button
      type="button"
      aria-pressed={mode === "rendered"}
      onclick={() => (mode = "rendered")}
      class="btn {mode === 'rendered' ? 'btn-secondary' : 'btn-ghost'}"
      >Rendered</button
    >
    <button
      type="button"
      aria-pressed={mode === "raw"}
      onclick={() => (mode = "raw")}
      class="btn {mode === 'raw' ? 'btn-secondary' : 'btn-ghost'}">Raw</button
    >
  </div>
  {#if mode === "rendered"}
    <div class="max-h-96 overflow-auto p-4 markdown-body" aria-live="polite">
      {@html renderedHtml}
    </div>
  {:else}
    <pre
      class="max-h-96 overflow-auto whitespace-pre-wrap p-4 text-sm text-ink"
      aria-live="polite">{source}</pre>
  {/if}
</div>
