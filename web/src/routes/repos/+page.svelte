<script lang="ts">
  import { onMount } from "svelte";
  import {
    repoKey,
    parseMaxTokens,
    parseTemperature,
    resolveTone,
    SAVE_MSG_TIMEOUT,
  } from "$lib/repoUtils";
  import {
    DEFAULT_COMMIT_TYPES,
    formatCommitTypes,
    getCommitTypeSelection,
    isKeepAll,
  } from "$lib/commitTypes";
  import {
    getRepos,
    putRepoSettings,
    generate,
    getInRepoInstructions,
    getModels,
    type Repo,
  } from "$lib/api";
  import CommitTypeSelector from "$lib/components/CommitTypeSelector.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import ErrorBanner from "$lib/components/ErrorBanner.svelte";
  import SectionHead from "$lib/components/SectionHead.svelte";
  import {
    formatError,
    handleAuthError,
  } from "$lib/composables/useAuthErrorHandler";

  const PRESETS = ["chronicler", "engineer", "launch"];
  const PRESET_OPTIONS = [...PRESETS, "custom"];

  type Draft = {
    toneOption: string;
    customTone: string;
    instructions: string;
    model: string;
    mode: string;
    temperature: string;
    trigger: string;
    maxTokens: string | null;
    thinkingLevel: string;
    inheritCommitTypes: boolean;
    selectedCommitTypes: string[];
    customCommitTypes: string;
  };

  let repos = $state<Repo[]>([]);
  let loadState = $state<"idle" | "loading" | "success" | "error">("idle");
  let loadError = $state("");
  let models = $state<string[]>([]);
  let modelsError = $state("");
  let pending = $state<
    Record<
      string,
      { saving?: boolean; toggling?: boolean; generating?: boolean }
    >
  >({});
  let saveMsg = $state<Record<string, string | null>>({});
  let saveErr = $state<Record<string, string | null>>({});
  let toggleErr = $state<Record<string, string | null>>({});
  let openPanel = $state<Record<string, "settings" | "generate" | undefined>>(
    {},
  );
  let drafts = $state<Record<string, Draft>>({});
  let force = $state<Record<string, boolean>>({});
  let publish = $state<Record<string, boolean>>({});
  let notesOut = $state<Record<string, string | null>>({});
  let genError = $state<Record<string, string | null>>({});
  let generateTag = $state<Record<string, string>>({});
  let genTagError = $state<Record<string, string | null>>({});
  let overwriteConfirm = $state<Record<string, boolean>>({});
  let temperatureError = $state<Record<string, string | null>>({});
  let maxTokensError = $state<Record<string, string | null>>({});

  let inventoryLede = $derived(
    loadState === "success" || loadState === "error"
      ? `${repos.length} ${repos.length === 1 ? "repository" : "repositories"} connected, ${repos.filter((r) => r.enabled).length} enabled.`
      : undefined,
  );

  onMount(refresh);

  async function toggleEnabled(r: Repo): Promise<void> {
    const key = repoKey(r);
    const nextEnabled = !r.enabled;
    toggleErr[key] = null;
    pending[key] = { ...(pending[key] ?? {}), toggling: true };
    try {
      await putRepoSettings(r.platform, r.owner, r.repo, {
        enabled: nextEnabled,
      });
      await refresh();
    } catch (e) {
      if (handleAuthError(e)) return;
      toggleErr[key] = formatError(
        e,
        "Could not update — check your connection and try again.",
      );
    } finally {
      pending[key] = { ...(pending[key] ?? {}), toggling: false };
    }
  }

  // Cached in-repo instructions, populated after page load in parallel.
  let inRepoInstructions = $state<Record<string, string | null>>({});
  let inRepoPending = $state<Record<string, boolean>>({});

  function loadInRepoInstructions(repo: Repo): void {
    const key = repoKey(repo);
    if (inRepoInstructions[key] !== undefined) return; // Already loaded.
    inRepoPending[key] = true;
    getInRepoInstructions(repo.platform, repo.owner, repo.repo)
      .then((content) => {
        inRepoInstructions[key] = content;
        inRepoPending[key] = false;
        // Update settings drafts if panel is open.
        if (openPanel[key] === "settings" && content && drafts[key]) {
          drafts[key].instructions = content;
        }
      })
      .catch(() => {
        inRepoInstructions[key] = null;
        inRepoPending[key] = false;
      });
  }

  // Model list for the per-repo dropdowns. A failure must NOT fail or block
  // the repo list; it only surfaces a retry banner and leaves the previous
  // list in place.
  async function loadModels(): Promise<void> {
    modelsError = "";
    try {
      models = await getModels();
    } catch (e) {
      if (handleAuthError(e)) return;
      modelsError = formatError(
        e,
        "Could not load the model list — the repositories below fall back to the previous list.",
      );
    }
  }

  async function refresh(): Promise<void> {
    loadError = "";
    loadState = "loading";
    try {
      repos = await getRepos();
      loadState = "success";
      // Kick off in-repo instruction fetches in parallel (non-blocking).
      for (const r of repos) {
        loadInRepoInstructions(r);
      }
      // Refresh the model list too (non-blocking; a failure only surfaces a
      // retry banner, never the repo list).
      void loadModels();
    } catch (e) {
      if (handleAuthError(e)) return;
      loadState = "error";
      loadError = formatError(
        e,
        "Could not load repositories — check your connection and try again.",
      );
    }
  }

  function openSettings(r: Repo): void {
    const key = repoKey(r);
    const tone = r.tone ?? "";
    const isPreset = PRESETS.includes(tone);
    const commitTypeSelection = getCommitTypeSelection(r.commit_types);
    drafts[key] = {
      toneOption: !tone ? "inherit" : isPreset ? tone : "custom",
      customTone: isPreset ? "" : tone,
      instructions: inRepoInstructions[key] ?? r.instructions ?? "",
      model: r.model ?? "",
      mode:
        r.mode === "deep" ? "deep" : r.mode === "lite" ? "lite" : "inherit",
      temperature: r.temperature === null ? "" : String(r.temperature),
      trigger: r.trigger ?? "auto",
      maxTokens:
        r.max_tokens === 0 || r.max_tokens == null ? "" : String(r.max_tokens),
      thinkingLevel: r.thinking_level === null || r.thinking_level === "" ? "inherit" : r.thinking_level,
      inheritCommitTypes: !r.commit_types,
      selectedCommitTypes: commitTypeSelection.selected,
      customCommitTypes: commitTypeSelection.custom.join(", "),
    };
    openPanel[key] = openPanel[key] === "settings" ? undefined : "settings";
    // Ensure in-repo instructions are loaded.
    loadInRepoInstructions(r);
  }

  // Computed: get the effective instructions for display,
  // preferring in-repo if loaded, else resolved.
  function getEffectiveInstructions(r: Repo): string {
    return (
      inRepoInstructions[repoKey(r)] ??
      r.effective.instructions ??
      "neutral (default)"
    );
  }

  function openGenerate(r: Repo): void {
    const key = repoKey(r);
    generateTag[key] ??= "";
    overwriteConfirm[key] = false;
    force[key] = false;
    publish[key] = false;
    openPanel[key] = openPanel[key] === "generate" ? undefined : "generate";
  }

  async function saveSettings(r: Repo): Promise<void> {
    const key = repoKey(r);
    const d = drafts[key];
    saveErr[key] = null;
    saveMsg[key] = null;
    temperatureError[key] = null;
    maxTokensError[key] = null;
    const tempResult = parseTemperature(d.temperature);
    if (tempResult.error) {
      temperatureError[key] = tempResult.error;
      return;
    }
    const tokensResult = parseMaxTokens(d.maxTokens);
    if (tokensResult.error) {
      maxTokensError[key] = tokensResult.error;
      return;
    }
    pending[key] = { ...(pending[key] ?? {}), saving: true };
    const tone = resolveTone(d.toneOption, d.customTone);
    try {
      await putRepoSettings(r.platform, r.owner, r.repo, {
        tone,
        instructions: d.instructions.trim() ? d.instructions : null,
        model: d.model.trim() ? d.model : null,
        mode: d.mode === "inherit" ? null : d.mode,
        temperature: tempResult.value,
        max_tokens: tokensResult.value,
        // "inherit" sends null (stored empty, resolves through the chain);
      // "off" sends the literal "off" so it suppresses a global/config
      // level rather than inheriting.
        thinking_level: d.thinkingLevel === "inherit" ? null : d.thinkingLevel,
        trigger: d.trigger,
        commit_types: d.inheritCommitTypes
          ? null
          : formatCommitTypes(d.selectedCommitTypes, d.customCommitTypes),
      });
      await refresh();
      saveMsg[key] = "Saved";
      temperatureError[key] = null;
      maxTokensError[key] = null;
      window.setTimeout(() => {
        saveMsg[key] = null;
      }, SAVE_MSG_TIMEOUT);
    } catch (e) {
      if (handleAuthError(e)) return;
      saveErr[key] = formatError(
        e,
        "Could not save — check your connection and try again.",
      );
    } finally {
      pending[key] = { ...(pending[key] ?? {}), saving: false };
    }
  }

  async function runGenerate(r: Repo): Promise<void> {
    const key = repoKey(r);
    const toTag = (generateTag[key] ?? "").trim();
    notesOut[key] = null;
    genError[key] = null;
    genTagError[key] = null;
    if (!toTag) {
      genTagError[key] = "Enter a release tag";
      return;
    }
    if ((force[key] ?? false) && !overwriteConfirm[key]) {
      overwriteConfirm[key] = true;
      return;
    }
    pending[key] = { ...pending[key], generating: true };
    try {
      const result = await generate(r.platform, r.owner, r.repo, {
        to_tag: toTag,
        force: force[key] ?? false,
        publish: publish[key] ?? false,
      });
      notesOut[key] = result.notes;
      overwriteConfirm[key] = false;
    } catch (e) {
      if (handleAuthError(e)) return;
      genError[key] =
        `Generation failed for ${toTag}. ${formatError(e, "Check that the tag exists and the LLM endpoint is reachable.")}`;
    } finally {
      pending[key] = { ...pending[key], generating: false };
    }
  }
</script>

<svelte:head>
  <title>Repositories · Annalist</title>
  <meta
    name="description"
    content="Manage connected repositories and release-note generation."
  />
</svelte:head>

<div class="trace-wall">
  <SectionHead
    label="Operations / connected sources"
    title="Repository trace inventory"
    lede={inventoryLede}
  >
    {#snippet actions()}
      {#if loadState === "loading"}
        <button
          disabled
          class="btn btn-secondary"
          aria-label="Refreshing repository inventory">Refreshing…</button
        >
      {:else if loadState === "success" || loadState === "error"}
        <button
          onclick={refresh}
          class="btn btn-secondary"
          aria-label="Refresh repository inventory">Refresh</button
        >
      {/if}
      <a href="/repos/add" class="btn btn-primary">Add repositories</a>
    {/snippet}
  </SectionHead>

  {#if loadState === "loading" || loadState === "idle"}
    <section class="panel" aria-busy="true" aria-label="Loading repositories">
      <p class="trace-label">Reading source inventory</p>
      <div class="skeleton mt-4 h-12 w-full"></div>
      <div class="skeleton mt-3 h-24 w-full"></div>
      <div class="skeleton mt-3 h-24 w-full"></div>
    </section>
  {:else if loadState === "error"}
    <ErrorBanner
      label="Source read failed"
      message="Couldn't load repositories."
      detail={loadError}
      actionLabel="Retry"
      onAction={refresh}
    />
  {:else if repos.length === 0}
    <EmptyState
      label="No connected sources"
      heading="The trace starts with a repository."
      description="Connect a GitHub or Forgejo repository to put release commits on the wall and shape the first note."
      actionLabel="Add your first repository"
      href="/repos/add"
    />
  {:else}
    {#if modelsError}
      <ErrorBanner
        label="Model list unavailable"
        message={modelsError}
        actionLabel="Retry"
        onAction={loadModels}
        variant="warning"
      />
    {/if}
    <section
      class="console-grid repo-inventory"
      aria-label="Connected repository inventory"
    >
      {#each repos as r (repoKey(r))}
        {@const key = repoKey(r)}
        <article class="panel repo-workpiece">
          <header class="repo-workpiece__header">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <span class="chip">{r.platform}</span>
                {#if inRepoInstructions[key] || r.instructions}
                  <span class="chip" title="Custom voice/prompt configured"
                    >Voice</span
                  >
                {/if}
                <span
                  class="status {r.enabled
                    ? 'status--healthy'
                    : 'status--quiet'}"
                  ><span
                    class="signal-dot {r.enabled ? '' : 'signal-dot--muted'}"
                    aria-hidden="true"
                  ></span>{r.enabled ? "Enabled" : "Disabled"}</span
                >
              </div>
              <h3 class="repo-workpiece__name">{r.owner}/{r.repo}</h3>
            </div>
            <label class="repo-switch"
              ><span class="trace-label">In trace</span><input
                type="checkbox"
                aria-label={"Enable " + r.owner + "/" + r.repo}
                checked={r.enabled}
                disabled={pending[key]?.toggling}
                onchange={() => toggleEnabled(r)}
              /></label
            >
          </header>

          <div class="repo-workpiece__details">
            <div>
              <p class="trace-label">Effective tone</p>
              <p class="mt-1 text-sm text-ink">
                {inRepoInstructions[key]
                  ? "custom"
                  : (r.effective.tone ?? "neutral")}
              </p>
            </div>
            <div>
              <p class="trace-label">Model</p>
              <p class="mt-1 break-all text-sm text-ink">
                {r.effective.model ?? "inherit"}
              </p>
            </div>
            <div>
              <p class="trace-label">Effective voice</p>
              <p
                class="mt-1 text-sm text-ink break-all max-h-8 overflow-hidden"
                title={getEffectiveInstructions(r) !== "neutral (default)"
                  ? getEffectiveInstructions(r)
                  : ""}
              >
                {inRepoPending[key] ? "loading…" : getEffectiveInstructions(r)}
              </p>
            </div>
            <div>
              <p class="trace-label">Trigger</p>
              <p class="mt-1 text-sm text-ink">{r.trigger ?? "auto"}</p>
            </div>
          </div>
          {#if toggleErr[key]}<p class="mt-3 text-xs text-alert" role="alert">
              {toggleErr[key]}
            </p>{/if}

          <div class="repo-workpiece__actions">
            <button
              onclick={() => openGenerate(r)}
              aria-expanded={openPanel[key] === "generate"}
              aria-controls={openPanel[key] === "generate"
                ? `repo-generate-${key}`
                : null}
              class="btn btn-primary"
              >{openPanel[key] === "generate"
                ? "Close generation"
                : "Generate note"}</button
            >
            <button
              onclick={() => openSettings(r)}
              aria-expanded={openPanel[key] === "settings"}
              aria-controls={openPanel[key] === "settings"
                ? `repo-settings-${key}`
                : null}
              class="btn btn-secondary"
              >{openPanel[key] === "settings"
                ? "Close settings"
                : "Settings"}</button
            >
          </div>

          {#if openPanel[key] === "settings"}
            {@const d = drafts[key]}
            <section
              id="repo-settings-{key}"
              class="trace-panel"
              aria-label="Settings for {r.owner}/{r.repo}"
            >
              <p class="trace-label">Tone contract / {r.owner}/{r.repo}</p>
              <div class="grid gap-4 md:grid-cols-2">
                <label class="field-group"
                  ><span class="field-group__label">Tone</span><select
                    bind:value={d.toneOption}
                    class="field"
                    ><option value="inherit">Inherit</option
                    >{#each PRESET_OPTIONS as p (p)}<option value={p}
                        >{p}</option
                      >{/each}</select
                  ><span class="field-group__hint"
                    >Use a preset or choose a custom persona.</span
                  ></label
                >
                {#if d.toneOption === "custom"}<label class="field-group"
                    ><span class="field-group__label">Custom tone</span><input
                      bind:value={d.customTone}
                      placeholder="Freeform persona"
                      class="field"
                    /></label
                  >{/if}
                <label class="field-group md:col-span-2"
                  ><span class="field-group__label">Instructions</span><textarea
                    bind:value={d.instructions}
                    rows="3"
                    class="field"></textarea><span class="field-group__hint"
                    >{#if inRepoPending[key] === true}Loading in-repo
                      instructions…
                    {:else}Extra guidance the writer follows for this
                      repository.{/if}</span
                  ></label
                >
                <label class="field-group"
                  ><span class="field-group__label"
                    >Model <span class="field-group__hint"
                      >(blank = inherit)</span
                    ></span
                  ><select bind:value={d.model} class="field"
                    ><option value="">Inherit (system default)</option
                    >{#each models as m (m)}<option value={m}>{m}</option
                      >{/each}{#if d.model && !models.includes(d.model)}<option
                        value={d.model}>{d.model} (not in /models list)</option
                      >{/if}</select
                  ><span class="field-group__hint"
                    >Empty inherits the global contract, then the server
                    default.</span
                  ></label
                >
                <label class="field-group"
                  ><span class="field-group__label"
                    >Temperature <span class="field-group__hint"
                      >(blank = inherit)</span
                    ></span
                  ><input
                    type="number"
                    step="0.1"
                    min="0"
                    max="2"
                    bind:value={d.temperature}
                    class="field"
                  /><span class="field-group__hint"
                    >0 = deterministic, 2 = very creative.</span
                  >{#if temperatureError[key]}<span
                      class="field-group__error"
                      role="alert">{temperatureError[key]}</span
                    >{/if}</label
                >
                <label class="field-group"
                  ><span class="field-group__label"
                    >Max output tokens <span class="field-group__hint"
                      >(blank = inherit)</span
                    ></span
                  ><input
                    type="number"
                    step="1"
                    min="1"
                    bind:value={d.maxTokens}
                    class="field"
                  /><span class="field-group__hint"
                    >Blank inherits the global value.</span
                  >{#if maxTokensError[key]}<span
                      class="field-group__error"
                      role="alert">{maxTokensError[key]}</span
                    >{/if}</label
                >
                <label class="field-group"
                  ><span class="field-group__label">Thinking level</span
                  ><select bind:value={d.thinkingLevel} class="field"
                    ><option value="inherit">Inherit (global)</option
                    ><option value="off">off — no extended thinking</option
                    ><option value="low">low</option
                    ><option value="medium">medium</option
                    ><option value="high">high</option
                  ></select><span class="field-group__hint"
                    >Sent to the model as reasoning_effort; off sends "none".</span
                  ></label
                >
                <label class="field-group"
                  ><span class="field-group__label">Trigger</span><select
                    bind:value={d.trigger}
                    class="field"
                    ><option value="auto">auto</option><option value="manual"
                      >manual</option
                    ></select
                  ><span class="field-group__hint"
                    >Auto runs on release webhooks; manual disables webhooks.</span
                  ></label
                >
                <label class="field-group"
                  ><span class="field-group__label">Mode</span><select
                    bind:value={d.mode}
                    class="field"
                    ><option value="inherit">Inherit (global)</option
                    ><option value="lite">lite — commit log only</option
                    ><option value="deep">deep — commit log + diff</option
                  ></select><span class="field-group__hint"
                    >deep also sends the code diff to the model for this
                    repository.</span
                  ></label
                >
                <fieldset class="field-group md:col-span-2">
                  <legend class="field-group__label">Commit types</legend>
                  <label class="check-control">
                    <input
                      class="check-input"
                      type="checkbox"
                      checked={d.inheritCommitTypes}
                      onchange={(event) => {
                        const inherit = event.currentTarget.checked;
                        d.inheritCommitTypes = inherit;
                        if (
                          !inherit &&
                          d.selectedCommitTypes.length === 0 &&
                          !d.customCommitTypes.trim()
                        ) {
                          d.selectedCommitTypes = [...DEFAULT_COMMIT_TYPES];
                        }
                      }}
                    />
                    <span>Inherit global selection</span>
                  </label>
                  <CommitTypeSelector
                    bind:selected={d.selectedCommitTypes}
                    bind:custom={d.customCommitTypes}
                    showOptions={!d.inheritCommitTypes}
                    hint={
                      d.inheritCommitTypes
                        ? "Uses the global selection. Turn off inheritance to choose repository-specific types."
                        : isKeepAll(
                            formatCommitTypes(
                              d.selectedCommitTypes,
                              d.customCommitTypes,
                            ),
                          )
                          ? "No filter — all commit types are included."
                          : "Selected types are included in notes."
                    }
                    ariaLabel="Repository commit types"
                  />
                </fieldset>
              </div>
              <p class="mt-4 text-xs text-ink-3">
                Effective: tone <span class="text-ink"
                  >{inRepoInstructions[key]
                    ? "custom"
                    : (r.effective.tone ?? "neutral")}</span
                >
                · model
                <span class="text-ink">{r.effective.model ?? "inherit"}</span>
                · temperature
                <span class="text-ink"
                  >{r.effective.temperature !== null &&
                  r.effective.temperature !== undefined
                    ? r.effective.temperature
                    : "inherit"}</span
                >
                · voice
                <span class="text-ink"
                  >{inRepoInstructions[key]
                    ? "custom (in-repo)"
                    : r.effective.instructions
                      ? "custom"
                      : "neutral (default)"}</span
                >
                · commit types
                <span class="text-ink"
                  >{r.effective.commit_types?.length
                    ? r.effective.commit_types.join(", ")
                    : "inherit"}</span
                >
                · mode <span class="text-ink">{r.effective.mode ?? "inherit"}</span>
                · max tokens
                <span class="text-ink">{r.effective.max_tokens}</span>
                · thinking
                <span class="text-ink"
                  >{r.effective.thinking_level || "off"}</span
                >
              </p>
              {#if inRepoInstructions[key] || r.effective.instructions}
                <details class="mt-4">
                  <summary class="trace-label cursor-pointer select-none"
                    >View effective voice/prompt</summary
                  >
                  <div
                    class="note-paper mt-2 text-sm whitespace-pre-wrap overflow-auto max-h-64 font-mono"
                  >
                    {#if inRepoInstructions[key]}
                      In-repo instructions
                      (.github/release-notes-instructions.md):\n\n{inRepoInstructions[
                        key
                      ]}
                    {:else}
                      {r.effective.instructions}
                    {/if}
                  </div>
                </details>
              {/if}
              <div class="mt-4 flex flex-wrap items-center gap-3">
                <button
                  onclick={() => saveSettings(r)}
                  disabled={pending[key]?.saving}
                  class="btn btn-primary"
                  >{pending[key]?.saving ? "Saving…" : "Save settings"}</button
                ><span class="text-xs text-ink-3"
                  >Save before generating; the writer uses the saved tone.</span
                >
              </div>
              <div aria-live="polite" class="mt-3">
                {#if saveMsg[key]}<p class="text-sm text-ok">
                    {saveMsg[key]}
                  </p>{/if}{#if saveErr[key]}<p
                    class="text-sm text-alert"
                    role="alert"
                  >
                    {saveErr[key]}
                  </p>{/if}
              </div>
            </section>
          {/if}

          {#if openPanel[key] === "generate"}
            <section
              id="repo-generate-{key}"
              class="trace-panel"
              aria-label="Generate a note for {r.owner}/{r.repo}"
            >
              <p class="trace-label">Note proof / {r.owner}/{r.repo}</p>
              <p class="mt-2 max-w-2xl text-sm text-ink-2">
                Generate a note for a release tag. A normal run preserves an
                existing hand-edited note; overwrite replaces it.
              </p>
              <div class="mt-3 flex items-center gap-3">
                <label class="flex items-center gap-2 text-sm text-ink"
                  ><input
                    type="checkbox"
                    bind:checked={publish[key]}
                    class="check-input"
                  />Publish to release</label
                >
                <span class="text-xs text-ink-3"
                  >Preview-only by default. Check to write the note to the
                  release.</span
                >
              </div>
              <div
                class="mt-4 grid gap-3 sm:grid-cols-2"
                aria-label="Generate mode for {r.owner}/{r.repo}"
              >
                <button
                  type="button"
                  aria-pressed={!force[key] ? "true" : "false"}
                  onclick={() => {
                    force[key] = false;
                    overwriteConfirm[key] = false;
                  }}
                  class="mode-choice {!force[key] ? 'mode-choice--active' : ''}"
                  ><span
                    class="flex items-center gap-2 text-sm font-medium text-ink"
                    ><span
                      class="signal-dot {!force[key]
                        ? 'signal-dot--heat'
                        : 'signal-dot--muted'}"
                      aria-hidden="true"
                    ></span>Generate</span
                  ><span class="mt-1 text-xs leading-relaxed text-ink-3"
                    >Reuse the note already on file and never overwrite a
                    release body edited by hand.</span
                  ></button
                >
                <button
                  type="button"
                  aria-pressed={force[key] ? "true" : "false"}
                  onclick={() => {
                    force[key] = true;
                    overwriteConfirm[key] = false;
                  }}
                  class="mode-choice {force[key] ? 'mode-choice--danger' : ''}"
                  ><span
                    class="flex items-center gap-2 text-sm font-medium text-ink"
                    ><span
                      class="signal-dot {force[key]
                        ? 'signal-dot--heat'
                        : 'signal-dot--muted'}"
                      aria-hidden="true"
                    ></span>Overwrite</span
                  ><span class="mt-1 text-xs leading-relaxed text-ink-3"
                    >Destroy the existing note body and generate a new one. This
                    cannot be undone.</span
                  ></button
                >
              </div>
              {#if force[key] && overwriteConfirm[key]}
                <div class="mt-3 panel panel--error" role="alert">
                  <p class="text-sm text-alert">
                    This will destroy the existing note for <strong
                      >{generateTag[key] || "this release tag"}</strong
                    >. Are you sure?
                  </p>
                  <div class="mt-2 flex flex-wrap gap-2">
                    <button
                      onclick={() => runGenerate(r)}
                      disabled={pending[key]?.generating}
                      class="btn btn-primary">Yes, overwrite</button
                    >
                    <button
                      onclick={() => {
                        force[key] = false;
                        overwriteConfirm[key] = false;
                      }}
                      class="btn btn-secondary">Cancel</button
                    >
                  </div>
                </div>
              {/if}
              <div class="mt-4 flex flex-wrap items-end gap-3">
                <label class="field-group min-w-0 flex-1 sm:max-w-xs"
                  ><span class="field-group__label">Release tag</span><input
                    bind:value={generateTag[key]}
                    placeholder="v1.0.0"
                    class="field"
                  />{#if genTagError[key]}<span
                      class="field-group__error"
                      role="alert">{genTagError[key]}</span
                    >{/if}</label
                ><button
                  onclick={() => runGenerate(r)}
                  disabled={pending[key]?.generating ||
                    (force[key] && overwriteConfirm[key])}
                  class="btn btn-primary w-full sm:w-auto"
                  >{pending[key]?.generating
                    ? "Generating…"
                    : publish[key]
                      ? "Generate & publish"
                      : "Preview"}</button
                >
              </div>
              <div aria-live="polite" class="mt-4">
                {#if genError[key]}<p class="text-sm text-alert" role="alert">
                    {genError[key]}
                  </p>{/if}{#if notesOut[key]}<p class="mt-1 text-sm text-ok">
                    Note generated.
                  </p>
                  <div class="mt-4">
                    <p class="trace-label">Generated note / release proof</p>
                    <pre
                      class="note-paper mt-2 max-h-96 overflow-auto">{notesOut[
                        key
                      ]}</pre>
                  </div>{/if}
              </div>
            </section>
          {/if}
        </article>
      {/each}
    </section>
  {/if}
</div>
