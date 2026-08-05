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
  let sortBy = $state<"name" | "activity">("activity");

  let selected = $state<Record<string, boolean>>({});

  function repoKey(r: AvailableRepo): string {
    return `${r.platform}/${r.owner}/${r.repo}`;
  }

  async function load(): Promise<void> {
    try {
      status = await getStatus();
      available = await getAvailableRepos();
      error = "";
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
        const aT = new Date(a.pushedAt ?? a.updatedAt ?? 0).getTime();
        const bT = new Date(b.pushedAt ?? b.updatedAt ?? 0).getTime();
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
  <h1 class="mb-2 font-display text-3xl tracking-tight text-white">ADD REPOSITORIES</h1>
  <p class="mb-8 text-base text-ink-2">
    Choose repos from an enabled source, or add one manually.
  </p>

  {#if error}
    <div class="card mb-4 border-alert bg-alert/10">
      <p class="text-sm text-alert">{error}</p>
    </div>
  {/if}

  {#if loading}
    <div class="card" aria-busy="true" aria-label="Loading repositories">
      <div class="space-y-4">
        <div class="skeleton h-8 w-full"></div>
        <div class="skeleton h-24 w-full"></div>
        <div class="skeleton h-10 w-1/2"></div>
      </div>
    </div>
  {:else if status}
    {@const enabled = SOURCES.filter((s) => status?.[s])}
    {#if enabled.length === 0}
      <div class="card">
        <p class="text-base text-ink-2">
          No platform is configured. Connect GitHub or Forgejo in your server config first.
        </p>
        <a href="/settings" class="btn-base btn-primary mt-4">
          Open settings
        </a>
      </div>
    {:else}
      <div class="mb-4 flex flex-wrap items-center gap-4">
        <input
          bind:value={search}
          type="text"
          placeholder="Search repositories…"
          class="field min-w-0 flex-1"
        />
        <label class="flex items-center gap-2 text-sm text-ink-2">
          Sort
          <select bind:value={sortBy} class="field w-auto">
            <option value="name">Name</option>
            <option value="activity">Recent activity</option>
          </select>
        </label>
        <label class="flex cursor-pointer items-center gap-2 text-sm text-ink-2">
          <input bind:checked={showForks} type="checkbox" class="h-4 w-4 accent-mark" />
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
            class="btn-base relative transition-colors {activeSource === source
              ? 'text-heat after:absolute after:bottom-0 after:left-0 after:right-0 after:h-px after:bg-gradient-to-r after:from-cherry after:via-ember after:to-heat'
              : 'bg-surface-2 text-ink-2 hover:bg-surface-1'} {isEnabled ? '' : 'opacity-50'}"
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
              class="btn-link"
            >
              {items.every((r) => selected[repoKey(r)]) ? "Deselect all" : "Select all"}
            </button>
          </div>
          <ul class="max-h-80 overflow-auto rounded border border-line">
            {#each items as r (repoKey(r))}
              <li class="flex items-center gap-3 border-b border-line px-4 py-3 last:border-b-0 transition-colors hover:bg-row-hover">
                <input
                  type="checkbox"
                  id={repoKey(r)}
                  checked={selected[repoKey(r)] ?? false}
                  onchange={(e) => (selected[repoKey(r)] = e.currentTarget.checked)}
                  class="h-4 w-4 accent-mark"
                />
                <label for={repoKey(r)} class="flex-1 cursor-pointer text-sm text-ink">
                  {r.owner}/{r.repo}
                </label>
              </li>
            {/each}
          </ul>
          <button
            onclick={addSelected}
            disabled={selectedCount() === 0 || saving}
            class="btn-base btn-mark mt-5"
          >
            {saving ? "Adding…" : `Add ${selectedCount()} selected`}
          </button>
        {/if}
      </div>
    {/if}

    <div class="card mt-10">
      <h2 class="mb-5 font-sans text-base font-semibold text-ink">Add manually</h2>
      <div class="grid gap-4 sm:grid-cols-[120px_1fr_1fr_auto]">
        <label class="flex flex-col gap-2">
          <span class="text-xs text-ink-3">Platform</span>
          <select bind:value={manualPlatform} class="field">
            {#each SOURCES as s}
              <option value={s}>{s}</option>
            {/each}
          </select>
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-xs text-ink-3">Owner</span>
          <input bind:value={manualOwner} placeholder="owner" class="field" />
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-xs text-ink-3">Repository</span>
          <input bind:value={manualRepo} placeholder="repo" class="field" />
        </label>
        <div class="flex items-end">
          <button
            onclick={addManual}
            disabled={saving}
            class="btn-base btn-primary w-full"
          >
            Add
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>
