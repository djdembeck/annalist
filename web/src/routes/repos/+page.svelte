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
  let loadState = $state<"idle" | "loading" | "success" | "error">("idle");
  let loadError = $state("");
  let pending = $state<
    Record<string, { saving?: boolean; toggling?: boolean; generating?: boolean }>
  >({});
  let saveMsg = $state<Record<string, string | null>>({});
  let saveErr = $state<Record<string, string | null>>({});
  let toggleErr = $state<Record<string, string | null>>({});
  let openPanel = $state<
    Record<string, "settings" | "regenerate" | undefined>
  >({});
  let drafts = $state<Record<string, Draft>>({});
  let force = $state<Record<string, boolean>>({});
  let notesOut = $state<Record<string, string | null>>({});
  let genError = $state<Record<string, string | null>>({});
  let regenerateTag = $state<Record<string, string>>({});
  let regenTagError = $state<Record<string, string | null>>({});

  function rowKey(r: Repo): string {
    return `${r.platform}/${r.owner}/${r.repo}`;
  }

  async function refresh(): Promise<void> {
    loadError = "";
    loadState = "loading";
    try {
      repos = await getRepos();
      loadState = "success";
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      loadState = "error";
      loadError = e instanceof Error ? e.message : "Failed to load repos";
    }
  }

  onMount(refresh);

  async function toggleEnabled(r: Repo): Promise<void> {
    const key = rowKey(r);
    const nextEnabled = !r.enabled;
    toggleErr[key] = null;
    pending[key] = { ...(pending[key] ?? {}), toggling: true };
    try {
      await putRepoSettings(r.platform, r.owner, r.repo, {
        enabled: nextEnabled,
      });
      await refresh();
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      toggleErr[key] = e instanceof Error ? e.message : "Failed to update";
    } finally {
      pending[key] = { ...(pending[key] ?? {}), toggling: false };
    }
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
    openPanel[key] = openPanel[key] === "settings" ? undefined : "settings";
  }

  function openRegenerate(r: Repo): void {
    const key = rowKey(r);
    regenerateTag[key] ??= "";
    openPanel[key] = openPanel[key] === "regenerate" ? undefined : "regenerate";
  }

  async function saveSettings(r: Repo): Promise<void> {
    const key = rowKey(r);
    const d = drafts[key];
    saveErr[key] = null;
    saveMsg[key] = null;
    pending[key] = { ...(pending[key] ?? {}), saving: true };
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
      saveMsg[key] = "Saved";
      window.setTimeout(() => {
        saveMsg[key] = null;
      }, 2500);
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      saveErr[key] = e instanceof Error ? e.message : "Failed to save settings";
    } finally {
      pending[key] = { ...(pending[key] ?? {}), saving: false };
    }
  }

  async function regenerate(r: Repo): Promise<void> {
    const key = rowKey(r);
    const toTag = (regenerateTag[key] ?? "").trim();
    notesOut[key] = null;
    genError[key] = null;
    regenTagError[key] = null;
    if (!toTag) {
      regenTagError[key] = "Enter a release tag";
      return;
    }
    pending[key] = { ...pending[key], generating: true };
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
    } finally {
      pending[key] = { ...pending[key], generating: false };
    }
  }
</script>

<div>
  <div class="mb-8 flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="font-display text-3xl tracking-tight text-white">REPOSITORIES</h1>
      <p class="mt-1 text-base text-ink-2">
        Manage connected release-note sources.
      </p>
    </div>
    <div class="flex items-center gap-3">
      {#if loadState === "success" || loadState === "error"}
        <button
          onclick={refresh}
          class="btn-base btn-primary"
        >
          Refresh
        </button>
      {/if}
      <a href="/repos/add" class="btn-base btn-mark">
        Add repositories
      </a>
    </div>
  </div>

  {#if loadState === "loading" || loadState === "idle"}
    <div class="card" aria-busy="true" aria-label="Loading repositories">
      <div class="space-y-4">
        <div class="skeleton h-8 w-full"></div>
        <div class="skeleton h-10 w-full"></div>
        <div class="skeleton h-10 w-full"></div>
        <div class="skeleton h-10 w-full"></div>
      </div>
    </div>
  {:else if loadState === "error"}
    <div class="card border-alert bg-alert/10">
      <p class="text-sm font-medium text-alert">
        Couldn't load repositories.
      </p>
      <p class="mt-1 text-sm text-ink-2">{loadError}</p>
      <button onclick={() => refresh()} class="btn-base btn-primary mt-4">
        Retry
      </button>
    </div>
  {:else if repos.length === 0}
    <div class="empty-state">
      <p class="text-base text-ink-2">
        No repositories have been added yet.
      </p>
      <p class="mt-1 text-sm text-ink-3">
        Connect a repo to start generating release notes.
      </p>
      <a href="/repos/add" class="btn-base btn-mark mt-5">
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
            <tr class="border-t border-line transition-colors hover:bg-row-hover">
              <td class="px-4 py-3">
                <span class="chip capitalize">
                  {#if r.platform === "github"}
                    <svg class="h-3 w-3 text-ink-3" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.54 1.23.77.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/></svg>
                  {:else}
                    <svg class="h-3 w-3 text-ink-3" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path d="M8 1a7 7 0 0 0 0 14h1.5a.5.5 0 0 0 0-1H8a6 6 0 1 1 0-12h1.5a.5.5 0 0 0 0-1H8Zm.5 3a.5.5 0 0 0 0 1h5v2h-5a.5.5 0 0 0 0 1h5v2h-5a.5.5 0 0 0 0 1h5.5a.5.5 0 0 0 .5-.5v-6a.5.5 0 0 0-.5-.5H8.5Z"/></svg>
                  {/if}
                  {r.platform}
                </span>
              </td>
              <td class="px-4 py-3 text-ink">{r.owner}/{r.repo}</td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  <span
                    class="status-dot {r.enabled ? 'bg-ok' : 'bg-line-strong'}"
                    aria-hidden="true"
                    title={r.enabled ? "Enabled" : "Disabled"}
                  ></span>
                  <input
                    type="checkbox"
                    aria-label="Enable {r.owner}/{r.repo}"
                    checked={r.enabled}
                    disabled={pending[key]?.toggling}
                    onchange={() => toggleEnabled(r)}
                    class="h-4 w-4 accent-mark focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring disabled:cursor-not-allowed disabled:opacity-50"
                  />
                </div>
                {#if toggleErr[key]}
                  <p class="mt-1 text-xs text-alert">{toggleErr[key]}</p>
                {/if}
              </td>
              <td class="px-4 py-3">
                <div class="flex flex-wrap gap-1.5">
                  <span class="chip">
                    {r.effective.tone ?? "neutral"}
                  </span>
                  <span class="chip">
                    {r.effective.model ?? "inherit"}
                  </span>
                </div>
              </td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  <button
                    onclick={() => openSettings(r)}
                    aria-expanded={openPanel[key] === "settings"}
                    aria-controls="repo-settings-{key}"
                    class="btn-base btn-primary"
                  >
                    Settings
                  </button>
                  <button
                    onclick={() => openRegenerate(r)}
                    aria-expanded={openPanel[key] === "regenerate"}
                    aria-controls="repo-regen-{key}"
                    class="btn-base btn-secondary"
                  >
                    Regenerate
                  </button>
                </div>
              </td>
            </tr>
            {#if openPanel[key] === "settings"}
              {@const d = drafts[key]}
              <tr class="border-t border-line bg-surface-1/60">
                <td id="repo-settings-{key}" colspan="5" class="px-6 py-4">
                  <p class="mb-4 text-xs font-medium uppercase tracking-wide text-ink-3">
                    Settings for {r.owner}/{r.repo}
                  </p>
                  <div class="grid gap-4 md:grid-cols-2">
                    <label class="flex flex-col gap-1">
                      <span class="text-xs text-ink-3">Tone</span>
                      <select bind:value={d.toneOption} class="field">
                        <option value="inherit">Inherit</option>
                        {#each PRESET_OPTIONS as p (p)}
                          <option value={p}>{p}</option>
                        {/each}
                      </select>
                      <p class="text-xs text-ink-3">
                        Use one of the presets or set a custom persona.
                      </p>
                    </label>
                    {#if d.toneOption === "custom"}
                      <label class="flex flex-col gap-1">
                        <span class="text-xs text-ink-3">Custom tone</span>
                        <input
                          bind:value={d.customTone}
                          placeholder="Freeform persona"
                          class="field"
                        />
                      </label>
                    {/if}
                    <label class="flex flex-col gap-1 md:col-span-2">
                      <span class="text-xs text-ink-3">Instructions</span>
                      <textarea bind:value={d.instructions} rows="3" class="field"></textarea>
                      <p class="text-xs text-ink-3">
                        Extra guidance the writer follows for this repo.
                      </p>
                    </label>
                    <label class="flex flex-col gap-1">
                      <span class="text-xs text-ink-3">Model (blank = inherit)</span>
                      <input bind:value={d.model} class="field" />
                      <p class="text-xs text-ink-3">
                        Override the global model for this repo.
                      </p>
                    </label>
                    <label class="flex flex-col gap-1">
                      <span class="text-xs text-ink-3">Temperature (blank = inherit)</span>
                      <input
                        type="number"
                        step="0.1"
                        min="0"
                        max="2"
                        bind:value={d.temperature}
                        class="field"
                      />
                      <p class="text-xs text-ink-3">
                        0 = deterministic, 2 = very creative.
                      </p>
                    </label>
                    <label class="flex flex-col gap-1">
                      <span class="text-xs text-ink-3">Trigger</span>
                      <select bind:value={d.trigger} class="field">
                        <option value="auto">auto</option>
                        <option value="manual">manual</option>
                      </select>
                      <p class="text-xs text-ink-3">
                        Auto runs on release webhooks; manual disables webhooks.
                      </p>
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
                      disabled={pending[key]?.saving}
                      class="btn-base btn-mark"
                    >
                      {pending[key]?.saving ? "Saving…" : "Save"}
                    </button>
                    <span class="text-xs text-ink-3">
                      Save settings before regenerating — the writer uses the
                      saved tone.
                    </span>
                  </div>

                  <div aria-live="polite" class="mt-3">
                    {#if saveMsg[key]}
                      <p class="text-sm text-ok">{saveMsg[key]}</p>
                    {/if}
                    {#if saveErr[key]}
                      <p class="text-sm text-alert">{saveErr[key]}</p>
                    {/if}
                  </div>
                </td>
              </tr>
            {/if}

            {#if openPanel[key] === "regenerate"}
              <tr class="border-t border-line bg-surface-1/60">
                <td id="repo-regen-{key}" colspan="5" class="px-6 py-4">
                  <p class="mb-3 text-xs font-medium uppercase tracking-wide text-ink-3">
                    Regenerate for {r.owner}/{r.repo}
                  </p>
                  <p class="mb-4 max-w-xl text-sm text-ink-2">
                    Runs the writer for a release tag and writes the note back
                    into the published release body. The mode you pick decides
                    how much of the current release it may replace:
                  </p>

                  <div
                    class="grid gap-3 sm:grid-cols-2"
                    aria-label="Regenerate mode for {r.owner}/{r.repo}"
                  >
                    <button
                      type="button"
                      aria-pressed={!force[key]}
                      onclick={() => (force[key] = false)}
                      class="flex flex-col gap-1.5 rounded border px-4 py-3 text-left transition-colors {!force[key]
                        ? 'border-mark/70 bg-control'
                        : 'border-line bg-surface-1 hover:border-line-strong'}"
                    >
                      <span
                        class="flex items-center gap-2 text-sm font-medium {!force[key]
                          ? 'text-ink'
                          : 'text-ink-2'}"
                      >
                        <span
                          aria-hidden="true"
                          class="h-2 w-2 rounded-full {!force[key]
                            ? 'bg-ember'
                            : 'bg-line-strong'}"
                        ></span>
                        Regenerate
                      </span>
                      <span class="text-xs leading-relaxed text-ink-3">
                        Reuses the note already on file for this tag and never
                        overwrites a release body you edited by hand.
                      </span>
                    </button>
                    <button
                      type="button"
                      aria-pressed={force[key] ?? false}
                      onclick={() => (force[key] = true)}
                      class="flex flex-col gap-1.5 rounded border px-4 py-3 text-left transition-colors {force[key]
                        ? 'border-alert/70 bg-control'
                        : 'border-line bg-surface-1 hover:border-line-strong'}"
                    >
                      <span
                        class="flex items-center gap-2 text-sm font-medium {force[key]
                          ? 'text-ink'
                          : 'text-ink-2'}"
                      >
                        <span
                          aria-hidden="true"
                          class="h-2 w-2 rounded-full {force[key]
                            ? 'bg-alert'
                            : 'bg-line-strong'}"
                        ></span>
                        Force regenerate
                      </span>
                      <span class="text-xs leading-relaxed text-ink-3">
                        Throws out the existing note and runs the writer again —
                        overwriting even a hand-edited release body.
                      </span>
                    </button>
                  </div>

                  <div class="mt-4 flex flex-wrap items-end gap-3">
                    <label class="flex flex-col gap-1">
                      <span class="text-xs text-ink-3">Release tag</span>
                      <input
                        bind:value={regenerateTag[key]}
                        placeholder="v1.0.0"
                        class="field"
                      />
                      {#if regenTagError[key]}
                        <p class="text-xs text-alert">{regenTagError[key]}</p>
                      {/if}
                    </label>
                    <button
                      onclick={() => regenerate(r)}
                      disabled={pending[key]?.generating}
                      class="btn-base btn-mark"
                    >
                      {pending[key]?.generating
                        ? "Generating…"
                        : force[key]
                          ? "Force regenerate"
                          : "Regenerate"}
                    </button>
                  </div>

                  <div aria-live="polite" class="mt-3">
                    {#if genError[key]}
                      <p class="text-sm text-alert">{genError[key]}</p>
                    {/if}
                    {#if notesOut[key]}
                      <div>
                        <p class="text-xs font-medium uppercase tracking-wide text-ink-3">
                          Generated notes
                        </p>
                        <pre
                          class="mt-2 max-h-64 overflow-auto rounded border border-line bg-page p-3 font-mono text-xs text-ink-2">{notesOut[key]}</pre>
                      </div>
                    {/if}
                  </div>
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
