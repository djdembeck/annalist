<script lang="ts">
  import type { GenerationRun } from "$lib/api";
  let { status }: { status?: GenerationRun["status"] } = $props();
  const labels = {
    running: "Running",
    published: "Published",
    generated: "Preview generated",
    cached: "Cached",
    preserved: "Preserved",
    failed: "Failed",
    interrupted: "Interrupted",
  };
</script>

<span class="status status-{status ?? 'empty'}">
  {#if status}
    <svg viewBox="0 0 16 16" aria-hidden="true">
      {#if status === "published"}<path d="m3 8 3 3 7-7" />
      {:else if status === "failed"}<path d="m8 2 6 11H2L8 2Zm0 4v3m0 2h.01" />
      {:else if status === "running"}<circle cx="8" cy="8" r="5" /><path
          d="M8 4v4l3 2"
        />
      {:else if status === "cached"}<rect
          x="5"
          y="2"
          width="8"
          height="10"
          rx="1"
        /><path d="M3 5H2v9h8v-1" />
      {:else if status === "preserved"}<path
          d="M8 1.5 13 4v4c0 3-5 6-5 6s-5-3-5-6V4l5-2.5Zm-2 6 1.5 1.5L10 6"
        />
      {:else if status === "interrupted"}<rect
          x="3"
          y="3"
          width="10"
          height="10"
          rx="1"
        /><path d="M6 5v6m4-6v6" />
      {:else}<path d="M3 2h7l3 3v9H3V2Zm7 0v4h3M5 9h6m-6 3h4" />{/if}
    </svg>
    {labels[status]}
  {:else}No runs yet{/if}
</span>
