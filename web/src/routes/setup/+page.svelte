<script lang="ts">
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
  import {
    filterAndSortRepos,
    getEnabledSources,
    isSourceEnabled,
    SOURCES,
    repoKey,
    resolveTone,
    batchAddRepos,
    retryFailedRepos,
    type Source,
    type RowStatus,
    type BatchResult,
  } from "$lib/repoUtils";
  import {
    handleAuthError,
    formatError,
  } from "$lib/composables/useAuthErrorHandler";

  const PRESET_DESCRIPTIONS: Record<string, string> = {
    chronicler:
      "The historian — frames each change as part of the project's story.",
    engineer:
      "The record-keeper — states each change and its behavior, fact-first.",
    launch:
      "The announcement — leads with what each change means for the user.",
  };

  const STEPS = [
    "Welcome & Token",
    "Connect platforms",
    "Add repositories",
    "Choose tone",
    "Done",
  ];

  // --- step 1: token ---
  let adminToken = $state<string>(getToken());
  let step = $state(0);
  let error = $state("");
  let status = $state<Status | null>(null);
  let busy = $state(false);

  // Keep the newly revealed step in view, especially after long mobile steps.
  $effect(() => {
    const currentStep = step;
    if (typeof window === "undefined") return;
    const frame = window.requestAnimationFrame(() => {
      if (currentStep !== step) return;
      window.scrollTo({
        top: 0,
        behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
          ? "auto"
          : "smooth",
      });
    });
    return () => window.cancelAnimationFrame(frame);
  });

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

  // Per-repository add results for batch reconciliation.
  let repoAddResults = $state<BatchResult>({});
  let addAttempted = $state(false);

  // Debounce search input so filtering large repo lists does not run on every keystroke.
  $effect(() => {
    const q = search;
    const id = setTimeout(() => {
      debouncedSearch = q;
    }, 150);
    return () => clearTimeout(id);
  });

  let configuredCount = $derived(
    (status?.github ? 1 : 0) + (status?.forgejo ? 1 : 0),
  );
  let selectedCount = $derived(
    Object.entries(selected).filter(
      ([key, checked]) => checked && repoAddResults[key]?.status !== "success",
    ).length,
  );

  // Memoize the filtered/sorted list so it is computed once per dependency change, not per template read.
  let filteredRepos = $derived(
    filterAndSortRepos(
      available,
      activeSource,
      debouncedSearch,
      showForks,
      showSharedNamespaces,
      sortBy,
    ),
  );

  // --- step 4: tone ---
  let toneOption = $state("inherit");
  let customTone = $state("");

  // --- interaction helpers ---
  let showToken = $state(false);
  let showFilters = $state(false);

  // Allow clicking back to any step already reached.
  function goTo(n: number): void {
    if (n < step) {
      step = n;
    }
  }

  function resetExpiredSession(): void {
    setToken("");
    adminToken = "";
    status = null;
    step = 0;
    skipWarning = false;
    error = "Your session expired. Enter the ADMIN_TOKEN again to continue.";
  }

  async function submitToken(): Promise<void> {
    error = "";
    busy = true;
    setToken(adminToken);
    try {
      status = await getStatus();
      skipWarning = false;
      step = 1;
    } catch (e) {
      if (handleAuthError(e, resetExpiredSession)) {
        busy = false;
        return;
      }
      error = formatError(e, "Failed to connect to Annalist");
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
    if (configuredCount > 0) {
      continueFromPlatforms();
    } else {
      skipPlatforms();
    }
  }

  // Persisted forward path to the tone step. Guarded: never advance while
  // an add is in flight, so a pending save cannot be skipped.
  function continueToTone(): void {
    if (saving) return;
    step = 3;
  }

  async function loadRepos(): Promise<void> {
    if (loading) return;
    loading = true;
    error = "";
    try {
      available = await getAvailableRepos();
      const enabled = getEnabledSources(status);
      if (enabled.length && !isSourceEnabled(status, activeSource)) {
        activeSource = enabled[0];
      }
    } catch (e) {
      if (handleAuthError(e, resetExpiredSession)) return;
      error = formatError(e, "Failed to load available repos");
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
    addAttempted = true;
    error = "";
    const targets = available.filter(
      (r) =>
        selected[repoKey(r)] &&
        repoAddResults[repoKey(r)]?.status !== "success",
    );

    // Seed every target as pending.
    const results: BatchResult = {};
    for (const r of targets) {
      results[repoKey(r)] = { status: "pending" };
    }
    repoAddResults = results;

    const successCount = await batchAddRepos(targets, repoAddResults, addRepo, () => {
      resetExpiredSession();
      saving = false;
      return true;
    });

    reposAdded += successCount;

    // Successful rows stay visible for reconciliation but leave the retry set.
    for (const r of targets) {
      const key = repoKey(r);
      if (repoAddResults[key]?.status === "success") selected[key] = false;
    }

    const failedKeys = Object.entries(repoAddResults)
      .filter(([k, v]) => v.status === "failure" && selected[k])
      .map(([k]) => k);

    if (failedKeys.length > 0) {
      error = "";
    } else if (successCount === targets.length) {
      step = 3;
    }

    saving = false;
  }

  // Retry only the repos that failed the last add attempt.
  async function retryFailedAdds(): Promise<void> {
    const failedKeys = Object.keys(repoAddResults).filter(
      (key) => repoAddResults[key].status === "failure",
    );
    if (failedKeys.length === 0) return;

    saving = true;
    addAttempted = true;
    error = "";

    // Reset failed items back to pending.
    for (const key of failedKeys) {
      repoAddResults[key] = { status: "pending" };
    }

    const successCount = await retryFailedRepos(
      available,
      failedKeys,
      repoAddResults,
      addRepo,
      () => {
        resetExpiredSession();
        saving = false;
        return true;
      },
    );

    reposAdded += successCount;

    for (const key of failedKeys) {
      if (repoAddResults[key]?.status === "success") {
        const r = available.find((r) => repoKey(r) === key);
        if (r) selected[key] = false;
      }
    }

    const stillFailed = Object.entries(repoAddResults).filter(
      ([, v]) => v.status === "failure",
    );
    if (stillFailed.length === 0) {
      step = 3;
    }

    saving = false;
  }

  async function finishSetup(): Promise<void> {
    saving = true;
    error = "";
    const tone = resolveTone(toneOption, customTone);
    try {
      await putSettings({ tone });
      step = 4;
    } catch (e) {
      if (handleAuthError(e, resetExpiredSession)) return;
      error = formatError(e, "Failed to save default tone");
    } finally {
      // Reset even on success so stepping back to the tone step (step 3)
      // doesn't leave the Finish button stuck at "Saving…".
      saving = false;
    }
  }

  function toneLabel(): string {
    if (toneOption === "inherit") return "Inherit (server default)";
    if (toneOption === "custom") return customTone.trim() || "Custom";
    return toneOption;
  }
</script>

<svelte:head>
  <title>Setup · Annalist</title>
  <meta
    name="description"
    content="Connect platforms, choose repositories, and shape Annalist's release-note tone."
  />
</svelte:head>

<div class="workspace setup-content">
  <header class="setup-heading">
    <h1>Setup</h1>
    <p>
      Connect to this instance, configure platforms, add repositories, and
      choose a default writing tone.
    </p>
  </header>
  <nav aria-label="Setup progress">
    <ol class="setup-progress">
      {#each STEPS as label, i}<li>
          {#if i < step}<button
              onclick={() => goTo(i)}
              aria-label={`Return to completed step ${i + 1}: ${label}`}
              >{i + 1}. {label}</button
            >{:else}<span aria-current={i === step ? "step" : undefined}
              >{i + 1}. {label}</span
            >{/if}
        </li>{/each}
    </ol>
  </nav>
  <div>
    <section class="panel min-w-0" aria-labelledby="setup-step-heading">
      <div
        class="mb-6 flex flex-wrap items-center justify-between gap-3 border-b border-line pb-4"
      >
        <div>
          <h2
            id="setup-step-heading"
            aria-live="polite"
            class="mt-1 text-xl font-semibold text-ink"
          >
            {STEPS[step]}
          </h2>
        </div>
        {#if busy || loading || saving}
          <span class="status status-active" aria-live="polite">
            {busy
              ? "Checking token"
              : loading
                ? "Reading repositories"
                : "Writing settings"}
          </span>
        {/if}
      </div>

      <div aria-live="polite" class="sr-only">
        Step {step + 1} of {STEPS.length}: {STEPS[step]}.
      </div>

      {#if error}
        <div
          class="panel panel--error text-alert"
          role="alert"
          aria-live="assertive"
        >
          {error}
        </div>
      {/if}
      {#if step === 0}
        <section class="space-y-6" aria-labelledby="welcome-heading">
          <div class="section-head block">
            <h3 id="welcome-heading" class="text-2xl font-semibold text-ink">
              Connect to Annalist
            </h3>
            <p class="mt-2 max-w-2xl text-base text-ink-2">
              Annalist is self-hosted. The admin token you provide protects
              every admin action on this instance, so keep it private.
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
                <label for="admin-token" class="field-caption"
                  >ADMIN_TOKEN</label
                >
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
                aria-describedby="admin-token-hint"
                aria-invalid={Boolean(error)}
                class="field"
              />
              <span id="admin-token-hint" class="text-xs leading-5 text-ink-3">
                This is the <span class="font-mono text-ink-2">ADMIN_TOKEN</span
                >
                you set in the server's
                <span class="font-mono text-ink-2">config.yaml</span> or environment.
                It is kept in your browser only.
              </span>
            </div>
            <div
              class="flex flex-col gap-3 border-t border-line pt-5 sm:flex-row sm:items-center"
            >
              <button
                type="submit"
                disabled={!adminToken.trim() || busy}
                aria-busy={busy}
                class="btn btn-primary w-full sm:w-auto"
              >
                {#if busy}
                  <svg
                    class="h-4 w-4 animate-spin"
                    viewBox="0 0 24 24"
                    fill="none"
                    aria-hidden="true"
                  >
                    <circle
                      class="opacity-25"
                      cx="12"
                      cy="12"
                      r="10"
                      stroke="currentColor"
                      stroke-width="4"
                    ></circle>
                    <path
                      class="opacity-75"
                      fill="currentColor"
                      d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                    ></path>
                  </svg>
                {/if}
                {busy ? "Connecting…" : "Continue"}
              </button>
              {#if busy}
                <span class="text-sm text-ink-2" aria-live="polite"
                  >Checking the token against Annalist…</span
                >
              {/if}
            </div>
          </form>
        </section>
      {/if}
      {#if step === 1}
        <section class="space-y-6" aria-labelledby="platform-heading">
          <div class="section-head block">
            <h3 id="platform-heading" class="text-2xl font-semibold text-ink">
              Configure platforms
            </h3>
            <p class="mt-2 max-w-2xl text-base text-ink-2">
              Annalist only reads release events and commit history. It cannot
              merge PRs, push code, or access source outside the repositories
              you add.
            </p>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            {#each SOURCES as source}
              {@const configured = isSourceEnabled(status, source)}
              <div class="panel-soft flex min-w-0 flex-col gap-3 p-4">
                <div class="flex items-center gap-3">
                  <div class="min-w-0">
                    <p class="font-medium capitalize text-ink">{source}</p>
                  </div>
                  <span
                    class:status-ok={configured}
                    class:status-muted={!configured}
                    class="status ml-auto"
                  >
                    {configured ? "Configured" : "Not configured"}
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
                    <svg
                      class="h-3.5 w-3.5"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2"
                      aria-hidden="true"
                    >
                      <path d="M7 17L17 7" stroke-linecap="round"></path>
                      <path
                        d="M9 7h8v8"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                      ></path>
                    </svg>
                  </a>
                {/if}
              </div>
            {/each}
          </div>

          <div class="flex flex-col gap-3 border-t border-line pt-5">
            <button
              onclick={advanceToRepos}
              disabled={busy}
              class="btn btn-primary w-full sm:w-auto"
            >
              Continue to repositories
            </button>
            {#if skipWarning}
              <p class="status status-error max-w-xl" role="status">
                No platform is configured yet, so you won't be able to add
                repositories. You can still continue and configure things later.
              </p>
              <button
                onclick={continueFromPlatforms}
                class="btn btn-secondary w-full sm:w-auto"
              >
                Continue anyway
              </button>
            {:else if configuredCount === 0}
              <p class="text-sm text-ink-3">
                Connect at least one platform to continue, or skip below.
              </p>
              <button
                onclick={skipPlatforms}
                class="btn btn-ghost w-full sm:w-auto"
              >
                Skip for now
              </button>
            {/if}
          </div>
        </section>
      {/if}
      {#if step === 2}
        <section class="space-y-6" aria-labelledby="repository-heading">
          <div class="section-head block">
            <h3 id="repository-heading" class="text-2xl font-semibold text-ink">
              Add repositories
            </h3>
            <p class="mt-2 max-w-2xl text-base text-ink-2">
              Add repositories to give release webhooks and commit history a
              clear path into the note pipeline.
            </p>
          </div>

          {#if configuredCount === 0}
            <div class="panel-soft space-y-4 p-5">
              <p class="text-base text-ink">
                No platforms are configured, so there are no repositories to add
                yet.
              </p>
              <p class="text-sm text-ink-2">
                Automation — release note generation — stays inactive until a
                platform and repository are configured. You can still set a
                default tone now and configure sources later.
              </p>
              <div class="flex flex-col gap-3 sm:flex-row">
                <button
                  onclick={() => (step = 1)}
                  class="btn btn-secondary w-full sm:w-auto"
                >
                  Go back and connect a platform
                </button>
              </div>
            </div>
          {:else if !reposVisible}
            <div class="panel-soft flex flex-col gap-4 p-5">
              <p class="text-base text-ink-2">
                Load the repositories Annalist can see from your configured
                platforms.
              </p>
              <div class="flex flex-col gap-3 sm:flex-row">
                <button
                  onclick={enterRepos}
                  class="btn btn-primary w-full sm:w-auto"
                >
                  Load repositories
                </button>
              </div>
            </div>
          {:else}
            <div class="flex flex-col gap-3">
              <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
                <label for="repository-search" class="sr-only"
                  >Search repositories</label
                >
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
                <div
                  class="panel-soft flex flex-col gap-3 p-4 sm:flex-row sm:flex-wrap sm:items-center"
                >
                  <label class="flex items-center gap-2 text-sm text-ink-2">
                    Sort
                    <select bind:value={sortBy} class="field w-auto min-w-40">
                      <option value="name">Name</option>
                      <option value="activity">Recent activity</option>
                    </select>
                  </label>
                  <label
                    class="flex w-fit cursor-pointer items-center gap-2 text-sm text-ink-2"
                  >
                    <input
                      type="checkbox"
                      bind:checked={showForks}
                      class="h-4 w-4 accent-mark"
                    />
                    Show forks
                  </label>
                  <label
                    class="flex w-fit cursor-pointer items-center gap-2 text-sm text-ink-2"
                  >
                    <input
                      type="checkbox"
                      bind:checked={showSharedNamespaces}
                      class="h-4 w-4 accent-mark"
                    />
                    Show organization &amp; shared namespaces
                  </label>
                </div>
              {/if}
            </div>

            <div class="flex flex-wrap gap-2" aria-label="Repository platforms">
              {#each SOURCES as source}
                {@const isEnabled = isSourceEnabled(status, source)}
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
              <div
                class="panel-soft max-h-80 overflow-auto p-2"
                aria-busy={loading}
                aria-label="Loading repositories"
              >
                {#each [1, 2, 3, 4, 5] as _}
                  <div
                    class="flex items-center gap-3 border-b border-line px-3 py-3 last:border-b-0"
                  >
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
                    Try clearing the search, enabling organization namespaces or
                    forks, or checking your platform token's repository access.
                  </p>
                  {#if search.trim()}
                    <button
                      onclick={() => (search = "")}
                      class="btn btn-ghost px-0 text-sm"
                    >
                      Clear search
                    </button>
                  {/if}
                </div>
              {:else}
                <div class="flex items-center justify-between gap-3">
                  <span class="field-caption">{items.length} AVAILABLE</span>
                  <button
                    onclick={() => {
                      const allSelected = items.every(
                        (r) => selected[repoKey(r)],
                      );
                      const next = { ...selected };
                      for (const r of items) {
                        next[repoKey(r)] = !allSelected;
                      }
                      selected = next;
                    }}
                    class="btn btn-ghost px-0 text-sm"
                  >
                    {items.every((r) => selected[repoKey(r)])
                      ? "Deselect all"
                      : "Select all"}
                  </button>
                </div>
                <ul
                  class="panel-soft max-h-80 overflow-auto p-1"
                  aria-label="Available repositories"
                >
                  {#each items as r (repoKey(r))}
                    {@const key = repoKey(r)}
                    {@const result = repoAddResults[key]}
                    <li
                      class="flex min-h-11 items-center gap-3 border-b border-line px-3 py-3 last:border-b-0 transition-colors hover:bg-row-hover [content-visibility:auto] [contain-intrinsic-size:auto_3rem]"
                    >
                      <input
                        type="checkbox"
                        id={key}
                        checked={selected[key] ?? false}
                        onchange={(e) =>
                          (selected[key] = e.currentTarget.checked)}
                        class="h-5 w-4 accent-mark sm:h-4 sm:w-4"
                      />
                      <label
                        for={key}
                        class="flex-1 cursor-pointer text-base text-ink sm:text-sm"
                      >
                        {r.owner}/<wbr />{r.repo}
                      </label>
                      {#if result}
                        {#if result.status === "pending"}
                          <span
                            class="status status-active"
                            aria-label="Adding {r.owner}/{r.repo}">Adding…</span
                          >
                        {:else if result.status === "success"}
                          <span
                            class="status status-ok"
                            aria-label="{r.owner}/{r.repo} added">Added</span
                          >
                        {:else if result.status === "failure"}
                          <span
                            class="status status-error"
                            role="status"
                            aria-label="{r.owner}/{r.repo} failed: {result.error}"
                            >Failed</span
                          >
                        {/if}
                      {/if}
                    </li>
                  {/each}
                </ul>

                {#if addAttempted}
                  {@const succeeded = Object.values(repoAddResults).filter(
                    (v) => v.status === "success",
                  ).length}
                  {@const failed = Object.values(repoAddResults).filter(
                    (v) => v.status === "failure",
                  ).length}
                  <div
                    class="space-y-3 border-t border-line pt-4"
                    role="status"
                    aria-live="polite"
                  >
                    <div
                      class="flex flex-wrap items-center gap-3 text-sm text-ink-2"
                    >
                      {#if succeeded > 0}
                        <span class="font-medium text-ok"
                          >{succeeded} added successfully</span
                        >
                      {/if}
                      {#if failed > 0}
                        <span class="font-medium text-alert"
                          >{failed} failed</span
                        >
                      {/if}
                    </div>
                    {#if failed > 0}
                      <div class="flex flex-col gap-3 sm:flex-row">
                        <button
                          onclick={retryFailedAdds}
                          disabled={saving}
                          class="btn btn-secondary w-full sm:w-auto"
                        >
                          {saving ? "Retrying…" : `Retry ${failed} failed`}
                        </button>
                      </div>
                      {#if succeeded > 0}
                        <p class="status status-error text-xs" role="status">
                          Unresolved repositories will remain inactive until
                          they are successfully added.
                        </p>
                      {/if}
                    {/if}
                  </div>
                {/if}

              {/if}

              <div class="flex flex-col gap-3 border-t border-line pt-5 sm:flex-row">
                <button
                  onclick={addSelected}
                  disabled={selectedCount === 0 || saving}
                  aria-busy={saving}
                  class="btn btn-primary w-full sm:w-auto"
                >
                  {saving
                    ? "Adding…"
                    : `Add ${selectedCount} selected`}
                </button>
              </div>
            {/if}
          {/if}

          <div class="flex flex-col gap-3 sm:flex-row sm:justify-end">
            <button
              onclick={continueToTone}
              disabled={saving}
              class="btn btn-ghost w-full sm:w-auto"
            >
              Continue to tone
            </button>
          </div>

          <aside class="note-paper text-sm text-ink-2">
            <p>
              When you add a repository, Annalist can read its release webhooks
              and commit history to write release notes. It writes only to the
              release body.
            </p>
          </aside>
        </section>
      {/if}
      {#if step === 3}
        <section class="space-y-6" aria-labelledby="tone-heading">
          <div class="section-head block">
            <h3 id="tone-heading" class="text-2xl font-semibold text-ink">
              Choose a default tone.
            </h3>
            <p class="mt-2 max-w-2xl text-base text-ink-2">
              {#if configuredCount === 0}
                No platform is configured yet, so release notes won't generate
                automatically. Pick a default tone now — it takes effect as soon
                as you add repositories.
              {:else}
                This becomes the default tone for every release note. You can
                override it per repository later.
              {/if}
            </p>
          </div>

          <label class="flex flex-col gap-2">
            <span class="field-caption">TONE PRESET</span>
            <select bind:value={toneOption} class="field">
              <option value="inherit">Inherit (server default)</option>
              <option value="chronicler"
                >Chronicler — frames each change as part of the story</option
              >
              <option value="engineer"
                >Engineer — states each change and its behavior, fact-first</option
              >
              <option value="launch"
                >Launch — leads with what each change means for users</option
              >
              <option value="custom">Custom…</option>
            </select>
          </label>

          {#if toneOption === "inherit"}
            <div class="panel-soft p-4 text-sm text-ink-2">
              Uses the tone configured on the server (config.yaml or
              environment). You can override the tone per repository later.
            </div>
          {:else if toneOption === "custom"}
            <label class="flex flex-col gap-2">
              <span class="field-caption">CUSTOM TONE</span>
              <input
                bind:value={customTone}
                placeholder="Freeform persona"
                class="field"
              />
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
              </div>
              <p class="text-sm text-ink-2">
                {PRESET_DESCRIPTIONS[toneOption]}
              </p>
            </div>
          {/if}

          <div
            class="flex flex-col gap-3 border-t border-line pt-5 sm:flex-row sm:items-center"
          >
            <button
              onclick={finishSetup}
              disabled={saving ||
                (toneOption === "custom" && !customTone.trim())}
              aria-busy={saving}
              class="btn btn-primary w-full sm:w-auto"
            >
              {saving ? "Saving…" : "Finish setup"}
            </button>
            {#if saving}
              <span class="text-sm text-ink-2" aria-live="polite"
                >Writing your default tone…</span
              >
            {/if}
          </div>
        </section>
      {/if}
      {#if step === 4}
        <section class="space-y-6" aria-labelledby="done-heading">
          <div class="section-head block">
            <h3 id="done-heading" class="text-2xl font-semibold text-ink">
              {#if configuredCount === 0}
                Setup complete — no platform configured.
              {:else}
                Setup complete.
              {/if}
            </h3>
            <p class="mt-2 max-w-2xl text-base text-ink-2">
              {#if configuredCount === 0}
                Automation is inactive until you connect a platform and add
                repositories. When you do, the next release you publish gets its
                notes written automatically in the <span class="text-ink"
                  >{toneLabel()}</span
                > tone.
              {:else}
                The next release you publish on an added repository gets its
                notes written automatically.
              {/if}
            </p>
          </div>

          {#if configuredCount === 0}
            <div class="panel-soft space-y-3 p-4">
              <p class="text-sm text-ink-2">
                Connect a source to activate release note generation.
              </p>
              <div class="flex flex-col gap-2 sm:flex-row">
                <button
                  onclick={() => (step = 1)}
                  class="btn btn-secondary w-full sm:w-auto"
                >
                  Connect a platform
                </button>
                <button
                  onclick={() => (step = 2)}
                  class="btn btn-ghost w-full sm:w-auto"
                >
                  Add repositories
                </button>
              </div>
            </div>
          {/if}

          <dl class="console-grid gap-3 text-sm sm:grid-cols-2">
            <div class="panel-soft flex items-center justify-between gap-3 p-4">
              <dt class="text-ink-2">Token</dt>
              <dd class="status status-ok">
                {adminToken.trim() ? "Set" : "Not set"}
              </dd>
            </div>
            <div class="panel-soft flex items-center justify-between gap-3 p-4">
              <dt class="text-ink-2">Platforms configured</dt>
              <dd class="text-ink">{configuredCount}</dd>
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
              <li>
                1. Annalist listens for release events on the repositories you
                added.
              </li>
              <li>
                2. On a release, it clones the repo, summarizes the commits in
                the
                <span class="text-ink">{toneLabel()}</span> tone, and writes the note
                into the release body.
              </li>
              <li>
                3. You can regenerate notes, tweak the tone, or change per-repo
                settings from the Repositories page.
              </li>
            </ol>
          </div>

          <div
            class="flex flex-col gap-3 border-t border-line pt-5 sm:flex-row sm:flex-wrap"
          >
            <a href="/repos" class="btn btn-primary w-full sm:w-auto"
              >Open repositories</a
            >
            <a href="/settings" class="btn btn-secondary w-full sm:w-auto"
              >Open Settings</a
            >
          </div>
        </section>
      {/if}
    </section>
  </div>
</div>
