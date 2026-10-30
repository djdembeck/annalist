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
    getRuns,
    type GenerationRun,
    type Repo,
  } from "$lib/api";
  import MarkdownPreview from "$lib/components/MarkdownPreview.svelte";
  import RunStatus from "$lib/components/RunStatus.svelte";
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
  let loadErrorDetail = $state("");
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
  let openPanel = $state<
    Record<string, "settings" | "generate" | "history" | undefined>
  >({});
  let drafts = $state<Record<string, Draft>>({});
  let draftBaselines = $state<Record<string, string>>({});
  let force = $state<Record<string, boolean>>({});
  let publish = $state<Record<string, boolean>>({});
  let notesOut = $state<Record<string, string | null>>({});
  let genError = $state<Record<string, string | null>>({});
  let generateTag = $state<Record<string, string>>({});
  let generateFromTag = $state<Record<string, string>>({});
  let generateProfile = $state<Record<string, string>>({});
  let generateDisplayVersion = $state<Record<string, string>>({});
  let genTagError = $state<Record<string, string | null>>({});
  let overwriteConfirm = $state<Record<string, boolean>>({});
  let temperatureError = $state<Record<string, string | null>>({});
  let maxTokensError = $state<Record<string, string | null>>({});

  let history = $state<Record<string, GenerationRun[]>>({});
  let historyError = $state<Record<string, string>>({});
  let historyPending = $state<Record<string, boolean>>({});
  let generationMessage = $state<Record<string, string>>({});
  // User-initiated refresh only; the 15s poll must not disable/relabel the
  // manual Refresh button.
  let manualRefreshing = $state(false);
  let mounted = false;

  onMount(() => {
    mounted = true;
    void refresh();
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") void refresh(true);
    }, 15000);
    return () => {
      mounted = false;
      window.clearInterval(timer);
    };
  });

  function runTime(run: GenerationRun): string {
    return new Date(run.finished_at ?? run.started_at).toLocaleString();
  }

  function failureCopy(code: string | null): string {
    switch (code) {
      case "configuration":
        return "Check server configuration, local state, the data directory, and server logs, then try again.";
      case "repository":
        return "Check repository access and that the release tag exists, then try again.";
      case "instructions":
        return "Check repository instructions and access permissions, then try again.";
      case "generation":
        return "The model could not generate notes. Check the provider and model settings, then try again.";
      case "publication":
        return "Notes could not be published. Check release permissions and try again.";
      case "timeout":
        return "The request timed out. Check the provider and try again.";
      case "cancelled":
        return "The request was cancelled. Start another generation when ready.";
      case "interrupted":
        return "The server stopped before this run finished. Check the release before retrying.";
      default:
        return "This attempt did not complete. Check repository and provider settings before retrying.";
    }
  }

  function warningCopy(code: string | null): string {
    return code === "cache_persistence"
      ? "Published, but the local note cache could not be saved. Check server storage before generating again."
      : "The run completed with a warning. Check server configuration before generating again.";
  }

  async function loadHistory(r: Repo): Promise<void> {
    const key = repoKey(r);
    if (historyPending[key]) return;
    historyPending[key] = true;
    historyError[key] = "";
    try {
      const runs = await getRuns({
        platform: r.platform,
        owner: r.owner,
        repo: r.repo,
      });
      if (mounted) history[key] = runs;
    } catch (e) {
      if (handleAuthError(e)) return;
      historyError[key] =
        "Could not load recent runs. Check your connection and retry.";
    } finally {
      historyPending[key] = false;
    }
  }

  function openHistory(r: Repo): void {
    const key = repoKey(r);
    openPanel[key] = openPanel[key] === "history" ? undefined : "history";
    if (openPanel[key] === "history") void loadHistory(r);
  }

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
    if (inRepoInstructions[key] !== undefined || inRepoPending[key]) return;
    inRepoPending[key] = true;
    getInRepoInstructions(repo.platform, repo.owner, repo.repo)
      .then((content) => {
        inRepoInstructions[key] = content;
        inRepoPending[key] = false;
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

  let refreshFlight: Promise<void> | null = null;
  let refreshQueued = false;
  let refreshModelsQueued = false;

  function refresh(background = false): Promise<void> {
    if (refreshFlight) {
      if (!background) {
        refreshQueued = true;
        refreshModelsQueued = true;
      }
      return refreshFlight;
    }
    if (!background) manualRefreshing = true;
    refreshModelsQueued = !background;
    refreshFlight = (async () => {
      try {
        do {
          refreshQueued = false;
          const inFlight = !background;
          const refreshModels = refreshModelsQueued;
          refreshModelsQueued = false;
          loadError = "";
          loadErrorDetail = "";
          // Only user-initiated flights replace a settled view; the 15s
          // background poll must not swap an error banner for a skeleton.
          if (inFlight && (loadState === "idle" || loadState === "error"))
            loadState = "loading";
          try {
            const incoming = await getRepos();
            if (!mounted) return;
            // A user read queued while this fetch was in flight is
            // authoritative: drop this older snapshot without touching
            // repos/loadState and loop so the queued read gets a fresh one.
            if (refreshQueued) continue;
            repos = incoming;
            loadState = "success";
            if (refreshModels) void loadModels();
            for (const r of repos) {
              if (openPanel[repoKey(r)] === "history") void loadHistory(r);
            }
          } catch (e) {
            if (handleAuthError(e)) return;
            if (loadState === "loading") loadState = "error";
            loadError =
              "Could not refresh repositories. Check your connection and try again.";
            loadErrorDetail = formatError(e, "");
          }
        } while (mounted && refreshQueued);
      } finally {
        // Clear in the flight's own job so queued callers cannot race settlement.
        refreshFlight = null;
        manualRefreshing = false;
      }
    })();
    return refreshFlight;
  }

  function openSettings(r: Repo): void {
    const key = repoKey(r);
    const tone = r.tone ?? "";
    const isPreset = PRESETS.includes(tone);
    const commitTypeSelection = getCommitTypeSelection(r.commit_types);
    // Re-seed from the saved row when the panel carries no unsaved edits,
    // so a stale draft cannot shadow the server values on reopen.
    if (
      drafts[key] &&
      JSON.stringify(drafts[key]) === draftBaselines[key]
    ) {
      delete drafts[key];
      delete draftBaselines[key];
    }
    if (!drafts[key]) {
      drafts[key] = {
        toneOption: !tone ? "inherit" : isPreset ? tone : "custom",
        customTone: isPreset ? "" : tone,
        instructions: r.instructions ?? "",
        model: r.model ?? "",
        mode:
          r.mode === "deep" ? "deep" : r.mode === "lite" ? "lite" : "inherit",
        temperature: r.temperature === null ? "" : String(r.temperature),
        trigger: r.trigger ?? "auto",
        maxTokens:
          r.max_tokens === 0 || r.max_tokens == null
            ? ""
            : String(r.max_tokens),
        thinkingLevel:
          r.thinking_level === null || r.thinking_level === ""
            ? "inherit"
            : r.thinking_level,
        inheritCommitTypes: !r.commit_types,
        selectedCommitTypes: commitTypeSelection.selected,
        customCommitTypes: commitTypeSelection.custom.join(", "),
      };
      draftBaselines[key] = JSON.stringify(drafts[key]);
    }
    openPanel[key] = openPanel[key] === "settings" ? undefined : "settings";
    // Ensure in-repo instructions are loaded.
    loadInRepoInstructions(r);
  }

  function openGenerate(r: Repo): void {
    const key = repoKey(r);
    generateTag[key] ??= "";
    // A fresh manual generation carries no webhook lineage.
    generateFromTag[key] = "";
    generateProfile[key] = "";
    generateDisplayVersion[key] = "";
    overwriteConfirm[key] = false;
    force[key] = false;
    publish[key] = false;
    openPanel[key] = openPanel[key] === "generate" ? undefined : "generate";
  }

  function retryGeneration(r: Repo, run: GenerationRun): void {
    const key = repoKey(r);
    generateTag[key] = run.to_tag;
    // Retry the same contract the run used, so named-profile webhook runs
    // keep their from_tag/profile/display_version.
    generateFromTag[key] = run.from_tag ?? "";
    generateProfile[key] = run.profile ?? "";
    generateDisplayVersion[key] = run.display_version ?? "";
    force[key] ??= false;
    publish[key] ??= false;
    overwriteConfirm[key] = false;
    genTagError[key] = null;
    genError[key] = null;
    notesOut[key] = null;
    // The generate panel keeps its publish/force checkboxes, so say what the
    // panel will actually do on submit.
    generationMessage[key] = publish[key]
      ? "Retrying the same contract — publish is on, so this will publish on success."
      : "Retrying the same contract in preview — publish is off; review the notes before publishing.";
    openPanel[key] = "generate";
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
    generationMessage[key] = "";
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
    pending[key] = { ...(pending[key] ?? {}), generating: true };
    try {
      const result = await generate(r.platform, r.owner, r.repo, {
        to_tag: toTag,
        from_tag: (generateFromTag[key] ?? "").trim() || null,
        profile: generateProfile[key] ?? "",
        display_version: (generateDisplayVersion[key] ?? "").trim() || null,
        force: force[key] ?? false,
        publish: publish[key] ?? false,
      });
      notesOut[key] = result.notes;
      const messages = {
        published: "Notes published to the release.",
        generated: "Preview generated. Nothing was published.",
        cached: "Cached notes returned. No new publication was made.",
        preserved:
          "The existing release body was preserved. Nothing was overwritten.",
      };
      generationMessage[key] =
        messages[result.outcome] +
        (result.warning_code ? " " + warningCopy(result.warning_code) : "");
      await refresh();
      overwriteConfirm[key] = false;
    } catch (e) {
      if (handleAuthError(e)) return;
      genError[key] =
        "Generation did not complete. Check that the tag exists, repository access is configured, and the model provider is reachable.";
      await refresh();
    } finally {
      pending[key] = { ...(pending[key] ?? {}), generating: false };
    }
  }
</script>

<svelte:head><title>Repositories · Annalist</title></svelte:head>
<div class="workspace">
  <SectionHead title="Repositories">
    {#snippet actions()}
      <button
        disabled={manualRefreshing}
        onclick={() => refresh(false)}
        class="btn btn-secondary"
        >{manualRefreshing ? "Refreshing…" : "Refresh"}</button
      >
      <a href="/repos/add" class="btn btn-primary">Add repositories</a>
    {/snippet}
  </SectionHead>
  {#if loadState === "loading" || loadState === "idle"}
    <section class="panel" aria-busy="true" aria-label="Loading repositories">
      <p role="status">Loading repositories…</p>
      <div class="skeleton mt-4 h-24"></div>
    </section>
  {:else}
    {#if loadError}<ErrorBanner
        message={loadError}
        detail={loadErrorDetail || undefined}
        actionLabel="Retry"
        onAction={() => refresh()}
      />{/if}
    {#if modelsError}<ErrorBanner
        message={modelsError}
        actionLabel="Retry models"
        onAction={loadModels}
        variant="warning"
      />{/if}
    {#if loadState === "success" && repos.length === 0}
      <EmptyState
        heading="No repositories yet"
        description="Add a GitHub or Forgejo repository to configure automation and generate release notes."
        actionLabel="Add repositories"
        href="/repos/add"
      />
    {:else if repos.length > 0}
      <section class="plane" aria-label="Repository register">
        <table class="repo-table">
          <colgroup
            ><col class="repo-col" /><col class="latest-col" /><col
              class="automation-col"
            /><col class="actions-col" /></colgroup
          >
          <thead
            ><tr
              ><th scope="col">Repository</th><th scope="col"
                >Latest generation</th
              ><th scope="col">Automation</th><th scope="col">Actions</th></tr
            ></thead
          >
          {#each repos as r (repoKey(r))}
            {@const key = repoKey(r)}
            {@const run = r.latest_run}
            <tbody class="data-row">
              <tr>
                <td data-label="Repository"
                  ><div class="repository">{r.owner}/<wbr />{r.repo}</div>
                  <span class="platform"
                    >{r.platform === "github"
                      ? "GitHub"
                      : r.platform === "forgejo"
                        ? "Forgejo"
                        : r.platform}</span
                  ></td
                >
                <td data-label="Latest generation">
                  <div class="result">
                    <RunStatus status={run?.status} />{#if run}<span
                        class="version"
                        >{run.display_version || run.to_tag}</span
                      >{/if}
                  </div>
                  {#if run}<div class="when">
                      <time datetime={run.finished_at ?? run.started_at}
                        >{runTime(run)}</time
                      >
                    </div>{/if}
                  <button
                    class="action-link history-link"
                    onclick={() => openHistory(r)}
                    aria-expanded={openPanel[key] === "history"}
                    >Recent runs</button
                  >
                </td>
                <td data-label="Automation">
                  <label class="automation-control"
                    ><input
                      type="checkbox"
                      aria-label={"Enable " + r.owner + "/" + r.repo}
                      checked={r.enabled}
                      disabled={pending[key]?.toggling}
                      onchange={() => toggleEnabled(r)}
                    /><span>{r.enabled ? "On" : "Off"}</span></label
                  >
                  {#if r.enabled}<span class="trigger"
                      >{r.trigger === "manual"
                        ? "Manual only"
                        : "On release"}</span
                    >{/if}
                  {#if toggleErr[key]}<p class="text-alert" role="alert">
                      {toggleErr[key]}
                    </p>{/if}
                </td>
                <td data-label="Actions"
                  ><div class="actions">
                    <button
                      class="btn btn-secondary"
                      onclick={() => openGenerate(r)}
                      aria-expanded={openPanel[key] === "generate"}
                      >{openPanel[key] === "generate"
                        ? "Close generation"
                        : "Generate notes"}</button
                    >
                    <button
                      class="action-link"
                      onclick={() => openSettings(r)}
                      aria-expanded={openPanel[key] === "settings"}
                      >{openPanel[key] === "settings"
                        ? "Close settings"
                        : "Settings"}</button
                    >
                  </div></td
                >
              </tr>
              {#if run && (run.status === "failed" || run.status === "interrupted" || run.status === "preserved" || run.warning_code)}
                <tr class="failure-row"
                  ><td colspan="4"
                    ><div class="failure-copy">
                      {#if run.status === "failed" || run.status === "interrupted"}<p
                        >
                          {failureCopy(
                            run.error_code ??
                              (run.status === "interrupted"
                                ? "interrupted"
                                : null),
                          )}
                        </p>
                      {:else if run.status === "preserved"}<p>
                          The existing release body was preserved. Use overwrite
                          only if you intend to replace it.
                        </p>{/if}
                      {#if run.warning_code}<p>
                          {warningCopy(run.warning_code)}
                        </p>{/if}
                      <div class="failure-actions">
                        {#if run.status === "failed" || run.status === "interrupted"}<button
                            class="btn btn-primary"
                            onclick={() => retryGeneration(r, run)}
                            >Retry generation</button
                          >{/if}<button
                          class="action-link"
                          onclick={() => openHistory(r)}
                          >View run details</button
                        >
                      </div>
                    </div></td
                  ></tr
                >
              {/if}
              {#if openPanel[key]}
                <tr class="detail-row"
                  ><td colspan="4">
                    {#if openPanel[key] === "settings"}
                      {@const d = drafts[key]}
                      <section
                        id="repo-settings-{key}"
                        class="repo-panel"
                        aria-label="Settings for {r.owner}/{r.repo}"
                      >
                        <p class="field-caption">
                          Settings / {r.owner}/{r.repo}
                        </p>
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
                          {#if d.toneOption === "custom"}<label
                              class="field-group"
                              ><span class="field-group__label"
                                >Custom tone</span
                              ><input
                                bind:value={d.customTone}
                                placeholder="Freeform persona"
                                class="field"
                              /></label
                            >{/if}
                          <label class="field-group md:col-span-2"
                            ><span class="field-group__label">Instructions</span
                            ><textarea
                              bind:value={d.instructions}
                              rows="3"
                              class="field"></textarea><span
                              class="field-group__hint"
                              >{#if inRepoPending[key] === true}Loading in-repo
                                instructions…
                              {:else if inRepoInstructions[key]}The repository's
                                release-notes-instructions.md file takes
                                priority over this saved field.
                              {:else}When set, these instructions replace the
                                global writing instructions.{/if}</span
                            ></label
                          >
                          <label class="field-group"
                            ><span class="field-group__label"
                              >Model <span class="field-group__hint"
                                >(blank = inherit)</span
                              ></span
                            ><select bind:value={d.model} class="field"
                              ><option value="">Inherit (system default)</option
                              >{#each models as m (m)}<option value={m}
                                  >{m}</option
                                >{/each}{#if d.model && !models.includes(d.model)}<option
                                  value={d.model}
                                  >{d.model} (not in /models list)</option
                                >{/if}</select
                            ><span class="field-group__hint"
                              >Empty inherits the global defaults, then the
                              server default.</span
                            ></label
                          >
                          <label class="field-group"
                            ><span class="field-group__label"
                              >Temperature <span class="field-group__hint"
                                >(blank = inherit)</span
                              ></span
                            ><input
                              type="number"
                              step="any"
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
                            ><span class="field-group__label"
                              >Thinking level</span
                            ><select bind:value={d.thinkingLevel} class="field"
                              ><option value="inherit">Inherit (global)</option
                              ><option value="off"
                                >off — no extended thinking</option
                              ><option value="low">low</option><option
                                value="medium">medium</option
                              ><option value="high">high</option></select
                            ><span class="field-group__hint"
                              >Sent to the model as reasoning_effort; off sends
                              "none".</span
                            ></label
                          >
                          <label class="field-group"
                            ><span class="field-group__label">Trigger</span
                            ><select bind:value={d.trigger} class="field"
                              ><option value="auto">auto</option><option
                                value="manual">manual</option
                              ></select
                            ><span class="field-group__hint"
                              >Auto runs on release webhooks; manual disables
                              webhooks.</span
                            ></label
                          >
                          <label class="field-group"
                            ><span class="field-group__label">Mode</span><select
                              bind:value={d.mode}
                              class="field"
                              ><option value="inherit">Inherit (global)</option
                              ><option value="lite"
                                >lite — commit log only</option
                              ><option value="deep"
                                >deep — commit log + diff</option
                              ></select
                            ><span class="field-group__hint"
                              >deep also sends the code diff to the model for
                              this repository.</span
                            ></label
                          >
                          <fieldset class="field-group md:col-span-2">
                            <legend class="field-group__label"
                              >Commit types</legend
                            >
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
                                    d.selectedCommitTypes = [
                                      ...DEFAULT_COMMIT_TYPES,
                                    ];
                                  }
                                }}
                              />
                              <span>Inherit global selection</span>
                            </label>
                            <CommitTypeSelector
                              bind:selected={d.selectedCommitTypes}
                              bind:custom={d.customCommitTypes}
                              showOptions={!d.inheritCommitTypes}
                              hint={d.inheritCommitTypes
                                ? "Uses the global selection. Turn off inheritance to choose repository-specific types."
                                : isKeepAll(
                                      formatCommitTypes(
                                        d.selectedCommitTypes,
                                        d.customCommitTypes,
                                      ),
                                    )
                                  ? "No filter — all commit types are included."
                                  : "Selected types are included in notes."}
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
                          <span class="text-ink"
                            >{r.effective.model ?? "inherit"}</span
                          >
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
                          · mode
                          <span class="text-ink"
                            >{r.effective.mode ?? "inherit"}</span
                          >
                          · max tokens
                          <span class="text-ink">{r.effective.max_tokens}</span>
                          · thinking
                          <span class="text-ink"
                            >{r.effective.thinking_level || "off"}</span
                          >
                        </p>
                        {#if inRepoInstructions[key] || r.effective.instructions}
                          <details class="mt-4">
                            <summary
                              class="field-caption cursor-pointer select-none"
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
                            >{pending[key]?.saving
                              ? "Saving…"
                              : "Save settings"}</button
                          ><span class="text-xs text-ink-3"
                            >Save before generating; generation uses saved
                            settings.</span
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
                        class="repo-panel"
                        aria-label="Generate a note for {r.owner}/{r.repo}"
                      >
                        <p class="field-caption">
                          Generate notes / {r.owner}/{r.repo}
                        </p>
                        <p class="mt-2 max-w-2xl text-sm text-ink-2">
                          Generate a note for a release tag. A normal run
                          preserves an existing hand-edited note; overwrite
                          replaces it.
                        </p>
                        <div class="mt-3 flex items-center gap-3">
                          <label
                            class="flex items-center gap-2 text-sm text-ink"
                            ><input
                              type="checkbox"
                              bind:checked={publish[key]}
                              class="check-input"
                            />Publish to release</label
                          >
                          <span class="text-xs text-ink-3"
                            >Preview-only by default. Check to write the note to
                            the release.</span
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
                            class="mode-choice {!force[key]
                              ? 'mode-choice--active'
                              : ''}"
                            ><span
                              class="flex items-center gap-2 text-sm font-medium text-ink"
                              >Generate</span
                            ><span
                              class="mt-1 text-xs leading-relaxed text-ink-3"
                              >Reuse the note already on file and never
                              overwrite a release body edited by hand.</span
                            ></button
                          >
                          <button
                            type="button"
                            aria-pressed={force[key] ? "true" : "false"}
                            onclick={() => {
                              force[key] = true;
                              overwriteConfirm[key] = false;
                            }}
                            class="mode-choice {force[key]
                              ? 'mode-choice--danger'
                              : ''}"
                            ><span
                              class="flex items-center gap-2 text-sm font-medium text-ink"
                              >Overwrite</span
                            ><span
                              class="mt-1 text-xs leading-relaxed text-ink-3"
                              >Generate fresh notes instead of reusing cached
                              notes. When publishing, replace the existing
                              release body.</span
                            ></button
                          >
                        </div>
                        {#if force[key] && overwriteConfirm[key]}
                          <div class="mt-3 panel panel--error" role="alert">
                            <p class="text-sm text-alert">
                              {publish[key]
                                ? "This will replace the existing release body for"
                                : "Generate fresh preview notes for"}
                              <strong
                                >{generateTag[key] ||
                                  "this release tag"}</strong
                              >{publish[key]
                                ? ". This publication cannot be undone. Continue?"
                                : "? Nothing will be published."}
                            </p>
                            <div class="mt-2 flex flex-wrap gap-2">
                              <button
                                onclick={() => runGenerate(r)}
                                disabled={pending[key]?.generating}
                                class="btn btn-primary"
                                >{publish[key]
                                  ? "Yes, overwrite"
                                  : "Generate fresh preview"}</button
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
                            ><span class="field-group__label">Release tag</span
                            ><input
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
                          {#if genError[key]}<p
                              class="text-sm text-alert"
                              role="alert"
                            >
                              {genError[key]}
                            </p>{/if}
                          {#if generationMessage[key]}<p
                              class="text-sm"
                              role="status"
                            >
                              {generationMessage[key]}
                            </p>{/if}
                          {#if notesOut[key]}
                            <div class="mt-4">
                              <MarkdownPreview source={notesOut[key] ?? ""} />
                            </div>{/if}
                        </div>
                      </section>
                    {/if}

                    {#if openPanel[key] === "history"}
                      <section
                        class="repo-panel"
                        aria-label="Recent runs for {r.owner}/{r.repo}"
                      >
                        <div class="panel-heading">
                          <h2>Recent runs</h2>
                          <button
                            class="action-link"
                            onclick={() => {
                              openPanel[key] = undefined;
                            }}>Close history</button
                          >
                        </div>
                        {#if historyError[key]}<ErrorBanner
                            message={historyError[key]}
                            actionLabel="Retry history"
                            onAction={() => loadHistory(r)}
                          />{/if}
                        {#if historyPending[key] && !history[key]}<p
                            role="status"
                          >
                            Loading recent runs…
                          </p>
                        {:else if history[key]?.length === 0}<p>
                            No recorded server generation attempts for this
                            repository.
                          </p>
                        {:else if history[key]}<ol class="run-history">
                            {#each history[key] as item (item.id)}
                              <li>
                                <div class="result">
                                  <RunStatus status={item.status} /><strong
                                    >{item.display_version ||
                                      item.to_tag}</strong
                                  >
                                </div>
                                <p class="when">
                                  {item.source === "webhook"
                                    ? "Release webhook"
                                    : "Manual request"} ·
                                  <time datetime={item.started_at}
                                    >{runTime(item)}</time
                                  >
                                </p>
                                <dl class="run-metadata">
                                  <div>
                                    <dt>Tag</dt>
                                    <dd>{item.to_tag}</dd>
                                  </div>
                                  {#if item.from_tag}<div>
                                      <dt>From</dt>
                                      <dd>{item.from_tag}</dd>
                                    </div>{/if}{#if item.profile}<div>
                                      <dt>Profile</dt>
                                      <dd>{item.profile}</dd>
                                    </div>{/if}
                                  <div>
                                    <dt>Started</dt>
                                    <dd>
                                      {new Date(
                                        item.started_at,
                                      ).toLocaleString()}
                                    </dd>
                                  </div>
                                  {#if item.finished_at}<div>
                                      <dt>Finished</dt>
                                      <dd>
                                        {new Date(
                                          item.finished_at,
                                        ).toLocaleString()}
                                      </dd>
                                    </div>{/if}
                                </dl>
                                {#if item.status === "failed" || item.status === "interrupted"}<p
                                  >
                                    {failureCopy(
                                      item.error_code ??
                                        (item.status === "interrupted"
                                          ? "interrupted"
                                          : null),
                                    )}
                                  </p>{/if}
                                {#if item.status === "preserved"}<p>
                                    The existing release body was not
                                    overwritten.
                                  </p>{/if}
                                {#if item.warning_code}<p>
                                    {warningCopy(item.warning_code)}
                                  </p>{/if}
                              </li>
                            {/each}
                          </ol>{/if}
                      </section>
                    {/if}
                  </td></tr
                >
              {/if}
            </tbody>
          {/each}
        </table>
      </section>
    {/if}
  {/if}
</div>
