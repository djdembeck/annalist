<script lang="ts">
  import type { Snippet } from "svelte";
  let {
    title,
    lede,
    compact = false,
    headingLevel = "h1",
    actions,
  }: {
    title: string;
    lede?: string;
    compact?: boolean;
    headingLevel?: "h1" | "h2";
    actions?: Snippet<[]>;
  } = $props();
</script>

<header class="section-head" class:section-head--compact={compact}>
  <div>
    {#if headingLevel === "h2"}<h2>{title}</h2>{:else}<h1>{title}</h1>{/if}
    {#if lede}<p class="section-head__lede">{lede}</p>{/if}
  </div>
  {#if actions}<div class="section-head__actions">{@render actions()}</div>{/if}
</header>
