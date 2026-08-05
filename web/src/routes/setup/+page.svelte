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

  type Source = "github" | "forgejo";

  const SOURCES: Source[] = ["github", "forgejo"];
  const PRESET_OPTIONS = ["chronicler", "engineer", "launch"];
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
      "This release hardens the forge. Webhook deliveries now surface a live health check, so you can see at a glance whether GitHub is reaching your instance. Note generation runs faster under load thanks to result caching, and the retired v1 endpoints step aside after a long farewell tour.",
    engineer:
      "- Add webhook delivery health check\n- Cache note generation results for faster builds\n- Fix pagination on release history\n- Remove deprecated v1 endpoints",
    launch:
      "Time to shine — this update makes your forge faster and healthier than ever. Webhook health checks keep you in the loop, caching makes note generation snappy, and release history scrolls like butter.",
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
  let showForks = $state(false);
  let showSharedNamespaces = $state(false);
  let sortBy = $state<"name" | "activity">("activity");

  // --- step 4: voice ---
  let toneOption = $state("inherit");
  let customTone = $state("");

  function platformsConnected(): number {
    if (!status) return 0;
    return (status.github ? 1 : 0) + (status.forgejo ? 1 : 0);
  }

  function repoKey(r: AvailableRepo): string {
    return `${r.platform}/${r.owner}/${r.repo}`;
  }

  function filtered(): AvailableRepo[] {
    const query = search.trim().toLowerCase();
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
  }

  function selectedCount(): number {
    return Object.values(selected).filter(Boolean).length;
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
    if (platformsConnected() > 0) {
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
    if (selectedCount() === 0) return;
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
      saving = false;
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error = e instanceof Error ? e.message : "Failed to add repositories";
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
      step = 4;
    } catch (e) {
      saving = false;
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error = e instanceof Error ? e.message : "Failed to save default tone";
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
</svelte:head>

<div class="mx-auto max-w-3xl py-8">
  <h1 class="mb-2 font-display text-2xl text-white">Setup</h1>
  <p class="mb-8 text-base text-ink-2">
    A quick walkthrough to get your forge online.
  </p>

  <!-- Stepper -->
  <nav
    aria-label="Setup progress"
    class="sticky top-14 z-10 mb-8 border-b border-line bg-page pb-4"
  >
    <ol class="flex flex-wrap items-center gap-x-6 gap-y-3">
      {#each STEPS as label, i}
        <li>
          {#if i < step}
            <button
              onclick={() => goTo(i)}
              class="inline-flex items-center gap-2 rounded text-sm text-ink-2 transition-colors hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
            >
              <svg
                class="h-4 w-4 text-ok"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2.5"
                aria-hidden="true"
              >
                <path d="M5 13l4 4L19 7" stroke-linecap="round" stroke-linejoin="round"></path>
              </svg>
              <span class="capitalize">{label}</span>
            </button>
          {:else if i === step}
            <span
              aria-current="step"
              class="relative inline-flex items-center gap-2 rounded text-sm font-medium text-heat"
            >
              <span
                class="flex h-5 w-5 items-center justify-center rounded border border-heat text-xs"
                >{i + 1}</span
              >
              <span class="capitalize">{label}</span>
              <span
                class="absolute -bottom-2 left-0 right-0 h-px bg-gradient-to-r from-cherry via-ember to-heat"
              ></span>
            </span>
          {:else}
            <span class="inline-flex items-center gap-2 rounded text-sm text-ink-3">
              <span
                class="flex h-5 w-5 items-center justify-center rounded border border-line text-xs"
                >{i + 1}</span
              >
              <span class="capitalize">{label}</span>
            </span>
          {/if}
        </li>
      {/each}
    </ol>
  </nav>

  <div aria-live="polite">
    {#if error}
      <p class="mb-4 rounded border border-alert/30 bg-alert/10 p-3 text-sm text-alert">
        {error}
      </p>
    {/if}
  </div>

  <!-- Step 1 — Welcome & Token -->
  {#if step === 0}
    <section class="space-y-6 rounded border border-line bg-surface-1 p-6">
      <div class="space-y-2">
        <h2 class="font-display text-2xl text-white">Welcome to your forge</h2>
        <p class="text-base text-ink-2">
          Annalist is self-hosted. The admin token you provide protects every admin
          action on this instance, so keep it private.
        </p>
      </div>
      <form
        class="flex flex-col gap-4"
        onsubmit={(e) => {
          e.preventDefault();
          submitToken();
        }}
      >
        <label class="flex flex-col gap-2">
          <span class="text-sm text-ink-2">ADMIN_TOKEN</span>
          <input
            type="password"
            bind:value={adminToken}
            placeholder="Paste your admin token"
            class="rounded border border-line-strong bg-page px-4 py-2.5 text-ink outline-none focus:border-focus focus-visible:outline-2 focus-visible:outline-focus-ring"
          />
        </label>
        <button
          type="submit"
          disabled={!adminToken.trim() || busy}
          class="inline-flex w-fit items-center gap-2 rounded bg-gradient-to-r from-cherry via-ember to-heat px-5 py-2.5 text-sm font-bold text-page hover:brightness-110 disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
        >
          {busy ? "Connecting…" : "Continue"}
        </button>
      </form>
    </section>
  {/if}

  <!-- Step 2 — Connect platforms -->
  {#if step === 1}
    <section class="space-y-6">
      <div class="grid gap-4 sm:grid-cols-2">
        {#each SOURCES as source}
          {@const configured = status?.[source] ?? false}
          <div
            class="rounded border border-line bg-surface-1 p-5"
            class:opacity-90={!configured}
          >
            <div class="flex items-center gap-3">
              {#if configured}
                <span class="flex h-6 w-6 items-center justify-center rounded bg-ok/15 text-ok">
                  <svg
                    class="h-4 w-4"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                    aria-hidden="true"
                  >
                    <path d="M5 13l4 4L19 7" stroke-linecap="round" stroke-linejoin="round"></path>
                  </svg>
                </span>
              {:else}
                <span class="flex h-6 w-6 items-center justify-center rounded bg-alert/15 text-alert">
                  <svg
                    class="h-4 w-4"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    aria-hidden="true"
                  >
                    <line x1="6" y1="6" x2="18" y2="18" stroke-linecap="round"></line>
                    <line x1="18" y1="6" x2="6" y2="18" stroke-linecap="round"></line>
                  </svg>
                </span>
              {/if}
              <div>
                <p class="font-medium capitalize text-ink">{source}</p>
                <p class="text-sm text-ink-2">{configured ? "Configured" : "Not configured"}</p>
              </div>
            </div>

            {#if !configured}
              <ol class="mt-5 list-inside list-decimal space-y-2 text-sm text-ink-2">
                {#if source === "github"}
                  <li>
                    Create a GitHub App at
                    <span class="text-ink"
                      >Settings → Developer settings → GitHub Apps → New</span
                    >.
                  </li>
                  <li>
                    Set the webhook URL to
                    <span class="break-all text-ink">https://YOUR_ANNALIST_HOST/webhook/github</span>
                    and subscribe to
                    <span class="text-ink">Release</span> events.
                  </li>
                  <li>
                    Generate and store a private key; set the env vars
                    <span class="text-ink"
                      >GITHUB_APP_ID</span
                    >, <span class="text-ink">GITHUB_INSTALLATION_ID</span>,
                    <span class="text-ink">GITHUB_WEBHOOK_SECRET</span> and
                    <span class="text-ink">GITHUB_PRIVATE_KEY</span>.
                  </li>
                  <li>
                    Install the app on the repositories you want to manage.
                  </li>
                {:else}
                  <li>
                    In your Forgejo repo or org settings, add a webhook URL
                    <span class="break-all text-ink">https://YOUR_ANNALIST_HOST/webhook/forgejo</span>.
                  </li>
                  <li>
                    Subscribe to <span class="text-ink">Release</span> events and set the
                    secret to <span class="text-ink">FORGEJO_WEBHOOK_SECRET</span>.
                  </li>
                {/if}
              </ol>
            {/if}
          </div>
        {/each}
      </div>

      <div
        class="rounded border border-line bg-surface-1 p-5 text-sm text-ink-2"
      >
        <p>
          Annalist only reads release events and commit history. It cannot merge PRs,
          push code, or access source outside the repositories you add.
        </p>
      </div>

      <div class="flex flex-col gap-3">
        <button
          onclick={advanceToRepos}
          disabled={platformsConnected() === 0}
          class="inline-flex w-fit items-center gap-2 rounded bg-gradient-to-r from-cherry via-ember to-heat px-5 py-2.5 text-sm font-bold text-page hover:brightness-110 disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
        >
          Continue
        </button>
        {#if skipWarning}
          <p class="text-sm text-alert">
            No platform is configured yet, so you won't be able to add repositories.
            You can still continue and configure things later.
          </p>
          <button
            onclick={continueFromPlatforms}
            class="inline-flex w-fit items-center gap-2 rounded border border-line-strong px-4 py-2 text-sm text-ink-2 hover:bg-surface-2 hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
          >
            Skip for now
          </button>
        {:else if platformsConnected() === 0}
          <button
            onclick={skipPlatforms}
            class="inline-flex w-fit items-center gap-2 rounded border border-line-strong px-4 py-2 text-sm text-ink-2 hover:bg-surface-2 hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
          >
            Skip setup for now
          </button>
        {/if}
      </div>
    </section>
  {/if}

  <!-- Step 3 — Add repositories -->
  {#if step === 2}
    <section class="space-y-6 rounded border border-line bg-surface-1 p-6">
      {#if platformsConnected() === 0}
        <div class="space-y-4">
          <p class="text-base text-ink-2">
            No platforms are configured, so there are no repositories to add yet.
          </p>
          <button
            onclick={() => (step = 1)}
            class="inline-flex w-fit items-center gap-2 rounded border border-line-strong px-4 py-2 text-sm text-ink-2 hover:bg-surface-2 hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
          >
            Go back and connect a platform
          </button>
        </div>
      {:else if !reposVisible}
        <div class="flex flex-col gap-3">
          <p class="text-base text-ink-2">
            Load the repositories Annalist can see from your connected platforms.
          </p>
          <button
            onclick={enterRepos}
            class="inline-flex w-fit items-center gap-2 rounded bg-gradient-to-r from-cherry via-ember to-heat px-5 py-2.5 text-sm font-bold text-page hover:brightness-110 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
          >
            Load repositories
          </button>
          <button
            onclick={() => (step = 3)}
            class="inline-flex w-fit items-center gap-2 rounded border border-line-strong px-4 py-2 text-sm text-ink-2 hover:bg-surface-2 hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
          >
            Skip — I'll add repositories later
          </button>
        </div>
      {:else}
        <div class="flex flex-col gap-3">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
            <input
              type="search"
              placeholder="Search repositories…"
              bind:value={search}
              class="rounded border border-line-strong bg-page px-4 py-2.5 text-ink outline-none placeholder:text-ink-3 focus:border-focus focus-visible:outline-2 focus-visible:outline-focus-ring"
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
          </div>
          <label class="flex w-fit cursor-pointer items-center gap-2 text-sm text-ink-2">
            <input
              type="checkbox"
              bind:checked={showForks}
              class="h-4 w-4 accent-mark"
            />
            Show forks
          </label>
          <label class="flex w-fit cursor-pointer items-center gap-2 text-sm text-ink-2">
            <input
              type="checkbox"
              bind:checked={showSharedNamespaces}
              class="h-4 w-4 accent-mark"
            />
            Show organization &amp; shared namespaces
          </label>
        </div>

        <div class="flex gap-2">
          {#each SOURCES as source}
            {@const isEnabled = status?.[source] ?? false}
            <button
              onclick={() => (activeSource = source)}
              disabled={!isEnabled}
              class="relative rounded px-4 py-2.5 text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
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

        {#if loading}
          <p class="text-base text-ink-3">Loading…</p>
        {:else}
          {@const items = filtered()}
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
                class="text-sm text-ink-2 hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
              >
                {items.every((r) => selected[repoKey(r)]) ? "Deselect all" : "Select all"}
              </button>
            </div>
            <ul class="max-h-80 overflow-auto rounded border border-line">
              {#each items as r (repoKey(r))}
                <li
                  class="flex items-center gap-3 border-b border-line px-4 py-3 last:border-b-0 transition-colors hover:bg-row-hover"
                >
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
              class="mt-5 inline-flex items-center gap-2 rounded bg-gradient-to-r from-cherry via-ember to-heat px-5 py-2.5 text-sm font-bold text-page hover:brightness-110 disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
            >
              {saving ? "Adding…" : `Add ${selectedCount()} selected`}
            </button>
          {/if}
        {/if}
      {/if}

      <div class="rounded border border-line bg-surface-1 p-4 text-sm text-ink-2">
        <p>
          When you add a repository, Annalist can read its release webhooks and commit
          history to write release notes. It writes only to the release body.
        </p>
      </div>
    </section>
  {/if}

  <!-- Step 4 — Choose voice -->
  {#if step === 3}
    <section class="space-y-6 rounded border border-line bg-surface-1 p-6">
      <div class="space-y-2">
        <h2 class="font-display text-2xl text-white">Choose your voice</h2>
        <p class="text-base text-ink-2">
          This becomes the default tone for every release note. You can override it
          per repository later.
        </p>
      </div>

      <label class="flex flex-col gap-2">
        <span class="text-sm text-ink-2">Tone</span>
        <select
          bind:value={toneOption}
          class="rounded border border-line-strong bg-page px-4 py-2.5 text-base text-ink outline-none focus:border-focus focus-visible:outline-2 focus-visible:outline-focus-ring"
        >
          <option value="inherit">Inherit (server default)</option>
          {#each PRESET_OPTIONS as p (p)}
            <option value={p}>{p}</option>
          {/each}
          <option value="custom">Custom…</option>
        </select>
      </label>

      {#if toneOption !== "custom" && toneOption !== "inherit"}
        <p class="text-sm text-ink-2">{PRESET_DESCRIPTIONS[toneOption] ?? ""}</p>
      {/if}

      {#if toneOption === "custom"}
        <label class="flex flex-col gap-2">
          <span class="text-sm text-ink-2">Custom tone</span>
          <input
            bind:value={customTone}
            placeholder="Freeform persona"
            class="rounded border border-line-strong bg-page px-4 py-2.5 text-base text-ink outline-none focus:border-focus focus-visible:outline-2 focus-visible:outline-focus-ring"
          />
        </label>
      {/if}

      <div class="rounded border border-line bg-surface-2 p-4 text-xs text-ink-3">
        Sample commits used in the synthetic examples below:
        <ul class="mt-1 list-inside list-disc space-y-0.5">
          {#each SAMPLE_COMMITS as c (c)}
            <li class="font-mono">{c}</li>
          {/each}
        </ul>
      </div>

      <div class="space-y-4">
        {#each PRESET_OPTIONS as p (p)}
          <div class="rounded border border-line bg-page p-4">
            <div class="mb-2 flex items-center justify-between">
              <span class="font-medium capitalize text-ink">{p}</span>
              <span class="text-xs text-ink-3">Synthetic example</span>
            </div>
            <p class="text-sm text-ink-3">{PRESET_DESCRIPTIONS[p]}</p>
            <p class="mt-3 whitespace-pre-line text-sm text-ink-2">
              {SYNTHETIC_EXAMPLES[p]}
            </p>
          </div>
        {/each}
      </div>

      <button
        onclick={finishSetup}
        disabled={saving}
        class="inline-flex w-fit items-center gap-2 rounded bg-gradient-to-r from-cherry via-ember to-heat px-5 py-2.5 text-sm font-bold text-page hover:brightness-110 disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
      >
        {saving ? "Saving…" : "Finish setup"}
      </button>
    </section>
  {/if}

  <!-- Step 5 — Done -->
  {#if step === 4}
    <section class="space-y-6 rounded border border-line bg-surface-1 p-6">
      <h2 class="font-display text-2xl text-white">Your forge is ready.</h2>
      <dl class="grid gap-4 text-sm sm:grid-cols-2">
        <div class="flex items-center justify-between rounded border border-line bg-surface-2 px-4 py-3">
          <dt class="text-ink-2">Token</dt>
          <dd class="text-ok">{adminToken.trim() ? "Set" : "Not set"}</dd>
        </div>
        <div class="flex items-center justify-between rounded border border-line bg-surface-2 px-4 py-3">
          <dt class="text-ink-2">Platforms connected</dt>
          <dd class="text-ink">{platformsConnected()}</dd>
        </div>
        <div class="flex items-center justify-between rounded border border-line bg-surface-2 px-4 py-3">
          <dt class="text-ink-2">Repositories added</dt>
          <dd class="text-ink">{reposAdded}</dd>
        </div>
        <div class="flex items-center justify-between rounded border border-line bg-surface-2 px-4 py-3">
          <dt class="text-ink-2">Default tone</dt>
          <dd class="text-ink capitalize">{toneLabel()}</dd>
        </div>
      </dl>
      <div class="flex flex-wrap items-center gap-4">
        <a
          href="/repos"
          class="inline-flex items-center gap-2 rounded bg-gradient-to-r from-cherry via-ember to-heat px-5 py-2.5 text-sm font-bold text-page hover:brightness-110 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
        >
          Go to Repos
        </a>
        <a
          href="/settings"
          class="inline-flex items-center gap-2 rounded border border-line-strong px-4 py-2 text-sm text-ink-2 hover:bg-surface-2 hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring"
        >
          Open Settings
        </a>
      </div>
    </section>
  {/if}
</div>
