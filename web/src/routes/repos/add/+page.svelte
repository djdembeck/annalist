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
  import { handleAuthError, formatError } from "$lib/composables/useAuthErrorHandler";

  let status = $state<Status | null>(null);
  let available = $state<AvailableRepo[]>([]);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state("");
  let activeSource = $state<Source>("github");
  let manualPlatform = $state<Source>("github");
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
      available = await getAvailableRepos();
      error = "";
      const enabled = getEnabledSources(status);
      if (enabled.length && !isSourceEnabled(status, activeSource))
        activeSource = enabled[0];
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

    const successCount = await batchAddRepos(targets, results, addRepo);
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
      results,
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

<div class="trace-wall">
  <SectionHead
    label="Operations / source intake"
    title="Source inventory"
    lede="Choose connected sources in batches, or place one repository on the wall by hand."
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
      <p class="trace-label">Reading enabled source inventory</p>
      <div class="skeleton mt-4 h-11 w-full"></div>
      <div class="skeleton mt-3 h-16 w-full"></div>
      <div class="skeleton mt-3 h-16 w-full"></div>
    </section>
  {:else if error}
    <section
      class="panel panel--error"
      role="alert"
      aria-label="Source intake error"
    >
      <p class="trace-label">Intake failed</p>
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
        <p class="trace-label">Manual entry</p>
        <h2 class="mt-1">Place a source directly</h2>
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
            >{#each SOURCES as s}<option value={s}>{s}</option>{/each}</select
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
    <!-- Live summary replaces static 3-step trace -->
    {@const enabled = getEnabledSources(status)}
    <section
      class="panel panel-soft live-summary"
      aria-label="Source intake summary"
    >
      <span class="live-summary__item"
        ><span
          class="signal-dot {enabled.length > 0
            ? 'signal-dot--cyan'
            : 'signal-dot--muted'}"
          aria-hidden="true"
        ></span><span class="trace-label"
          >{enabled.length} platform{enabled.length !== 1 ? "s" : ""} connected</span
        ></span
      >
      <span class="live-summary__item"
        ><span class="trace-label"
          >{available.length} repo{available.length !== 1 ? "s" : ""} discoverable</span
        ></span
      >
      {#if selectedCount() > 0}
        <span class="live-summary__item live-summary__item--highlight"
          ><span class="signal-dot signal-dot--heat" aria-hidden="true"
          ></span><span class="trace-label"
            >{selectedCount()} selected for add</span
          ></span
        >
      {/if}
      {#if Object.keys(rowResults).length > 0}
        <span class="live-summary__item live-summary__item--result"
          ><span class="trace-label">
            {Object.values(rowResults).filter((r) => r.status === "success")
              .length} added,
            {Object.values(rowResults).filter((r) => r.status === "failure")
              .length} failed
          </span></span
        >
      {/if}
    </section>

    {#if error}
      <ErrorBanner label="Intake needs attention" message={error} />
    {/if}

    {#if enabled.length === 0}
      <EmptyState
        label="No source signal"
        heading="No platform is configured."
        description="Connect GitHub or Forgejo in server configuration before discovering repositories."
        actionLabel="Open settings"
        href="/settings"
      />
    {:else}
      <!-- Batch operation results banner -->
      {#if Object.keys(rowResults).length > 0}
        <div
          class="panel panel-soft batch-results"
          role="status"
          aria-live="polite"
        >
          <div class="batch-results__header">
            <p class="trace-label">Operation results</p>
          </div>
          {#if Object.values(rowResults).some((r) => r.status === "success")}
            <div class="batch-results__group batch-results__group--success">
              <p class="batch-results__group-label">Completed</p>
              <ul class="batch-results__list">
                {#each Object.entries(rowResults).filter(([, v]) => v.status === "success") as [key]}
                  <li class="batch-results__item">
                    <span class="signal-dot signal-dot--cyan" aria-hidden="true"
                    ></span>{key}
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
                    <span class="signal-dot signal-dot--heat" aria-hidden="true"
                    ></span>{key}<span
                      class="batch-results__item-error"
                      title={v.error}>{v.error}</span
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
        aria-label="Available repository source inventory"
      >
        <div class="source-inventory__topline">
          <div>
            <p class="trace-label">Batch selection</p>
            <h2 class="mt-1">Repositories ready to connect</h2>
          </div>
          <button
            onclick={addSelected}
            disabled={selectedCount() === 0 || saving}
            class="btn btn-primary"
          >
            {saving ? "Adding…" : `Add ${selectedCount()} selected`}
          </button>
        </div>

        <!-- Source switching as button group, not tabs -->
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
              <span
                class="signal-dot {isEnabled
                  ? 'signal-dot--cyan'
                  : 'signal-dot--muted'}"
                aria-hidden="true"
              ></span>
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
            <p class="trace-label">No matching source</p>
            <p class="mt-2 text-sm text-ink-2">
              No available {activeSource} repositories match the current filters.
            </p>
          </div>
        {:else}
          <div class="source-list__meta">
            <span class="trace-label"
              >{filteredList.length} available in current view</span
            ><button
              onclick={() => {
                const allSelected = filteredList.every(
                  (r) => selected[repoKey(r)],
                );
                for (const r of filteredList)
                  selected[repoKey(r)] = !allSelected;
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
                  disabled={rowStatus === "pending"}
                  onchange={(e) => (selected[rkey] = e.currentTarget.checked)}
                />
                <label for={rkey} class="source-row__label">
                  <span class="source-row__name">{r.owner}/{r.repo}</span>
                  <span class="source-row__meta"
                    >{r.platform}{r.ownNamespace === false
                      ? " · shared namespace"
                      : ""}{r.fork === true ? " · fork" : ""}</span
                  >
                </label>
                {#if rowStatus === "pending"}
                  <span class="signal-dot signal-dot--muted" aria-hidden="true"
                  ></span>
                {:else if rowStatus === "success"}
                  <span class="signal-dot signal-dot--cyan" aria-hidden="true"
                  ></span>
                {:else if rowStatus === "failure"}
                  <span
                    class="signal-dot signal-dot--heat"
                    aria-hidden="true"
                    title={rowResults[rkey]?.error}
                  ></span>
                {:else}
                  <span class="signal-dot signal-dot--cyan" aria-hidden="true"
                  ></span>
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
        <p class="trace-label">Manual entry</p>
        <h2 class="mt-1">Place a source directly</h2>
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
            >{#each SOURCES as s}<option value={s}>{s}</option>{/each}</select
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

<style>
  .live-summary {
    display: flex;
    flex-wrap: wrap;
    gap: var(--trace-space-4);
    align-items: center;
  }
  .live-summary__item {
    display: inline-flex;
    align-items: center;
    gap: var(--trace-space-2);
  }
  .live-summary__item--highlight {
    font-weight: 600;
  }
  .batch-results {
    display: grid;
    gap: var(--trace-space-3);
  }
  .batch-results__header {
    display: flex;
    align-items: center;
    gap: var(--trace-space-2);
  }
  .batch-results__group {
    display: grid;
    gap: var(--trace-space-2);
  }
  .batch-results__group-label {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .batch-results__group--success .batch-results__group-label {
    color: var(--trace-cyan);
  }
  .batch-results__group--failure .batch-results__group-label {
    color: var(--trace-red);
  }
  .batch-results__list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: var(--trace-space-1);
  }
  .batch-results__item {
    display: flex;
    align-items: center;
    gap: var(--trace-space-2);
    font-family: var(--font-mono);
    font-size: 0.8125rem;
    color: var(--trace-ink);
    overflow-wrap: anywhere;
  }
  .batch-results__item-error {
    color: var(--trace-muted);
    font-size: 0.6875rem;
    margin-left: auto;
    max-width: 20rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .source-row--success {
    background: color-mix(in srgb, var(--trace-cyan) 8%, var(--trace-panel));
  }
  .source-row--failure {
    background: color-mix(in srgb, var(--trace-red) 8%, var(--trace-panel));
  }
  .source-row--pending {
    opacity: 0.6;
  }
</style>
