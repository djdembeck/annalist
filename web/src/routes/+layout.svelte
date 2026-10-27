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
      <svg viewBox="0 0 40 40" aria-hidden="true">
        <path
          d="M20 11c-4-4-10-5-15-3v23c5-2 11-1 15 3 4-4 10-5 15-3V8c-5-2-11-1-15 3Z"
          fill="none"
          stroke="currentColor"
          stroke-width="2.3"
          stroke-linejoin="round"
        />
        <path
          d="M20 11v23M10 14c2 0 4 .5 6 1.5M10 20c2 0 4 .5 6 1.5M10 26c2 0 4 .5 6 1.5"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
        />
        <path
          d="M25 14c1.5-.8 3.2-1.2 5-1.2v9l-2.5-1.5L25 23Z"
          fill="var(--rust)"
        />
      </svg>
      <span class="brand-name">Annalist</span>
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
    <div class="theme-switch" role="group" aria-label="Color theme">
      <button
        type="button"
        aria-pressed={theme === "light"}
        onclick={() => chooseTheme("light")}>Light</button
      >
      <button
        type="button"
        aria-pressed={theme === "dark"}
        onclick={() => chooseTheme("dark")}>Dark</button
      >
    </div>
  </header>
  <main id="main-content" class="shell-main">{@render children()}</main>
</div>
