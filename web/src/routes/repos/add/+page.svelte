<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import {
    getStatus,
    getAvailableRepos,
    addRepo,
    type AvailableRepo,
    type Status,
  } from "$lib/api";

  type Source = "github" | "forgejo";

  const SOURCES: Source[] = ["github", "forgejo"];

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
  let sortBy = $state<"name" | "activity">("name");

  let selected = $state<Record<string, boolean>>({});

  function repoKey(r: AvailableRepo): string {
    return `${r.platform}/${r.owner}/${r.repo}`;
  }

  async function load(): Promise<void> {
    try {
      status = await getStatus();
      available = await getAvailableRepos();
      error = "";
      // Default to the first enabled source.
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

  onMount(load);

  function filtered(): AvailableRepo[] {
    let result = available.filter((r) => r.platform === activeSource);
    if (!showSharedNamespaces) {
      result = result.filter((r) => r.ownNamespace !== false);
    }
    if (!showForks) {
      result = result.filter((r) => r.fork !== true);
    }
    const query = search.trim().toLowerCase();
    if (query) {
      result = result.filter((r) =>
        `${r.owner}/${r.repo}`.toLowerCase().includes(query),
      );
    }
    if (sortBy === "activity") {
      return result.sort((a, b) => {
        const aT = new Date(a.updatedAt ?? 0).getTime();
        const bT = new Date(b.updatedAt ?? 0).getTime();
        if (aT !== bT) return bT - aT;
        if (a.owner !== b.owner) return a.owner.localeCompare(b.owner);
        return a.repo.localeCompare(b.repo);
      });
    }
    return result.sort((a, b) => {
      const aOwn = a.ownNamespace === false ? 1 : 0;
      const bOwn = b.ownNamespace === false ? 1 : 0;
      if (aOwn !== bOwn) return aOwn - bOwn;
      const aFork = a.fork === true ? 1 : 0;
      const bFork = b.fork === true ? 1 : 0;
      if (aFork !== bFork) return aFork - bFork;
      if (a.owner !== b.owner) return a.owner.localeCompare(b.owner);
      return a.repo.localeCompare(b.repo);
    });
  }

  function selectedCount(): number {
    return Object.values(selected).filter(Boolean).length;
  }

  async function addSelected(): Promise<void> {
    saving = true;
    error = "";
    const targets = available.filter((r) => selected[repoKey(r)]);
    try {
      for (const r of targets) {
        await addRepo(r);
      }
      goto("/repos");
    } catch (e) {
      saving = false;
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error = e instanceof Error ? e.message : "Failed to add repositories";
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
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error = e instanceof Error ? e.message : "Failed to add repository";
    }
  }
</script>

<div class="mx-auto max-w-3xl">
  <h1 class="mb-2 font-display text-3xl text-white">ADD REPOSITORIES</h1>
  <p class="mb-8 text-base text-ink-2">
    Choose repos from an enabled source, or add one manually.
  </p>

  {#if error}
    <p class="mb-4 text-sm text-alert">{error}</p>
  {/if}

  {#if loading}
    <p class="text-base text-ink-3">Loading…</p>
  {:else if status}
    {@const enabled = SOURCES.filter((s) => status?.[s])}
    {#if enabled.length === 0}
      <div class="rounded border border-line bg-surface-1 p-4 text-base text-ink-2">
        No platform is configured. Connect GitHub or Forgejo in your server config first.
      </div>
    {:else}
      <div class="mb-4 flex flex-wrap items-center gap-4">
        <input
          bind:value={search}
          type="text"
          placeholder="Search repositories…"
          class="min-w-0 flex-1 rounded border border-line-strong bg-page px-4 py-2.5 text-base text-ink outline-none focus:border-focus focus:outline-2 focus:outline-focus-ring"
        />
        <label class="flex items-center gap-2 text-sm text-ink-2">
          Sort
          <select
            bind:value={sortBy}
            class="rounded border border-line-strong bg-page px-4 py-2.5 text-base text-ink outline-none focus:border-focus focus-visible:outline-2 focus-visible:outline-focus-ring"
          >
            <option value="name">Name</option>
            <option value="activity">Recent activity</option>
          </select>
        </label>
        <label class="flex cursor-pointer items-center gap-2 text-sm text-ink-2">
          <input
            bind:checked={showForks}
            type="checkbox"
            class="h-4 w-4 accent-mark"
          />
          Show forks
        </label>
        <label class="flex cursor-pointer items-center gap-2 text-sm text-ink-2">
          <input
            bind:checked={showSharedNamespaces}
            type="checkbox"
            class="h-4 w-4 accent-mark"
          />
          Show organization &amp; shared namespaces
        </label>
      </div>
      <div class="mb-8 flex gap-2">
        {#each SOURCES as source}
          {@const isEnabled = status[source]}
          <button
            onclick={() => (activeSource = source)}
            disabled={!isEnabled}
            class="relative rounded px-4 py-2.5 text-sm font-medium transition-colors focus:outline-2 focus:outline-offset-2 focus:outline-focus-ring"
            class:bg-surface-2={activeSource !== source}
            class:text-ink-2={activeSource !== source}
            class:hover:bg-surface-1={activeSource !== source}
            class:bg-surface-1={activeSource === source}
            class:text-heat={activeSource === source}
            class:opacity-50={!isEnabled}
            class:after:absolute={activeSource === source}
            class:after:bottom-0={activeSource === source}
            class:after:left-0={activeSource === source}
            class:after:right-0={activeSource === source}
            class:after:h-px={activeSource === source}
            class:after:bg-gradient-to-r={activeSource === source}
            class:after:from-cherry={activeSource === source}
            class:after:via-ember={activeSource === source}
            class:after:to-heat={activeSource === source}
          >
            {source}
          </button>
        {/each}
      </div>

      {@const items = filtered()}
      <div class="rounded border border-line bg-surface-1 p-5">
        {#if items.length === 0}
          <p class="text-base text-ink-3">
            No available {activeSource} repositories found.
          </p>
        {:else}
          <div class="mb-4 flex items-center justify-between">
            <span class="text-sm text-ink-3">{items.length} available</span>
            <button
              onclick={() => {
                const allSelected = items.every((r) => selected[repoKey(r)]);
                for (const r of items) {
                  selected[repoKey(r)] = !allSelected;
                }
              }}
              class="text-sm text-ink-2 hover:text-ink focus:outline-2 focus:outline-offset-2 focus:outline-focus-ring"
            >
              {items.every((r) => selected[repoKey(r)]) ? "Deselect all" : "Select all"}
            </button>
          </div>
          <ul class="max-h-80 overflow-auto rounded border border-line">
            {#each items as r (repoKey(r))}
              <li class="flex items-center gap-3 border-b border-line px-4 py-3 last:border-b-0 transition-colors hover:bg-row-hover hover:border-b-row-hover-line last:hover:border-b-transparent">
                <input
                  type="checkbox"
                  id={repoKey(r)}
                  checked={selected[repoKey(r)] ?? false}
                  onchange={(e) => (selected[repoKey(r)] = e.currentTarget.checked)}
                  class="h-4 w-4 accent-mark"
                />
                <label for={repoKey(r)} class="flex-1 cursor-pointer text-base text-ink">
                  {r.owner}/{r.repo}
                </label>
              </li>
            {/each}
          </ul>
          <button
            onclick={addSelected}
            disabled={selectedCount() === 0 || saving}
            class="mt-5 inline-flex items-center gap-2 rounded bg-mark bg-gradient-to-r from-cherry via-ember to-heat px-5 py-2.5 text-sm font-bold text-page hover:brightness-110 disabled:opacity-50 focus:outline-2 focus:outline-offset-2 focus:outline-focus-ring"
          >
            {saving ? "Adding…" : `Add ${selectedCount()} selected`}
          </button>
        {/if}
      </div>
    {/if}

    <div class="mt-10 rounded border border-line bg-surface-2-warm p-5">
      <h2 class="mb-5 font-sans text-base font-semibold text-ink">Add manually</h2>
      <div class="grid gap-4 sm:grid-cols-[120px_1fr_1fr_auto]">
        <label class="flex flex-col gap-2">
          <span class="text-xs text-ink-3">Platform</span>
          <select
            bind:value={manualPlatform}
            class="rounded border border-line-strong bg-page px-4 py-2.5 text-base text-ink outline-none focus:border-focus focus:outline-2 focus:outline-focus-ring"
          >
            {#each SOURCES as s}
              <option value={s}>{s}</option>
            {/each}
          </select>
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-xs text-ink-3">Owner</span>
          <input
            bind:value={manualOwner}
            placeholder="owner"
            class="rounded border border-line-strong bg-page px-4 py-2.5 text-base text-ink outline-none focus:border-focus focus:outline-2 focus:outline-focus-ring"
          />
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-xs text-ink-3">Repository</span>
          <input
            bind:value={manualRepo}
            placeholder="repo"
            class="rounded border border-line-strong bg-page px-4 py-2.5 text-base text-ink outline-none focus:border-focus focus:outline-2 focus:outline-focus-ring"
          />
        </label>
        <div class="flex items-end">
          <button
            onclick={addManual}
            disabled={saving}
            class="w-full rounded bg-control px-4 py-2.5 text-sm font-medium text-ink hover:bg-control-hover disabled:opacity-50 focus:outline-2 focus:outline-offset-2 focus:outline-focus-ring"
          >
            Add
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>
