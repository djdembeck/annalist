<script lang="ts">
  import { goto } from "$app/navigation";
  import {
    getStatus,
    setToken,
    getToken,
    getAvailableRepos,
    addRepo,
    putSettings,
    type Status,
    type AvailableRepo,
  } from "$lib/api";
  import MarkdownPreview from "$lib/components/MarkdownPreview.svelte";

  type Source = "github" | "forgejo";

  const SOURCES: Source[] = ["github", "forgejo"];
  const PRESET_DESCRIPTIONS: Record<string, string> = {
    chronicler: "Narrative. Tells the story of what changed and why it matters.",
    engineer: "Concise. Terse bullet items that get straight to the point.",
    launch: "Upbeat. Celebratory and energized for a big reveal.",
  };

  const STEPS = [
    "Welcome & Token",
    "Connect platforms",
    "Add repositories",
    "Choose voice",
    "Done",
  ];

  // Shared sample commits used to render honest synthetic tone examples below.
  const SAMPLE_COMMITS = [
    "fix: correct pagination on release history",
    "feat: add webhook delivery health check",
    "perf: cache release note generation results",
    "chore: drop deprecated v1 endpoints",
  ];

  const SYNTHETIC_EXAMPLES: Record<string, string> = {
    chronicler:
      `This release hardens the forge. Webhook deliveries now surface a live health check, so you can see at a glance whether GitHub is reaching your instance. Note generation runs faster under load thanks to result caching, and the retired v1 endpoints step aside after a long farewell tour.

## Features
- Webhook deliveries now carry a live health check, so you can see at a glance whether GitHub is reaching your instance.

## Fixes
- Release history pagination is corrected — long lists finally page the way they should.

## Improvements
- Note generation results are cached, so releases build faster under load.
- The deprecated v1 endpoints step aside after a long farewell tour.`,
    engineer:
      `This release adds a webhook delivery health check, caches note generation output, fixes pagination on the release history, and removes the deprecated v1 endpoints.

## Features
- Add webhook delivery health check

## Fixes
- Fix pagination on release history

## Improvements
- Cache note generation results for faster rebuilds
- Remove deprecated v1 endpoints`,
    launch:
      `Time to shine — this update makes your forge faster and healthier than ever. Webhook health checks keep you in the loop, note generation gets a speed boost, and release history finally scrolls like butter.

## Features
- Webhook health checks keep you in the loop on every delivery.

## Fixes
- Release history scrolls like butter, even with a long history.

## Improvements
- Note generation gets a speed boost when the forge is under load.`,
  };

  // --- step 1: token ---
  let adminToken = $state<string>(getToken());
  let step = $state(0);
  let error = $state("");
  let status = $state<Status | null>(null);
  let busy = $state(false);

  // --- step 2: platforms ---
  let skipWarning = $state(false);

  // --- step 3: repositories ---
  let available = $state<AvailableRepo[]>([]);
  let activeSource = $state<Source>("github");
  let selected = $state<Record<string, boolean>>({});
  let loading = $state(false);
  let saving = $state(false);
  let reposAdded = $state(0);
  let reposVisible = $state(false);
  let search = $state("");
  let debouncedSearch = $state("");
  let showForks = $state(false);
  let showSharedNamespaces = $state(false);
  let sortBy = $state<"name" | "activity">("activity");

  // Debounce search input so filtering large repo lists does not run on every keystroke.
  $effect(() => {
    const q = search;
    const id = setTimeout(() => {
      debouncedSearch = q;
    }, 150);
    return () => clearTimeout(id);
  });

  let connectedCount = $derived((status?.github ? 1 : 0) + (status?.forgejo ? 1 : 0));
  let selectedCount = $derived(Object.values(selected).filter(Boolean).length);

  // Memoize the filtered/sorted list so it is computed once per dependency change, not per template read.
  let filteredRepos = $derived((() => {
    const query = debouncedSearch.trim().toLowerCase();
    const items = available.filter((r) => {
      if (r.platform !== activeSource) return false;
      if (r.ownNamespace === false && !showSharedNamespaces) return false;
      if (r.fork === true && !showForks) return false;
      if (query && !`${r.owner}/${r.repo}`.toLowerCase().includes(query)) return false;
      return true;
    });
    const sorted = [...items];
    if (sortBy === "activity") {
      return sorted.sort((a, b) => {
        const at = new Date(a.pushedAt ?? a.updatedAt ?? 0).getTime();
        const bt = new Date(b.pushedAt ?? b.updatedAt ?? 0).getTime();
        if (at !== bt) return bt - at;
        if (a.owner !== b.owner) return a.owner.localeCompare(b.owner);
        return a.repo.localeCompare(b.repo);
      });
    }
    return sorted.sort((a, b) => {
      const an = a.ownNamespace === false ? 1 : 0;
      const bn = b.ownNamespace === false ? 1 : 0;
      if (an !== bn) return an - bn;
      const af = a.fork === true ? 1 : 0;
      const bf = b.fork === true ? 1 : 0;
      if (af !== bf) return af - bf;
      if (a.owner !== b.owner) return a.owner.localeCompare(b.owner);
      return a.repo.localeCompare(b.repo);
    });
  })());

  // --- step 4: voice ---
  let toneOption = $state("inherit");
  let customTone = $state("");

  // --- interaction helpers ---
  let showToken = $state(false);
  let showFilters = $state(false);

  function repoKey(r: AvailableRepo): string {
    return `${r.platform}/${r.owner}/${r.repo}`;
  }

  // Allow clicking back to any step already reached.
  function goTo(n: number): void {
    if (n < step) {
      step = n;
    }
  }

  async function submitToken(): Promise<void> {
    error = "";
    busy = true;
    setToken(adminToken);
    try {
      status = await getStatus();
      step = 1;
      skipWarning = false;
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        error = "That token was not accepted. Check your ADMIN_TOKEN and try again.";
      } else {
        error = e instanceof Error ? e.message : "Failed to connect to Annalist";
      }
      status = null;
    } finally {
      busy = false;
    }
  }

  function continueFromPlatforms(): void {
    skipWarning = false;
    step = 2;
  }

  function skipPlatforms(): void {
    skipWarning = true;
  }

  function advanceToRepos(): void {
    if (connectedCount > 0) {
      continueFromPlatforms();
    } else {
      skipPlatforms();
    }
  }

  async function loadRepos(): Promise<void> {
    if (loading) return;
    loading = true;
    error = "";
    try {
      available = await getAvailableRepos();
      const enabled = SOURCES.filter((s) => status?.[s]);
      if (enabled.length && !status?.[activeSource]) {
        activeSource = enabled[0];
      }
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error = e instanceof Error ? e.message : "Failed to load available repos";
    } finally {
      loading = false;
    }
  }

  function enterRepos(): void {
    reposVisible = true;
    loadRepos();
  }

  async function addSelected(): Promise<void> {
    if (selectedCount === 0) return;
    saving = true;
    error = "";
    const targets = available.filter((r) => selected[repoKey(r)]);
    try {
      for (const r of targets) {
        await addRepo(r);
      }
      reposAdded = targets.length;
      step = 3;
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error = e instanceof Error ? e.message : "Failed to add repositories";
    } finally {
      // `saving` is shared with the voice step's Finish button; it must not
      // stay true after the repo add succeeds or that button lands disabled
      // showing "Saving…" (the reported stuck state).
      saving = false;
    }
  }

  async function finishSetup(): Promise<void> {
    saving = true;
    error = "";
    let tone: string | null;
    if (toneOption === "inherit") {
      tone = null;
    } else if (toneOption === "custom") {
      tone = customTone.trim() ? customTone.trim() : null;
    } else {
      tone = toneOption;
    }
    try {
      await putSettings({ tone });
      localStorage.setItem("annalist.setup-complete", "1");
      step = 4;
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error = e instanceof Error ? e.message : "Failed to save default tone";
    } finally {
      // Reset even on success so stepping back to the voice step (step 3)
      // doesn't leave the Finish button stuck at "Saving…".
      saving = false;
    }
  }

  function toneLabel(): string {
    if (toneOption === "inherit") return "Inherit (server default)";
    if (toneOption === "custom") return customTone.trim() || "Custom";
    return toneOption;
  }

  // Honest preview of the selected voice, built from known sample commits.
  function previewNote(): string {
    return SYNTHETIC_EXAMPLES[toneOption] ?? SYNTHETIC_EXAMPLES.chronicler;
  }
</script>

<svelte:head>
  <title>Setup · Annalist</title>
</svelte:head>

<div class="mx-auto max-w-6xl px-4 py-6 sm:px-6 sm:py-10">
  <header class="section-head mb-6 flex-col items-start gap-4 sm:flex-row sm:items-end sm:justify-between">
    <div>
      <p class="trace-label mb-2">FIRST-RUN ACTIVATION</p>
      <h1 class="font-display text-3xl text-ink sm:text-4xl">Make the next release legible.</h1>
      <p class="mt-2 max-w-2xl text-base text-ink-2">
        Connect your forge, choose the repositories Annalist can read, and shape the voice
        that will ship with every release note.
      </p>
    </div>
    <span class="chip shrink-0">
      <span class="signal-dot" aria-hidden="true"></span>
      Station {step + 1} of {STEPS.length}
    </span>
  </header>

  <div class="console-grid setup-layout items-start gap-6 lg:grid-cols-[minmax(12rem,16rem)_minmax(0,1fr)]">
    <aside class="panel setup-track lg:sticky lg:top-24" aria-label="Setup progress">
      <div class="mb-4 flex items-center justify-between gap-3">
        <p class="trace-label">RELEASE TRACE</p>
        <span class="text-xs text-ink-3">{step + 1}/{STEPS.length}</span>
      </div>
      <nav aria-label="Setup progress">
        <ol class="space-y-1">
          {#each STEPS as label, i}
            <li>
              {#if i < step}
                <button
                  onclick={() => goTo(i)}
                  class="setup-node group w-full text-left"
                  aria-label={`Return to completed step ${i + 1}: ${label}`}
                >
                  <span class="signal-dot bg-ok text-ok" aria-hidden="true">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
                      <path d="M5 13l4 4L19 7" stroke-linecap="round" stroke-linejoin="round"></path>
                    </svg>
                  </span>
                  <span class="min-w-0 flex-1 truncate text-sm text-ink-2 group-hover:text-ink">{label}</span>
                  <span class="status status-ok">Done</span>
                </button>
              {:else if i === step}
                <span class="setup-node setup-node-active w-full" aria-current="step">
                  <span class="signal-dot bg-control text-heat" aria-hidden="true"></span>
                  <span class="min-w-0 flex-1 truncate text-sm font-semibold text-ink">{label}</span>
                  <span class="status status-active">Active</span>
                </span>
              {:else}
                <span class="setup-node w-full opacity-60">
                  <span class="signal-dot" aria-hidden="true"></span>
                  <span class="min-w-0 flex-1 truncate text-sm text-ink-3">{label}</span>
                  <span class="status">Queued</span>
                </span>
              {/if}
            </li>
          {/each}
        </ol>
      </nav>
      <div class="mt-5 border-t border-line pt-4">
        <p class="text-xs leading-5 text-ink-3">
          Each station leaves evidence on the trace. Completed stations stay available above.
        </p>
      </div>
    </aside>

    <main class="panel panel-soft min-w-0" aria-labelledby="station-heading">
      <div class="mb-6 flex flex-wrap items-center justify-between gap-3 border-b border-line pb-4">
        <div>
          <p class="trace-label">STATION {String(step + 1).padStart(2, "0")}</p>
          <h2 id="station-heading" class="mt-1 text-xl font-semibold text-ink">{STEPS[step]}</h2>
        </div>
        {#if busy || loading || saving}
          <span class="status status-active" aria-live="polite">
            <span class="signal-dot" aria-hidden="true"></span>
            {busy ? "Checking token" : loading ? "Reading repositories" : "Writing settings"}
          </span>
        {:else}
          <span class="status status-ready">Ready</span>
        {/if}
      </div>

      <div aria-live="polite" class="sr-only">
        Step {step + 1} of {STEPS.length}: {STEPS[step]}.
      </div>

      <div role="alert" aria-live="assertive">
        {#if error}
          <p class="mb-6 border border-alert/40 bg-alert/10 p-3 text-sm text-alert">
            {error}
          </p>
        {/if}
      </div>

      <!-- Step 1 — Welcome & Token -->
      {#if step === 0}
        <section class="space-y-6" aria-labelledby="welcome-heading">
          <div class="section-head block">
            <p class="trace-label mb-2">COMMIT INPUT</p>
            <h3 id="welcome-heading" class="text-2xl font-semibold text-ink">Connect the control plane.</h3>
            <p class="mt-2 max-w-2xl text-base text-ink-2">
              Annalist is self-hosted. The admin token you provide protects every admin
              action on this instance, so keep it private.
            </p>
          </div>
          <form
            class="flex flex-col gap-5"
            aria-busy={busy}
            onsubmit={(e) => {
              e.preventDefault();
              submitToken();
            }}
          >
            <div class="flex flex-col gap-2">
              <div class="flex items-center justify-between gap-3">
                <label for="admin-token" class="trace-label">ADMIN_TOKEN</label>
                <button
                  type="button"
                  onclick={() => (showToken = !showToken)}
                  class="btn btn-ghost px-2 py-1 text-xs"
                >
                  {showToken ? "Hide" : "Show"}
                </button>
              </div>
              <input
                id="admin-token"
                type={showToken ? "text" : "password"}
                bind:value={adminToken}
                placeholder="Paste your admin token"
                autocomplete="off"
                class="field"
              />
              <span class="text-xs leading-5 text-ink-3">
                This is the <span class="font-mono text-ink-2">ADMIN_TOKEN</span> you set in the
                server's <span class="font-mono text-ink-2">config.yaml</span> or environment. It is
                kept in your browser only.
              </span>
            </div>
            <div class="flex flex-col gap-3 border-t border-line pt-5 sm:flex-row sm:items-center">
              <button
                type="submit"
                disabled={!adminToken.trim() || busy}
                aria-busy={busy}
                class="btn btn-primary w-full sm:w-auto"
              >
                {#if busy}
                  <svg class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                  </svg>
                {/if}
                {busy ? "Connecting…" : "Continue"}
              </button>
              {#if busy}
                <span class="text-sm text-ink-2" aria-live="polite">Checking the token against Annalist…</span>
              {/if}
            </div>
          </form>
        </section>
      {/if}

      <!-- Step 2 — Connect platforms -->
      {#if step === 1}
        <section class="space-y-6" aria-labelledby="platform-heading">
          <div class="section-head block">
            <p class="trace-label mb-2">SIGNAL SOURCES</p>
            <h3 id="platform-heading" class="text-2xl font-semibold text-ink">Connect the platforms that emit releases.</h3>
            <p class="mt-2 max-w-2xl text-base text-ink-2">
              Annalist only reads release events and commit history. It cannot merge PRs,
              push code, or access source outside the repositories you add.
            </p>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            {#each SOURCES as source}
              {@const configured = status?.[source] ?? false}
              <div class="panel-soft flex min-w-0 flex-col gap-3 p-4">
                <div class="flex items-center gap-3">
                  <span class="signal-dot" class:bg-ok={configured} class:text-ok={configured} aria-hidden="true">
                    {#if configured}
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true">
                        <path d="M5 13l4 4L19 7" stroke-linecap="round" stroke-linejoin="round"></path>
                      </svg>
                    {/if}
                  </span>
                  <div class="min-w-0">
                    <p class="font-medium capitalize text-ink">{source}</p>
                    <p class="text-sm text-ink-2">{configured ? "Configured" : "Not configured"}</p>
                  </div>
                  <span class:status-ok={configured} class:status-muted={!configured} class="status ml-auto">
                    {configured ? "Connected" : "Offline"}
                  </span>
                </div>
                {#if !configured}
                  <a
                    href="https://github.com/djdembeck/annalist#platform-setup"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="btn btn-ghost w-fit px-0 text-sm"
                  >
                    How to configure {source}
                    <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
                      <path d="M7 17L17 7" stroke-linecap="round"></path>
                      <path d="M9 7h8v8" stroke-linecap="round" stroke-linejoin="round"></path>
                    </svg>
                  </a>
                {/if}
              </div>
            {/each}
          </div>

          <div class="flex flex-col gap-3 border-t border-line pt-5">
            <button
              onclick={advanceToRepos}
              disabled={connectedCount === 0}
              class="btn btn-primary w-full sm:w-auto"
            >
              Continue to repositories
            </button>
            {#if skipWarning}
              <p class="status status-error max-w-xl" role="status">
                No platform is configured yet, so you won't be able to add repositories.
                You can still continue and configure things later.
              </p>
              <button onclick={continueFromPlatforms} class="btn btn-secondary w-full sm:w-auto">
                Continue anyway
              </button>
            {:else if connectedCount === 0}
              <p class="text-sm text-ink-3">
                Connect at least one platform to continue, or skip below.
              </p>
              <button onclick={skipPlatforms} class="btn btn-ghost w-full sm:w-auto">
                Skip for now
              </button>
            {/if}
          </div>
        </section>
      {/if}

      <!-- Step 3 — Add repositories -->
      {#if step === 2}
        <section class="space-y-6" aria-labelledby="repository-heading">
          <div class="section-head block">
            <p class="trace-label mb-2">REPOSITORY WORKPIECE</p>
            <h3 id="repository-heading" class="text-2xl font-semibold text-ink">Choose what Annalist can read.</h3>
            <p class="mt-2 max-w-2xl text-base text-ink-2">
              Add repositories to give release webhooks and commit history a clear path
              into the note pipeline.
            </p>
          </div>

          {#if connectedCount === 0}
            <div class="panel-soft space-y-4 p-5">
              <p class="text-base text-ink-2">
                No platforms are configured, so there are no repositories to add yet.
              </p>
              <button onclick={() => (step = 1)} class="btn btn-secondary w-full sm:w-auto">
                Go back and connect a platform
              </button>
            </div>
          {:else if !reposVisible}
            <div class="panel-soft flex flex-col gap-4 p-5">
              <p class="text-base text-ink-2">
                Load the repositories Annalist can see from your connected platforms.
              </p>
              <div class="flex flex-col gap-3 sm:flex-row">
                <button onclick={enterRepos} class="btn btn-primary w-full sm:w-auto">
                  Load repositories
                </button>
                <button onclick={() => (step = 3)} class="btn btn-ghost w-full sm:w-auto">
                  Skip for now
                </button>
              </div>
            </div>
          {:else}
            <div class="flex flex-col gap-3">
              <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
                <label for="repository-search" class="sr-only">Search repositories</label>
                <input
                  id="repository-search"
                  type="search"
                  placeholder="Search repositories…"
                  bind:value={search}
                  class="field sm:flex-1"
                />
                <button
                  type="button"
                  onclick={() => (showFilters = !showFilters)}
                  aria-expanded={showFilters}
                  class="btn btn-secondary w-full sm:w-auto"
                >
                  {showFilters ? "Hide filters" : "Show filters"}
                </button>
              </div>
              {#if showFilters}
                <div class="panel-soft flex flex-col gap-3 p-4 sm:flex-row sm:flex-wrap sm:items-center">
                  <label class="flex items-center gap-2 text-sm text-ink-2">
                    Sort
                    <select bind:value={sortBy} class="field w-auto min-w-40">
                      <option value="name">Name</option>
                      <option value="activity">Recent activity</option>
                    </select>
                  </label>
                  <label class="flex w-fit cursor-pointer items-center gap-2 text-sm text-ink-2">
                    <input type="checkbox" bind:checked={showForks} class="h-4 w-4 accent-mark" />
                    Show forks
                  </label>
                  <label class="flex w-fit cursor-pointer items-center gap-2 text-sm text-ink-2">
                    <input type="checkbox" bind:checked={showSharedNamespaces} class="h-4 w-4 accent-mark" />
                    Show organization &amp; shared namespaces
                  </label>
                </div>
              {/if}
            </div>

            <div class="flex flex-wrap gap-2" aria-label="Repository platforms">
              {#each SOURCES as source}
                {@const isEnabled = status?.[source] ?? false}
                <button
                  onclick={() => (activeSource = source)}
                  disabled={!isEnabled}
                  aria-pressed={activeSource === source}
                  class="btn btn-secondary capitalize"
                  class:btn-primary={activeSource === source}
                  class:opacity-50={!isEnabled}
                >
                  {source}
                </button>
              {/each}
            </div>

            {#if loading}
              <div class="panel-soft max-h-80 overflow-auto p-2" aria-busy={loading} aria-label="Loading repositories">
                {#each [1, 2, 3, 4, 5] as _}
                  <div class="flex items-center gap-3 border-b border-line px-3 py-3 last:border-b-0">
                    <div class="h-4 w-4 animate-pulse bg-surface-2"></div>
                    <div class="h-4 flex-1 animate-pulse bg-surface-2"></div>
                  </div>
                {/each}
              </div>
            {:else}
              {@const items = filteredRepos}
              {#if items.length === 0}
                <div class="panel-soft space-y-2 p-5">
                  <p class="text-base text-ink-2">
                    No available {activeSource} repositories found.
                  </p>
                  <p class="text-sm text-ink-3">
                    Try clearing the search, enabling organization namespaces or forks, or
                    checking your platform token's repository access.
                  </p>
                  {#if search.trim()}
                    <button onclick={() => (search = "")} class="btn btn-ghost px-0 text-sm">
                      Clear search
                    </button>
                  {/if}
                </div>
              {:else}
                <div class="flex items-center justify-between gap-3">
                  <span class="trace-label">{items.length} AVAILABLE</span>
                  <button
                    onclick={() => {
                      const allSelected = items.every((r) => selected[repoKey(r)]);
                      const next = { ...selected };
                      for (const r of items) {
                        next[repoKey(r)] = !allSelected;
                      }
                      selected = next;
                    }}
                    class="btn btn-ghost px-0 text-sm"
                  >
                    {items.every((r) => selected[repoKey(r)]) ? "Deselect all" : "Select all"}
                  </button>
                </div>
                <ul class="panel-soft max-h-80 overflow-auto p-1" aria-label="Available repositories">
                  {#each items as r (repoKey(r))}
                    <li class="flex min-h-11 items-center gap-3 border-b border-line px-3 py-3 last:border-b-0 transition-colors hover:bg-row-hover [content-visibility:auto] [contain-intrinsic-size:auto_3rem]">
                      <input
                        type="checkbox"
                        id={repoKey(r)}
                        checked={selected[repoKey(r)] ?? false}
                        onchange={(e) => (selected[repoKey(r)] = e.currentTarget.checked)}
                        class="h-5 w-5 accent-mark sm:h-4 sm:w-4"
                      />
                      <label for={repoKey(r)} class="flex-1 cursor-pointer text-base text-ink sm:text-sm">
                        {r.owner}/{r.repo}
                      </label>
                    </li>
                  {/each}
                </ul>
                <button
                  onclick={addSelected}
                  disabled={selectedCount === 0 || saving}
                  aria-busy={saving}
                  class="btn btn-primary w-full sm:w-auto"
                >
                  {saving ? "Adding…" : `Add ${selectedCount} selected`}
                </button>
              {/if}
            {/if}
          {/if}

          <aside class="note-paper text-sm text-ink-2">
            <p>
              When you add a repository, Annalist can read its release webhooks and commit
              history to write release notes. It writes only to the release body.
            </p>
          </aside>
        </section>
      {/if}

      <!-- Step 4 — Choose voice -->
      {#if step === 3}
        <section class="space-y-6" aria-labelledby="voice-heading">
          <div class="section-head block">
            <p class="trace-label mb-2">NOTE SHAPING</p>
            <h3 id="voice-heading" class="text-2xl font-semibold text-ink">Choose the voice on the note.</h3>
            <p class="mt-2 max-w-2xl text-base text-ink-2">
              This becomes the default tone for every release note. You can override it
              per repository later.
            </p>
          </div>

          <label class="flex flex-col gap-2">
            <span class="trace-label">TONE PRESET</span>
            <select bind:value={toneOption} class="field">
              <option value="inherit">Inherit (server default)</option>
              <option value="chronicler">Chronicler — tells the story of what changed</option>
              <option value="engineer">Engineer — terse, right to the point</option>
              <option value="launch">Launch — upbeat and celebratory</option>
              <option value="custom">Custom…</option>
            </select>
          </label>

          {#if toneOption === "inherit"}
            <div class="panel-soft p-4 text-sm text-ink-2">
              Uses the tone configured on the server (config.yaml or environment). You can
              override the tone per repository later.
            </div>
          {:else if toneOption === "custom"}
            <label class="flex flex-col gap-2">
              <span class="trace-label">CUSTOM TONE</span>
              <input bind:value={customTone} placeholder="Freeform persona" class="field" />
              {#if !customTone.trim()}
                <span class="status status-error" role="status">
                  Enter a custom tone before finishing.
                </span>
              {/if}
            </label>
          {:else if PRESET_DESCRIPTIONS[toneOption]}
            <div class="note-paper space-y-3">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <h4 class="font-semibold capitalize text-ink">{toneOption}</h4>
                <span class="chip">Synthetic example</span>
              </div>
              <p class="text-sm text-ink-2">{PRESET_DESCRIPTIONS[toneOption]}</p>
              <MarkdownPreview source={SYNTHETIC_EXAMPLES[toneOption]} />
              <p class="text-xs leading-5 text-ink-3">
                Sample output for these commits:
                <span class="font-mono text-ink-2">{SAMPLE_COMMITS.join(", ")}</span>
              </p>
            </div>
          {/if}

          <div class="flex flex-col gap-3 border-t border-line pt-5 sm:flex-row sm:items-center">
            <button
              onclick={finishSetup}
              disabled={saving || (toneOption === "custom" && !customTone.trim())}
              aria-busy={saving}
              class="btn btn-primary w-full sm:w-auto"
            >
              {saving ? "Saving…" : "Finish setup"}
            </button>
            {#if saving}
              <span class="text-sm text-ink-2" aria-live="polite">Writing your default tone…</span>
            {/if}
          </div>
        </section>
      {/if}

      <!-- Step 5 — Done -->
      {#if step === 4}
        <section class="space-y-6" aria-labelledby="done-heading">
          <div class="section-head block">
            <p class="trace-label mb-2">RELEASE READY</p>
            <h3 id="done-heading" class="text-2xl font-semibold text-ink">Your forge is ready.</h3>
            <p class="mt-2 max-w-2xl text-base text-ink-2">
              The next release you publish on an added repository gets its notes written
              automatically.
            </p>
          </div>

          <dl class="console-grid gap-3 text-sm sm:grid-cols-2">
            <div class="panel-soft flex items-center justify-between gap-3 p-4">
              <dt class="text-ink-2">Token</dt>
              <dd class="status status-ok">{adminToken.trim() ? "Set" : "Not set"}</dd>
            </div>
            <div class="panel-soft flex items-center justify-between gap-3 p-4">
              <dt class="text-ink-2">Platforms connected</dt>
              <dd class="text-ink">{connectedCount}</dd>
            </div>
            <div class="panel-soft flex items-center justify-between gap-3 p-4">
              <dt class="text-ink-2">Repositories added</dt>
              <dd class="text-ink">{reposAdded}</dd>
            </div>
            <div class="panel-soft flex items-center justify-between gap-3 p-4">
              <dt class="text-ink-2">Default tone</dt>
              <dd class="text-right capitalize text-ink">{toneLabel()}</dd>
            </div>
          </dl>

          <div class="space-y-3 border-t border-line pt-5">
            <h4 class="text-base font-semibold text-ink">What happens next</h4>
            <ol class="space-y-2 text-sm leading-6 text-ink-2">
              <li>1. Annalist listens for release events on the repositories you added.</li>
              <li>
                2. On a release, it clones the repo, summarizes the commits in the
                <span class="text-ink">{toneLabel()}</span> tone, and writes the note into
                the release body.
              </li>
              <li>
                3. You can regenerate notes, tweak the voice, or change per-repo settings from
                the Repos page.
              </li>
            </ol>
          </div>

          <div class="note-paper space-y-3">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="trace-label">NOTE PROOF</span>
              <span class="chip">Synthetic example</span>
            </div>
            <MarkdownPreview source={previewNote()} />
          </div>

          <div class="flex flex-col gap-3 border-t border-line pt-5 sm:flex-row sm:flex-wrap">
            <a href="/repos" class="btn btn-primary w-full sm:w-auto">Go to Repos</a>
            <a href="/settings" class="btn btn-secondary w-full sm:w-auto">Open Settings</a>
          </div>
        </section>
      {/if}
    </main>
  </div>
</div>
