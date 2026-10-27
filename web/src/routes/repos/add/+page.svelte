<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import {
    filterAndSortRepos,
    getEnabledSources,
    isSourceEnabled,
    SOURCES,
    repoKey,
    batchAddRepos,
    retryFailedRepos,
    type Source,
    type RowStatus,
    type BatchResult,
  } from "$lib/repoUtils";
  import {
    getStatus,
    getAvailableRepos,
    addRepo,
    type AvailableRepo,
    type Status,
  } from "$lib/api";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import ErrorBanner from "$lib/components/ErrorBanner.svelte";
  import SectionHead from "$lib/components/SectionHead.svelte";
  import {
    handleAuthError,
    formatError,
  } from "$lib/composables/useAuthErrorHandler";

  let status = $state<Status | null>(null);
  let available = $state<AvailableRepo[]>([]);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state("");
  let activeSource = $state<Source>("github");
  // Null until a platform is chosen; defaults to the first configured
  // platform on load, consistent with discovery.
  let manualPlatform = $state<Source | null>(null);
  let manualOwner = $state<string>("");
  let manualRepo = $state<string>("");
  let search = $state("");
  let showForks = $state(false);
  let showSharedNamespaces = $state(false);
  let sortBy = $state<"name" | "activity">("activity");
  let selected = $state<Record<string, boolean>>({});

  // Per-row operation tracking for batch intake
  let rowResults = $state<BatchResult>({});

  async function load(): Promise<void> {
    try {
      status = await getStatus();
      const enabled = getEnabledSources(status);
      if (enabled.length && !isSourceEnabled(status, activeSource))
        activeSource = enabled[0];
      // Default the manual entry to the first configured platform, the same
      // way discovery does — but never overwrite a platform the operator
      // has already chosen, so an explicit selection survives later async
      // reloads.
      if (enabled.length && manualPlatform === null)
        manualPlatform = enabled[0];
      available = await getAvailableRepos();
      error = "";
    } catch (e) {
      if (handleAuthError(e)) return;
      error = formatError(e, "Failed to load available repos");
    } finally {
      loading = false;
    }
  }

  onMount(load);

  // Memoized filtered/sorted list — recomputed by Svelte when dependencies change.
  let filteredList = $derived(
    filterAndSortRepos(
      available,
      activeSource,
      search,
      showForks,
      showSharedNamespaces,
      sortBy,
    ),
  );

  function selectedCount(): number {
    return Object.values(selected).filter(Boolean).length;
  }

  async function addSelected(): Promise<void> {
    saving = true;
    error = "";
    const targets = available.filter((r) => selected[repoKey(r)]);
    const results: BatchResult = {};
    for (const r of targets) {
      results[repoKey(r)] = { status: "pending" };
    }
    rowResults = results;

    const successCount = await batchAddRepos(targets, rowResults, addRepo);
    const allSuccess = successCount === targets.length;

    if (allSuccess) {
      rowResults = {};
      selected = {};
      goto("/repos");
    } else {
      saving = false;
      for (const r of targets) {
        const key = repoKey(r);
        if (rowResults[key]?.status === "success") {
          selected[key] = false;
        }
      }
    }
  }

  // Retry only the failed rows
  async function retryFailed(): Promise<void> {
    saving = true;
    error = "";
    const failedKeys = Object.keys(rowResults).filter(
      (key) => rowResults[key].status === "failure",
    );
    if (failedKeys.length === 0) {
      saving = false;
      return;
    }

    // Seed failed keys as pending; keep successful rows from previous batch.
    const results: BatchResult = {};
    for (const key of failedKeys) {
      results[key] = { status: "pending" };
    }
    for (const [key, v] of Object.entries(rowResults)) {
      if (v.status === "success") results[key] = { status: "success" };
    }
    rowResults = results;

    const successCount = await retryFailedRepos(
      available,
      failedKeys,
      rowResults,
      addRepo,
    );
    const allSuccess = successCount === failedKeys.length;

    if (allSuccess) {
      rowResults = {};
      selected = {};
      goto("/repos");
    } else {
      saving = false;
      for (const key of failedKeys) {
        if (rowResults[key]?.status === "success") {
          selected[key] = false;
        }
      }
    }
  }

  async function addManual(): Promise<void> {
    const owner = manualOwner.trim();
    const repo = manualRepo.trim();
    if (!owner || !repo) {
      error = "Owner and repository name are required.";
      return;
    }
    if (!manualPlatform) {
      error = "Choose a platform before adding.";
      return;
    }
    saving = true;
    error = "";
    try {
      await addRepo({ platform: manualPlatform, owner, repo });
      manualOwner = "";
      manualRepo = "";
      goto("/repos");
    } catch (e) {
      saving = false;
      if (handleAuthError(e)) return;
      error = formatError(e, "Failed to add repository");
    }
  }

  // Keyboard handling for button-group source switching
  function handleSourceKeyDown(e: KeyboardEvent): void {
    const sources = getEnabledSources(status);
    if (sources.length <= 1) return;
    const currentIndex = sources.indexOf(activeSource);
    if (e.key === "ArrowRight" || e.key === "ArrowDown") {
      e.preventDefault();
      activeSource = sources[(currentIndex + 1) % sources.length];
    } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
      e.preventDefault();
      activeSource =
        sources[(currentIndex - 1 + sources.length) % sources.length];
    }
  }
</script>

<svelte:head>
  <title>Add repositories · Annalist</title>
  <meta
    name="description"
    content="Connect repositories to Annalist's release-note pipeline."
  />
</svelte:head>

<div class="workspace">
  <SectionHead
    title="Add repositories"
    lede="Choose repositories from a configured platform, or add one by name."
  >
    {#snippet actions()}
      <a href="/repos" class="btn btn-secondary">Back to repositories</a>
    {/snippet}
  </SectionHead>

  {#if loading}
    <section
      class="panel"
      aria-busy="true"
      aria-label="Loading available repositories"
    >
      <p class="field-caption">Reading enabled repositories</p>
      <div class="skeleton mt-4 h-11 w-full"></div>
      <div class="skeleton mt-3 h-16 w-full"></div>
      <div class="skeleton mt-3 h-16 w-full"></div>
    </section>
  {:else if error}
    <section
      class="panel panel--error"
      role="alert"
      aria-label="Repository discovery error"
    >
      <p class="field-caption">Intake failed</p>
      <p class="mt-2 text-sm text-alert">{error}</p>
      <div class="mt-4 flex flex-col gap-3 sm:flex-row">
        <button
          onclick={load}
          disabled={loading}
          class="btn btn-primary w-full sm:w-auto"
        >
          Retry
        </button>
        <a href="/repos" class="btn btn-secondary w-full sm:w-auto"
          >Back to repositories</a
        >
      </div>
    </section>
    <section class="panel panel-soft manual-source">
      <div>
        <p class="field-caption">Manual entry</p>
        <h2 class="mt-1">Add a repository by name</h2>
        <p class="mt-2 text-sm text-ink-2">
          Use this when discovery is unavailable or the repository is outside
          the filtered inventory.
        </p>
      </div>
      <div class="manual-source__fields">
        <label class="field-group"
          ><span class="field-group__label">Platform</span><select
            bind:value={manualPlatform}
            class="field"
            >{#if manualPlatform === null}<option value="" disabled hidden
                >Select a platform…</option
              >{/if}{#each SOURCES as s}<option value={s}>{s}{status !== null &&
              !isSourceEnabled(status, s)
              ? " (not configured)"
              : ""}</option>{/each}</select
          ></label
        >
        <label class="field-group"
          ><span class="field-group__label">Owner</span><input
            bind:value={manualOwner}
            placeholder="owner"
            class="field"
          /></label
        >
        <label class="field-group"
          ><span class="field-group__label">Repository</span><input
            bind:value={manualRepo}
            placeholder="repo"
            class="field"
          /></label
        >
        <button
          onclick={addManual}
          disabled={saving}
          class="btn btn-secondary manual-source__action"
          >{saving ? "Adding…" : "Add manually"}</button
        >
      </div>
    </section>
  {:else if status}
    {@const enabled = getEnabledSources(status)}
    <section
      class="panel panel-soft live-summary"
      aria-label="Repository discovery summary"
    >
      <span class="live-summary__item"
        ><span class="field-caption"
          >{enabled.length} platform{enabled.length !== 1 ? "s" : ""} configured</span
        ></span
      >
      <span class="live-summary__item"
        ><span class="field-caption"
          >{available.length} repo{available.length !== 1 ? "s" : ""} discoverable</span
        ></span
      >
      {#if selectedCount() > 0}
        <span class="live-summary__item live-summary__item--highlight"
          ><span class="field-caption">{selectedCount()} selected for add</span
          ></span
        >
      {/if}
      {#if Object.keys(rowResults).length > 0}
        <span class="live-summary__item live-summary__item--result"
          ><span class="field-caption">
            {Object.values(rowResults).filter((r) => r.status === "success")
              .length} added,
            {Object.values(rowResults).filter((r) => r.status === "failure")
              .length} failed
          </span></span
        >
      {/if}
    </section>

    {#if error}
      <ErrorBanner message={error} />
    {/if}

    {#if enabled.length === 0}
      <EmptyState
        heading="No platform is configured."
        description="Connect GitHub or Forgejo in server configuration before discovering repositories."
        actionLabel="Open settings"
        href="/settings"
      />
    {:else}
      {#if Object.keys(rowResults).length > 0}
        <div
          class="panel panel-soft batch-results"
          role="status"
          aria-live="polite"
        >
          <div class="batch-results__header">
            <p class="field-caption">Operation results</p>
          </div>
          {#if Object.values(rowResults).some((r) => r.status === "success")}
            <div class="batch-results__group batch-results__group--success">
              <p class="batch-results__group-label">Completed</p>
              <ul class="batch-results__list">
                {#each Object.entries(rowResults).filter(([, v]) => v.status === "success") as [key]}
                  <li class="batch-results__item">
                    {key}
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
          {#if Object.values(rowResults).some((r) => r.status === "failure")}
            <div class="batch-results__group batch-results__group--failure">
              <p class="batch-results__group-label">
                Failed — {Object.values(rowResults).filter(
                  (r) => r.status === "failure",
                ).length} item{Object.values(rowResults).filter(
                  (r) => r.status === "failure",
                ).length !== 1
                  ? "s"
                  : ""}
              </p>
              <ul class="batch-results__list">
                {#each Object.entries(rowResults).filter(([, v]) => v.status === "failure") as [key, v]}
                  <li class="batch-results__item">
                    {key}<span class="batch-results__item-error" title={v.error}
                      >{v.error}</span
                    >
                  </li>
                {/each}
              </ul>
            </div>
            <button
              onclick={retryFailed}
              disabled={saving}
              class="btn btn-primary mt-3"
            >
              {saving
                ? "Retrying…"
                : `Retry ${Object.values(rowResults).filter((r) => r.status === "failure").length} failed`}
            </button>
          {/if}
        </div>
      {/if}

      <section
        class="panel source-inventory"
        aria-label="Available repositories"
      >
        <div class="source-inventory__topline">
          <div>
            <p class="field-caption">Batch selection</p>
            <h2 class="mt-1">Available repositories</h2>
          </div>
          <button
            onclick={addSelected}
            disabled={selectedCount() === 0 || saving}
            class="btn btn-primary"
          >
            {saving ? "Adding…" : `Add ${selectedCount()} selected`}
          </button>
        </div>
        <div class="source-tabs" role="group" aria-label="Repository platforms">
          {#each SOURCES as source}
            {@const isEnabled = isSourceEnabled(status, source)}
            <button
              onclick={() => (activeSource = source)}
              onkeydown={handleSourceKeyDown}
              disabled={!isEnabled}
              class="source-tab {activeSource === source
                ? 'source-tab--active'
                : ''}"
              aria-pressed={activeSource === source}
              aria-label={isEnabled
                ? `${source} platform${activeSource === source ? ", selected" : ""}`
                : `${source} platform (not configured)`}
            >
              {source}
              {!isEnabled ? " (not configured)" : ""}
            </button>
          {/each}
        </div>

        <div class="source-filters">
          <label class="field-group min-w-0 flex-1"
            ><span class="field-group__label">Search repositories</span><input
              bind:value={search}
              type="text"
              placeholder="owner/repository"
              class="field"
            /></label
          >
          <label class="field-group"
            ><span class="field-group__label">Sort</span><select
              bind:value={sortBy}
              class="field"
              ><option value="name">Name</option><option value="activity"
                >Recent activity</option
              ></select
            ></label
          >
          <label class="check-control"
            ><input bind:checked={showForks} type="checkbox" /><span
              >Show forks</span
            ></label
          >
          <label class="check-control"
            ><input bind:checked={showSharedNamespaces} type="checkbox" /><span
              >Show organization &amp; shared namespaces</span
            ></label
          >
        </div>

        {#if filteredList.length === 0}
          <div class="empty-state source-empty">
            <p class="field-caption">No matching repositories</p>
            <p class="mt-2 text-sm text-ink-2">
              No available {activeSource} repositories match the current filters.
            </p>
          </div>
        {:else}
          <div class="source-list__meta">
            <span class="field-caption"
              >{filteredList.length} available in current view</span
            ><button
              onclick={() => {
                const allSelected = filteredList.every(
                  (r) => selected[repoKey(r)],
                );
                for (const r of filteredList) {
                  if (rowResults[repoKey(r)]?.status !== "success")
                    selected[repoKey(r)] = !allSelected;
                }
              }}
              class="btn btn-ghost"
              >{filteredList.every((r) => selected[repoKey(r)])
                ? "Deselect all"
                : "Select all"}</button
            >
          </div>
          <ul class="source-list">
            {#each filteredList as r (repoKey(r))}
              {@const rkey = repoKey(r)}
              {@const rowStatus = rowResults[rkey]?.status}
              <li
                class="source-row {rowStatus === 'success'
                  ? 'source-row--success'
                  : ''} {rowStatus === 'failure'
                  ? 'source-row--failure'
                  : ''} {rowStatus === 'pending' ? 'source-row--pending' : ''}"
              >
                <input
                  type="checkbox"
                  id={rkey}
                  checked={selected[rkey] ?? false}
                  disabled={rowStatus === "pending" || rowStatus === "success"}
                  onchange={(e) => (selected[rkey] = e.currentTarget.checked)}
                />
                <label for={rkey} class="source-row__label">
                  <span class="source-row__name">{r.owner}/<wbr />{r.repo}</span
                  >
                  <span class="source-row__meta"
                    >{r.platform}{r.ownNamespace === false
                      ? " · shared namespace"
                      : ""}{r.fork === true ? " · fork" : ""}</span
                  >
                </label>
                {#if rowStatus === "pending" || rowStatus === "success"}
                  <span class="text-sm" role="status"
                    >{rowStatus === "pending" ? "Adding…" : "Added"}</span
                  >
                {/if}
              </li>
            {/each}
          </ul>
          <button
            onclick={addSelected}
            disabled={selectedCount() === 0 || saving}
            class="btn btn-primary mt-5 w-full sm:w-auto"
            >{saving ? "Adding…" : `Add ${selectedCount()} selected`}</button
          >
        {/if}
      </section>
    {/if}

    <section class="panel panel-soft manual-source">
      <div>
        <p class="field-caption">Manual entry</p>
        <h2 class="mt-1">Add a repository by name</h2>
        <p class="mt-2 text-sm text-ink-2">
          Use this when discovery is unavailable or the repository is outside
          the filtered inventory.
        </p>
      </div>
      <div class="manual-source__fields">
        <label class="field-group"
          ><span class="field-group__label">Platform</span><select
            bind:value={manualPlatform}
            class="field"
            >{#if manualPlatform === null}<option value="" disabled hidden
                >Select a platform…</option
              >{/if}{#each SOURCES as s}<option value={s}>{s}{status !== null &&
              !isSourceEnabled(status, s)
              ? " (not configured)"
              : ""}</option>{/each}</select
          ></label
        >
        <label class="field-group"
          ><span class="field-group__label">Owner</span><input
            bind:value={manualOwner}
            placeholder="owner"
            class="field"
          /></label
        >
        <label class="field-group"
          ><span class="field-group__label">Repository</span><input
            bind:value={manualRepo}
            placeholder="repo"
            class="field"
          /></label
        >
        <button
          onclick={addManual}
          disabled={saving}
          class="btn btn-secondary manual-source__action"
          >{saving ? "Adding…" : "Add manually"}</button
        >
      </div>
    </section>
  {/if}
</div>
