<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { getToken } from "$lib/api";
  onMount(() => {
    void goto(getToken() ? "/repos" : "/setup", { replaceState: true });
  });
</script>

<svelte:head><title>Annalist</title></svelte:head>
<p role="status" class="text-ink-2">Opening Annalist…</p>
