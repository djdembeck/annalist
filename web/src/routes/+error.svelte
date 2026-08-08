<script lang="ts">
  import { page } from "$app/stores";

  let { error, status } = $props<{
    error: Error;
    status?: number;
  }>();
</script>

<svelte:head>
  <title>Error · Annalist</title>
</svelte:head>

<div class="mx-auto max-w-2xl px-4 py-16 sm:py-24">
  <section class="panel panel--error space-y-6" role="alert" aria-labelledby="error-heading">
    <div>
      <p class="trace-label mb-2">SIGNAL LOST</p>
      <h1 id="error-heading" class="font-display text-3xl text-ink sm:text-4xl">Trace interrupted.</h1>
      {#if status}
        <p class="mt-1"><span class="chip">Status {status}</span></p>
      {/if}
    </div>

    <div class="flex items-start gap-3">
      <span class="signal-dot signal-dot--error shrink-0" aria-hidden="true"></span>
      <p class="text-base text-ink">{error instanceof Error ? error.message : "The application returned an unexpected error."}</p>
    </div>

    <div class="flex flex-col gap-3 pt-4 sm:flex-row">
      <button onclick={() => ($page.form ? history.back() : location.reload())} class="btn btn-primary w-full sm:w-auto">
        Try again
      </button>
      <a href="/" class="btn btn-secondary w-full sm:w-auto">Return home</a>
      {#if !$page.url.pathname.startsWith("/setup")}
        <a href="/setup" class="btn btn-ghost w-full sm:w-auto">Open setup</a>
      {/if}
    </div>
  </section>
</div>
