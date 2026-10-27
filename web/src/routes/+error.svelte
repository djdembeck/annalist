<script lang="ts">
  import { page } from "$app/stores";
</script>

<svelte:head>
  <title>Error · Annalist</title>
</svelte:head>

<div class="mx-auto max-w-2xl px-4 py-16 sm:py-24">
  <section
    class="panel panel--error space-y-6"
    role="alert"
    aria-labelledby="error-heading"
  >
    <div>
      <h1 id="error-heading" class="font-display text-3xl text-ink sm:text-4xl">
        Page unavailable.
      </h1>
      <p class="mt-1"><span class="chip">Status {$page.status}</span></p>
    </div>

    {#if $page.status === 404}
      <p class="text-base text-ink">
        That address doesn’t match a page. Check the address or return to your
        repositories.
      </p>
    {:else}
      <p class="text-base text-ink">
        An unexpected error prevented this page from loading. Try again, or
        return to your repositories.
      </p>
    {/if}

    <div class="flex flex-col gap-3 pt-4 sm:flex-row">
      <button
        type="button"
        onclick={() => ($page.form ? history.back() : location.reload())}
        class="btn btn-primary w-full sm:w-auto"
      >
        Try again
      </button>
      <a href="/repos" class="btn btn-secondary w-full sm:w-auto"
        >View repositories</a
      >
      {#if !$page.url.pathname.startsWith("/setup")}
        <a href="/setup" class="btn btn-ghost w-full sm:w-auto">Open setup</a>
      {/if}
    </div>
  </section>
</div>
