<script lang="ts">
  import { afterNavigate, goto } from "$app/navigation";
  import { page } from "$app/stores";
  import { onMount } from "svelte";
  import { getToken } from "$lib/api";
  import "../app.css";

  let { children } = $props();
  let token = $state(getToken());
  let theme = $state<"light" | "dark">(
    typeof document !== "undefined" &&
      document.documentElement.dataset.theme === "dark"
      ? "dark"
      : "light",
  );
  const isActive = (path: string) =>
    $page.url.pathname === path || $page.url.pathname.startsWith(`${path}/`);

  onMount(() => {
    theme =
      document.documentElement.dataset.theme === "dark" ? "dark" : "light";
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const followSystem = () => {
      let choice: string | null = null;
      try {
        choice = localStorage.getItem("annalist-theme");
      } catch {
        /* Storage may be unavailable. */
      }
      if (choice !== "light" && choice !== "dark") {
        theme = media.matches ? "dark" : "light";
        document.documentElement.dataset.theme = theme;
      }
    };
    media.addEventListener("change", followSystem);
    return () => media.removeEventListener("change", followSystem);
  });

  function chooseTheme(value: "light" | "dark") {
    theme = value;
    document.documentElement.dataset.theme = value;
    // Keep the browser chrome in step with the app surface.
    const meta = document.querySelector<HTMLMetaElement>(
      'meta[name="theme-color"]',
    );
    if (meta) meta.content = value === "dark" ? "#211d19" : "#f5eee5";
    try {
      localStorage.setItem("annalist-theme", value);
    } catch {
      /* Keep the current choice for this page. */
    }
  }

  afterNavigate(() => {
    token = getToken();
    if (!token && $page.url.pathname !== "/" && $page.url.pathname !== "/setup")
      goto("/setup");
  });
</script>

<div class="app-shell">
  <a class="skip-link" href="#main-content">Skip to main content</a>
  <header class="topbar">
    <a
      href={token ? "/repos" : "/setup"}
      class="brand"
      aria-label="Annalist home"
    >
      <img
        class="brand-logo--light"
        src="/annalist-logo-light.svg"
        alt=""
        width="984"
        height="256"
      />
      <img
        class="brand-logo--dark"
        src="/annalist-logo-dark.svg"
        alt=""
        width="984"
        height="256"
      />
    </a>
    <nav class="topnav" aria-label="Primary">
      {#if token}
        <a href="/repos" aria-current={isActive("/repos") ? "page" : undefined}
          >Repositories</a
        >
        <a
          href="/settings"
          aria-current={isActive("/settings") ? "page" : undefined}>Settings</a
        >
      {/if}
      <a href="/setup" aria-current={isActive("/setup") ? "page" : undefined}
        >Setup</a
      >
    </nav>
    <button
      type="button"
      class="theme-switch"
      role="switch"
      aria-checked={theme === "dark"}
      aria-label="Dark theme"
      title={theme === "dark"
        ? "Switch to light theme"
        : "Switch to dark theme"}
      onclick={() => chooseTheme(theme === "dark" ? "light" : "dark")}
    >
      <!-- One flat icon per plan, chosen by the app root like the lockup:
           no hydration swap, and the icon is right before first paint. -->
      <svg
        class="theme-switch-icon theme-switch-icon--sun"
        viewBox="0 0 16 16"
        aria-hidden="true"><circle cx="8" cy="8" r="2.7" /><path
          d="M8 1.2v1.5M8 13.3v1.5M1.2 8h1.5M13.3 8h1.5M3.2 3.2l1.1 1.1M11.7 11.7l1.1 1.1M12.8 3.2l-1.1 1.1M4.3 11.7l-1.1 1.1"
        /></svg
      >
      <svg
        class="theme-switch-icon theme-switch-icon--moon"
        viewBox="0 0 16 16"
        aria-hidden="true"><path d="M8 2a4 4 0 0 0 6 6 6 6 0 1 1-6-6Z" /></svg
      >
    </button>
  </header>
  <main id="main-content" class="shell-main">{@render children()}</main>
</div>
