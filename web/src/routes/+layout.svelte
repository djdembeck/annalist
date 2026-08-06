<script lang="ts">
  import { afterNavigate, goto } from "$app/navigation";
  import { page } from "$app/stores";
  import { getToken } from "$lib/api";
  import "../app.css";

  let { children } = $props();

  let token = $state<string>(getToken());

  // UI-only hint that onboarding finished; does not gate any route.
  let setupComplete = $state(
    typeof localStorage !== "undefined" && localStorage.getItem("annalist.setup-complete") === "1",
  );

  // Active nav link: exact route or a route nested beneath it.
  const isActive = (path: string) =>
    $page.url.pathname === path || $page.url.pathname.startsWith(`${path}/`);

  afterNavigate(() => {
    token = getToken();
    setupComplete =
      typeof localStorage !== "undefined" &&
      localStorage.getItem("annalist.setup-complete") === "1";
    // The landing page and setup page stay public; every other route needs a token.
    if (!token && $page.url.pathname !== "/" && $page.url.pathname !== "/setup") {
      goto("/setup");
    }
  });
</script>

<div class="min-h-dvh">
  <nav
    class="sticky top-0 z-40 flex items-center gap-4 sm:gap-6 border-b border-line bg-surface-1 px-4 sm:px-6 pt-[env(safe-area-inset-top)] pl-[env(safe-area-inset-left)] pr-[env(safe-area-inset-right)] py-3 sm:py-4"
  >
    <a href="/" class="focus-ring font-display text-lg tracking-tight text-white">ANNALIST</a>
    <div class="ml-auto flex items-center gap-2 sm:gap-6">
      {#if token}
        <a
          href="/setup"
          class="focus-ring nav-link p-1.5 text-sm {isActive('/setup') ? 'active text-heat' : 'text-ink-2 hover:text-ink'}"
        >Setup{#if setupComplete}<span
              class="ml-1 inline-block h-1.5 w-1.5 rounded-full bg-ok"
              aria-hidden="true"
            ></span
            ><span class="sr-only"> (complete)</span>{/if}</a>
        <a
          href="/repos"
          class="focus-ring nav-link p-1.5 text-sm {isActive('/repos') ? 'active text-heat' : 'text-ink-2 hover:text-ink'}"
        >Repos</a>
        <a
          href="/settings"
          class="focus-ring nav-link p-1.5 text-sm {isActive('/settings') ? 'active text-heat' : 'text-ink-2 hover:text-ink'}"
        >Settings</a>
      {:else}
        <a
          href="/setup"
          class="focus-ring nav-link p-1.5 text-sm {isActive('/setup') ? 'active text-heat' : 'text-ink-2 hover:text-ink'}"
        >Dashboard</a>
      {/if}
    </div>
  </nav>
  <main
    class={$page.url.pathname === "/"
      ? ""
      : "mx-auto w-full max-w-5xl px-4 py-4 sm:px-6 sm:py-6 lg:px-8 lg:py-8 pb-[max(2rem,env(safe-area-inset-bottom))]"}
  >
    {@render children()}
  </main>
</div>
