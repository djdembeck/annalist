<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { getStatus, getAvailableRepos, addRepo, type AvailableRepo, type Status } from "$lib/api";

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
      if (enabled.length && !status?.[activeSource]) activeSource = enabled[0];
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
    if (!showSharedNamespaces) result = result.filter((r) => r.ownNamespace !== false);
    if (!showForks) result = result.filter((r) => r.fork !== true);
    const query = search.trim().toLowerCase();
    if (query) result = result.filter((r) => `${r.owner}/${r.repo}`.toLowerCase().includes(query));
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
      for (const r of targets) await addRepo(r);
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

<div class="trace-wall">
  <header class="section-head">
    <div>
      <p class="trace-label">Operations / source intake</p>
      <h1>Source inventory</h1>
      <p class="section-head__lede">Choose connected sources in batches, or place one repository on the wall by hand.</p>
    </div>
    <a href="/repos" class="btn btn-secondary">Back to repositories</a>
  </header>

  <section class="panel panel-soft intake-trace" aria-label="Repository intake workflow">
    <div class="intake-node"><span class="signal-dot" aria-hidden="true"></span><div><p class="trace-label">1 / Discover</p><p class="intake-node__title">Read enabled platforms</p></div></div>
    <div class="intake-trace__line" aria-hidden="true"></div>
    <div class="intake-node"><span class="signal-dot signal-dot--cyan" aria-hidden="true"></span><div><p class="trace-label">2 / Select</p><p class="intake-node__title">Mark repositories</p></div></div>
    <div class="intake-trace__line" aria-hidden="true"></div>
    <div class="intake-node"><span class="signal-dot signal-dot--heat" aria-hidden="true"></span><div><p class="trace-label">3 / Commit</p><p class="intake-node__title">Add selected sources</p></div></div>
  </section>

  {#if error}
    <div class="panel panel--error" role="alert"><p class="trace-label">Intake needs attention</p><p class="mt-2 text-sm text-alert">{error}</p></div>
  {/if}

  {#if loading}
    <section class="panel" aria-busy="true" aria-label="Loading available repositories">
      <p class="trace-label">Reading enabled source inventory</p>
      <div class="skeleton mt-4 h-11 w-full"></div><div class="skeleton mt-3 h-16 w-full"></div><div class="skeleton mt-3 h-16 w-full"></div>
    </section>
  {:else if status}
    {@const enabled = SOURCES.filter((s) => status?.[s])}
    {#if enabled.length === 0}
      <section class="panel empty-state text-left">
        <p class="trace-label">No source signal</p>
        <h2 class="mt-2">No platform is configured.</h2>
        <p class="mt-2 max-w-xl text-sm text-ink-2">Connect GitHub or Forgejo in server configuration before discovering repositories.</p>
        <a href="/settings" class="btn btn-primary mt-5">Open settings</a>
      </section>
    {:else}
      <section class="panel source-inventory" aria-label="Available repository source inventory">
        <div class="source-inventory__topline">
          <div><p class="trace-label">Batch selection</p><h2 class="mt-1">Repositories ready to connect</h2></div>
          <button onclick={addSelected} disabled={selectedCount() === 0 || saving} class="btn btn-primary">
            {saving ? "Adding…" : `Add ${selectedCount()} selected`}
          </button>
        </div>

        <div class="source-tabs" role="tablist" aria-label="Repository platforms">
          {#each SOURCES as source}
            {@const isEnabled = status[source]}
            <button role="tab" aria-selected={activeSource === source} onclick={() => (activeSource = source)} disabled={!isEnabled} class="source-tab {activeSource === source ? 'source-tab--active' : ''}">
              <span class="signal-dot {isEnabled ? 'signal-dot--cyan' : 'signal-dot--muted'}" aria-hidden="true"></span>{source}
              {!isEnabled ? " (not configured)" : ""}
            </button>
          {/each}
        </div>

        <div class="source-filters">
          <label class="field-group min-w-0 flex-1"><span class="field-group__label">Search repositories</span><input bind:value={search} type="text" placeholder="owner/repository" class="field" /></label>
          <label class="field-group"><span class="field-group__label">Sort</span><select bind:value={sortBy} class="field"><option value="name">Name</option><option value="activity">Recent activity</option></select></label>
          <label class="check-control"><input bind:checked={showForks} type="checkbox" /><span>Show forks</span></label>
          <label class="check-control"><input bind:checked={showSharedNamespaces} type="checkbox" /><span>Show organization &amp; shared namespaces</span></label>
        </div>

        {#if filtered().length === 0}
          <div class="empty-state source-empty"><p class="trace-label">No matching source</p><p class="mt-2 text-sm text-ink-2">No available {activeSource} repositories match the current filters.</p></div>
        {:else}
          <div class="source-list__meta"><span class="trace-label">{filtered().length} available in current view</span><button onclick={() => { const allSelected = filtered().every((r) => selected[repoKey(r)]); for (const r of filtered()) selected[repoKey(r)] = !allSelected; }} class="btn btn-ghost">{filtered().every((r) => selected[repoKey(r)]) ? "Deselect all" : "Select all"}</button></div>
          <ul class="source-list">
            {#each filtered() as r (repoKey(r))}
              <li class="source-row">
                <input type="checkbox" id={repoKey(r)} checked={selected[repoKey(r)] ?? false} onchange={(e) => (selected[repoKey(r)] = e.currentTarget.checked)} />
                <label for={repoKey(r)} class="source-row__label"><span class="source-row__name">{r.owner}/{r.repo}</span><span class="source-row__meta">{r.platform}{r.ownNamespace === false ? " · shared namespace" : ""}{r.fork === true ? " · fork" : ""}</span></label>
                <span class="signal-dot signal-dot--cyan" aria-hidden="true"></span>
              </li>
            {/each}
          </ul>
          <button onclick={addSelected} disabled={selectedCount() === 0 || saving} class="btn btn-primary mt-5 w-full sm:w-auto">{saving ? "Adding…" : `Add ${selectedCount()} selected`}</button>
        {/if}
      </section>
    {/if}

    <section class="panel panel-soft manual-source">
      <div><p class="trace-label">Manual entry</p><h2 class="mt-1">Place a source directly</h2><p class="mt-2 text-sm text-ink-2">Use this when discovery is unavailable or the repository is outside the filtered inventory.</p></div>
      <div class="manual-source__fields">
        <label class="field-group"><span class="field-group__label">Platform</span><select bind:value={manualPlatform} class="field">{#each SOURCES as s}<option value={s}>{s}</option>{/each}</select></label>
        <label class="field-group"><span class="field-group__label">Owner</span><input bind:value={manualOwner} placeholder="owner" class="field" /></label>
        <label class="field-group"><span class="field-group__label">Repository</span><input bind:value={manualRepo} placeholder="repo" class="field" /></label>
        <button onclick={addManual} disabled={saving} class="btn btn-secondary manual-source__action">{saving ? "Adding…" : "Add manually"}</button>
      </div>
    </section>
  {/if}
</div>
