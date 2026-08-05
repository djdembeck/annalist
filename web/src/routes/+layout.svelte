<script lang="ts">
  import { afterNavigate, goto } from "$app/navigation";
  import { page } from "$app/stores";
  import { getToken } from "$lib/api";
  import "../app.css";

  let { children } = $props();

  let token = $state<string>(getToken());

  // Active nav link: exact route or a route nested beneath it.
  const isActive = (path: string) =>
    $page.url.pathname === path || $page.url.pathname.startsWith(`${path}/`);

  afterNavigate(() => {
    token = getToken();
    // The landing page stays public; every other route needs a token.
    if (!token && $page.url.pathname !== "/") {
      goto("/setup");
    }
  });
</script>

<div class="min-h-dvh">
  <nav class="flex items-center gap-6 border-b border-line bg-surface-1 px-6 py-4">
    <a href="/" class="font-display text-lg tracking-tight text-white">ANNALIST</a>
    <div class="ml-auto flex items-center gap-6">
      {#if token}
        <a
          href="/setup"
          class="nav-link text-sm {isActive('/setup') ? 'active text-heat' : 'text-ink-2 hover:text-ink'}"
        >Setup</a>
        <a
          href="/repos"
          class="nav-link text-sm {isActive('/repos') ? 'active text-heat' : 'text-ink-2 hover:text-ink'}"
        >Repos</a>
        <a
          href="/settings"
          class="nav-link text-sm {isActive('/settings') ? 'active text-heat' : 'text-ink-2 hover:text-ink'}"
        >Settings</a>
      {:else}
        <a
          href="/setup"
          class="nav-link text-sm {isActive('/setup') ? 'active text-heat' : 'text-ink-2 hover:text-ink'}"
        >Dashboard</a>
      {/if}
    </div>
  </nav>
  <main class={$page.url.pathname === "/" ? "" : "mx-auto max-w-5xl p-8"}>
    {@render children()}
  </main>
</div>
