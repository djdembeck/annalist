<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { getRepos, putRepoSettings, generate, type Repo } from "$lib/api";

  const PRESETS = ["chronicler", "engineer", "launch"];
  const PRESET_OPTIONS = [...PRESETS, "custom"];

  type Draft = {
    toneOption: string;
    customTone: string;
    instructions: string;
    model: string;
    temperature: string;
    trigger: string;
  };

  let repos = $state<Repo[]>([]);
  let loading = $state(true);
  let error = $state("");
  let expanded = $state<Record<string, boolean>>({});
  let drafts = $state<Record<string, Draft>>({});
  let force = $state<Record<string, boolean>>({});
  let notesOut = $state<Record<string, string | null>>({});
  let genError = $state<Record<string, string | null>>({});

  function rowKey(r: Repo): string {
    return `${r.platform}/${r.owner}/${r.repo}`;
  }

  async function refresh(): Promise<void> {
    try {
      repos = await getRepos();
      error = "";
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error = e instanceof Error ? e.message : "Failed to load repos";
    } finally {
      loading = false;
    }
  }

  onMount(refresh);

  function toggleEnabled(r: Repo): void {
    putRepoSettings(r.platform, r.owner, r.repo, { enabled: !r.enabled })
      .then(refresh)
      .catch((e) => {
        error = e instanceof Error ? e.message : "Failed to update";
      });
  }

  function openSettings(r: Repo): void {
    const key = rowKey(r);
    const tone = r.tone ?? "";
    const isPreset = PRESETS.includes(tone);
    drafts[key] = {
      toneOption: !tone ? "inherit" : isPreset ? tone : "custom",
      customTone: isPreset ? "" : tone,
      instructions: r.instructions ?? "",
      model: r.model ?? "",
      temperature: r.temperature === null ? "" : String(r.temperature),
      trigger: r.trigger ?? "auto",
    };
    expanded[key] = !expanded[key];
  }

  async function saveSettings(r: Repo): Promise<void> {
    const d = drafts[rowKey(r)];
    let tone: string | null;
    if (d.toneOption === "inherit") {
      tone = null;
    } else if (d.toneOption === "custom") {
      tone = d.customTone.trim() ? d.customTone : null;
    } else {
      tone = d.toneOption;
    }
    try {
      await putRepoSettings(r.platform, r.owner, r.repo, {
        tone,
        instructions: d.instructions.trim() ? d.instructions : null,
        model: d.model.trim() ? d.model : null,
        temperature:
          d.temperature === "" ? null : parseFloat(d.temperature) || null,
        trigger: d.trigger,
      });
      await refresh();
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error = e instanceof Error ? e.message : "Failed to save settings";
    }
  }

  async function regenerate(r: Repo): Promise<void> {
    const key = rowKey(r);
    const toTag = window.prompt(
      `Release tag to generate notes for (${r.owner}/${r.repo}):`,
    );
    notesOut[key] = null;
    genError[key] = null;
    if (!toTag) return;
    try {
      const result = await generate(r.platform, r.owner, r.repo, {
        to_tag: toTag,
        force: force[key] ?? false,
      });
      notesOut[key] = result.notes;
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      genError[key] = e instanceof Error ? e.message : "Generate failed";
    }
  }
</script>

<div>
  <div class="mb-8 flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="font-display text-3xl text-white">REPOSITORIES</h1>
      <p class="mt-1 text-base text-ink-2">
        Manage connected release-note sources.
      </p>
    </div>
    <div class="flex items-center gap-3">
      {#if !loading}
        <button
          onclick={refresh}
          class="rounded bg-control px-4 py-2 text-sm font-medium text-ink hover:bg-control-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
        >
          Refresh
        </button>
      {/if}
      <a
        href="/repos/add"
        class="inline-flex items-center gap-2 rounded bg-mark bg-gradient-to-r from-cherry via-ember to-heat px-4 py-2 text-sm font-bold text-page hover:brightness-110 focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
      >
        Add repositories
      </a>
    </div>
  </div>

  {#if error}
    <p class="mb-4 text-sm text-alert">{error}</p>
  {/if}

  {#if loading}
    <p class="text-sm text-ink-3">Loading…</p>
  {:else if repos.length === 0}
    <div class="rounded border border-line bg-surface-2-warm p-8 text-center">
      <p class="text-base text-ink-2">
        No repositories have been added yet.
      </p>
      <a
        href="/repos/add"
        class="mt-5 inline-flex items-center gap-2 rounded bg-mark bg-gradient-to-r from-cherry via-ember to-heat px-4 py-2 text-sm font-bold text-page hover:brightness-110 focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
      >
        Add your first repository
      </a>
    </div>
  {:else}
    <div class="overflow-x-auto rounded border border-line">
      <table class="w-full text-left text-sm">
        <thead class="bg-surface-1 text-ink-2">
          <tr>
            <th class="px-4 py-3 font-medium">Platform</th>
            <th class="px-4 py-3 font-medium">Owner/Repo</th>
            <th class="px-4 py-3 font-medium">Enabled</th>
            <th class="px-4 py-3 font-medium">Tone / Model</th>
            <th class="px-4 py-3 font-medium">Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each repos as r (rowKey(r))}
            {@const key = rowKey(r)}
            <tr class="border-t border-line border-b border-transparent transition-colors hover:bg-row-hover hover:border-t-row-hover-line hover:border-b-row-hover-line">
              <td class="px-4 py-3">
                <span
                  class="inline-flex items-center rounded bg-surface-2 px-2 py-0.5 text-xs font-medium capitalize text-ink-2 {r.platform === 'github' ? 'bg-surface-1-warm' : ''}"
                >
                  {#if r.platform === "github"}
                    <svg class="mr-1.5 h-3 w-3 text-ink-3" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.54 1.23.77.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/></svg>
                  {:else}
                    <svg class="mr-1.5 h-3 w-3 text-ink-3" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M8 1a7 7 0 0 0 0 14h1.5a.5.5 0 0 0 0-1H8a6 6 0 1 1 0-12h1.5a.5.5 0 0 0 0-1H8Zm.5 3a.5.5 0 0 0 0 1h5v2h-5a.5.5 0 0 0 0 1h5v2h-5a.5.5 0 0 0 0 1h5.5a.5.5 0 0 0 .5-.5v-6a.5.5 0 0 0-.5-.5H8.5Z"/></svg>
                  {/if}
                  {r.platform}
                </span>
              </td>
              <td class="px-4 py-3 text-ink">{r.owner}/{r.repo}</td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  <span
                    class="inline-block h-2 w-2 rounded-sm {r.enabled ? 'bg-ok' : 'bg-line-strong'}"
                    aria-hidden="true"
                  ></span>
                  <input
                    type="checkbox"
                    checked={r.enabled}
                    onchange={() => toggleEnabled(r)}
                    class="h-4 w-4 accent-mark focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
                  />
                </div>
              </td>
              <td class="px-4 py-3">
                <div class="flex flex-wrap gap-1.5">
                  <span class="rounded bg-surface-2 px-2 py-0.5 text-xs text-ink-2">
                    {r.effective.tone ?? "neutral"}
                  </span>
                  <span class="rounded bg-surface-2 px-2 py-0.5 text-xs text-ink-2">
                    {r.effective.model ?? "inherit"}
                  </span>
                </div>
              </td>
              <td class="px-4 py-3">
                <button
                  onclick={() => openSettings(r)}
                  class="rounded bg-control px-4 py-2 text-sm font-medium text-ink hover:bg-control-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
                >
                  Settings
                </button>
              </td>
            </tr>
            {#if expanded[key]}
              {@const d = drafts[key]}
              <tr class="border-t border-line bg-surface-1/60">
                <td colspan="5" class="px-6 py-4">
                  <div class="grid gap-4 sm:grid-cols-2">
                    <label class="flex flex-col gap-1">
                      <span class="text-xs text-ink-3">Tone</span>
                      <select
                        bind:value={d.toneOption}
                        class="rounded border border-line-strong bg-page px-3 py-2 text-sm text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
                      >
                        <option value="inherit">Inherit</option>
                        {#each PRESET_OPTIONS as p (p)}
                          <option value={p}>{p}</option>
                        {/each}
                      </select>
                    </label>
                    {#if d.toneOption === "custom"}
                      <label class="flex flex-col gap-1">
                        <span class="text-xs text-ink-3">Custom tone</span>
                        <input
                          bind:value={d.customTone}
                          placeholder="Freeform persona"
                          class="rounded border border-line-strong bg-page px-3 py-2 text-sm text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
                        />
                      </label>
                    {/if}
                    <label class="flex flex-col gap-1 sm:col-span-2">
                      <span class="text-xs text-ink-3">Instructions</span>
                      <textarea
                        bind:value={d.instructions}
                        rows="3"
                        class="rounded border border-line-strong bg-page px-3 py-2 text-sm text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
                      ></textarea>
                    </label>
                    <label class="flex flex-col gap-1">
                      <span class="text-xs text-ink-3">Model (blank = inherit)</span>
                      <input
                        bind:value={d.model}
                        class="rounded border border-line-strong bg-page px-3 py-2 text-sm text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
                      />
                    </label>
                    <label class="flex flex-col gap-1">
                      <span class="text-xs text-ink-3">Temperature (blank = inherit)</span>
                      <input
                        type="number"
                        step="0.1"
                        min="0"
                        max="2"
                        bind:value={d.temperature}
                        class="rounded border border-line-strong bg-page px-3 py-2 text-sm text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
                      />
                    </label>
                    <label class="flex flex-col gap-1">
                      <span class="text-xs text-ink-3">Trigger</span>
                      <select
                        bind:value={d.trigger}
                        class="rounded border border-line-strong bg-page px-3 py-2 text-sm text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
                      >
                        <option value="auto">auto</option>
                        <option value="manual">manual</option>
                      </select>
                    </label>
                  </div>

                  <p class="mt-3 text-xs text-ink-3">
                    Effective: tone
                    <span class="text-ink">{r.effective.tone ?? "neutral"}</span>
                    · model
                    <span class="text-ink">{r.effective.model ?? "inherit"}</span>
                    · temperature
                    <span class="text-ink">{r.effective.temperature ?? "inherit"}</span>
                  </p>

                  <div class="mt-4 flex flex-wrap items-center gap-3">
                    <button
                      onclick={() => saveSettings(r)}
                      class="rounded bg-mark bg-gradient-to-r from-cherry via-ember to-heat px-4 py-2 text-sm font-bold text-page hover:brightness-110 focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
                    >
                      Save
                    </button>
                    <label class="flex items-center gap-2 text-sm text-ink-2">
                      <input
                        type="checkbox"
                        checked={force[key] ?? false}
                        onchange={(e) => {
                          force[key] = e.currentTarget.checked;
                        }}
                        class="accent-mark"
                      />
                      Force regenerate
                    </label>
                    <button
                      onclick={() => regenerate(r)}
                      class="rounded bg-surface-2 px-4 py-2 text-sm font-medium text-ink hover:bg-control focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring"
                    >
                      Regenerate
                    </button>
                  </div>

                  {#if genError[key]}
                    <p class="mt-3 text-sm text-alert">{genError[key]}</p>
                  {/if}
                  {#if notesOut[key]}
                    <pre class="mt-3 max-h-64 overflow-auto rounded border border-line bg-page p-3 font-mono text-xs text-ink-2">{notesOut[key]}</pre>
                  {/if}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
