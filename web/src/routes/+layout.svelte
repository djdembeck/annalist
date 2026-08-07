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

<!--
  THESIS: Every release is a traceable workpiece moving from commit input to shaped note; this shell refuses the interchangeable admin dashboard.
  OWN-WORLD: Dark powder-coated panels, thermal-paper proof, copper action rails, cyan trace signals, and compact instrumentation labels.
  STORY: Operators see what is connected, what is active, which voice is applied, and what note will ship.
  FIRST VIEWPORT: A release trace occupies the primary field; the note proof and next action sit beside it on desktop and stack on mobile.
  FORM: Release Trace Wall, seed aa4fba49.
  FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, and DESIGN.md
-->
<div class="app-shell">
  <header class="operator-nav">
    <nav class="nav-inner" aria-label="Primary navigation">
      <a href="/" class="brand-lockup focus-ring" aria-label="Annalist home">
        <span class="brand-name">ANNALIST</span>
        <span class="brand-subtitle">RELEASE CONTROL</span>
      </a>

      {#if token}
        <div class="nav-actions">
          <span
            class="readiness {setupComplete ? 'status-ok' : 'status-warn'}"
            role="status"
            aria-label={setupComplete ? "Setup ready" : "Setup pending"}
          >
            <span class="signal-dot" aria-hidden="true"></span>
            {setupComplete ? "SETUP READY" : "SETUP PENDING"}
          </span>
          <div class="nav-links">
            <a
              href="/setup"
              aria-current={isActive('/setup') ? 'page' : undefined}
              class="nav-link focus-ring {isActive('/setup') ? 'active' : ''}"
            >Setup{#if setupComplete}<span class="sr-only"> (complete)</span>{/if}</a>
            <a
              href="/repos"
              aria-current={isActive('/repos') ? 'page' : undefined}
              class="nav-link focus-ring {isActive('/repos') ? 'active' : ''}"
            >Repos</a>
            <a
              href="/settings"
              aria-current={isActive('/settings') ? 'page' : undefined}
              class="nav-link focus-ring {isActive('/settings') ? 'active' : ''}"
            >Settings</a>
          </div>
        </div>
      {:else}
        <div class="nav-actions">
          <div class="nav-links">
            <a
              href="/setup"
              aria-current={isActive('/setup') ? 'page' : undefined}
              class="nav-link focus-ring {isActive('/setup') ? 'active' : ''}"
            >Setup</a>
          </div>
        </div>
      {/if}
    </nav>
  </header>

  <main
    class="shell-main {$page.url.pathname === '/' ? 'shell-main-public' : 'shell-main-console'}"
  >
    {@render children()}
  </main>
</div>
