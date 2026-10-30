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
