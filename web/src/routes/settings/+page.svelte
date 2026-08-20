<script lang="ts">
  import { onMount } from "svelte";
  import { parseTemperature, resolveTone } from "$lib/repoUtils";
  import {
    DEFAULT_COMMIT_TYPES,
    formatCommitTypes,
    getCommitTypeSelection,
  } from "$lib/commitTypes";
  import {
    getSettings,
    putSettings,
    getModels,
    type Settings,
    type SettingsUpdate,
  } from "$lib/api";
  import CommitTypeSelector from "$lib/components/CommitTypeSelector.svelte";
  import ErrorBanner from "$lib/components/ErrorBanner.svelte";
  import SectionHead from "$lib/components/SectionHead.svelte";
  import {
    handleAuthError,
    formatError,
  } from "$lib/composables/useAuthErrorHandler";

  const PRESET_OPTIONS = ["chronicler", "engineer", "launch"];

  let settings = $state<Settings | null>(null);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state("");
  let saved = $state(false);
  let toneOption = $state("inherit");
  let customTone = $state("");
  let instructions = $state("");
  let model = $state("");
  let baseUrl = $state("");
  let apiKey = $state("");
  let apiKeyTouched = $state(false);
  let models = $state<string[]>([]);
  let modelsError = $state("");
  let mode = $state("lite");
  let temperature = $state("");
  let temperatureError = $state("");
  let selectedCommitTypes = $state<string[]>(DEFAULT_COMMIT_TYPES);
  let customCommitTypes = $state("");
  let commitTypesDirty = $state(false);

  let platformStatus = $derived(
    settings
      ? settings.github && settings.forgejo
        ? "Both GitHub and Forgejo are configured."
        : settings.github
          ? "GitHub configured, Forgejo not configured."
          : settings.forgejo
            ? "Forgejo configured, GitHub not configured."
            : "No platforms configured yet — add a platform below to start."
      : undefined,
  );
  let headerLede = $derived(
    settings
      ? `${platformStatus} · Base URL ${settings.llm.base_url || "not set"} · Model ${model.trim() || "system default"}`
      : undefined,
  );

  async function load(): Promise<void> {
    try {
      settings = await getSettings();
      const s = settings;
      const tone = s.tone ?? "";
      toneOption = !tone
        ? "inherit"
        : PRESET_OPTIONS.includes(tone)
          ? tone
          : "custom";
      customTone = PRESET_OPTIONS.includes(tone) ? "" : tone;
      instructions = s.instructions ?? "";
      model = s.model ?? "";
      baseUrl = s.llm.base_url ?? "";
      apiKey = "";
      apiKeyTouched = false;
      mode = s.mode === "deep" ? "deep" : "lite";
      temperature = s.temperature === null ? "" : String(s.temperature);
      const commitTypeSelection = getCommitTypeSelection(s.commit_types);
      const hasSavedCommitTypes = Boolean(s.commit_types?.trim());
      selectedCommitTypes = hasSavedCommitTypes
        ? commitTypeSelection.selected
        : [...DEFAULT_COMMIT_TYPES];
      customCommitTypes = commitTypeSelection.custom.join(", ");
      commitTypesDirty = false;
      temperatureError = "";
      error = "";
    } catch (e) {
      if (handleAuthError(e)) return;
      error = formatError(
        e,
        "Could not load settings — the server may be unreachable. Check your connection and try refreshing.",
      );
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    loadModels();
  });

  async function loadModels(): Promise<void> {
    modelsError = "";
    try {
      models = await getModels();
    } catch (e) {
      if (handleAuthError(e)) return;
      modelsError = formatError(
        e,
        "Could not load the model list — check the base URL and API key below.",
      );
    }
  }

  async function save(): Promise<void> {
    saved = false;
    saving = true;
    const tone = resolveTone(toneOption, customTone);
    try {
      const tempResult = parseTemperature(temperature);
      if (tempResult.error) {
        temperatureError = tempResult.error;
        saving = false;
        return;
      }
      const payload: SettingsUpdate = {
        tone,
        instructions: instructions.trim() ? instructions : null,
        model: model.trim() ? model : null,
        mode,
        temperature: tempResult.value,
        commit_types: commitTypesDirty
          ? formatCommitTypes(selectedCommitTypes, customCommitTypes)
          : (settings?.commit_types ?? null),
      };
      // Only send llm_base_url when the effective value changed (null = revert
      // to env). Only send llm_api_key when the field was touched (null = clear
      // the stored key; a blanked, untouched key is omitted so it's kept).
      // Note the asymmetry: the base URL field is initialized from the same
      // *effective* value (env/config when nothing is saved) that the server
      // returns, so both comparison operands are the same value — an untouched
      // field always matches and no llm_base_url is sent; a set is only sent
      // when the field is actually edited (blank → null = revert to env).
      // The API key is never echoed back, so it needs the apiKeyTouched flag
      // to know whether the field was edited at all.
      if (baseUrl.trim() !== (settings?.llm.base_url ?? "")) {
        payload.llm_base_url = baseUrl.trim() === "" ? null : baseUrl.trim();
      }
      if (apiKeyTouched) {
        payload.llm_api_key = apiKey === "" ? null : apiKey;
      }
      settings = await putSettings(payload);
      commitTypesDirty = false;
      apiKey = "";
      apiKeyTouched = false;
      saved = true;
      temperatureError = "";
      error = "";
    } catch (e) {
      if (handleAuthError(e)) return;
      error = formatError(
        e,
        "Failed to save settings — check your connection and try again.",
      );
    } finally {
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>Settings · Annalist</title>
  <meta
    name="description"
    content="Tune Annalist's global release-note contract and tone."
  />
</svelte:head>

<div class="trace-wall">
  <SectionHead
    label="Operations / global contract"
    title="Machine contract &amp; tone proof"
    lede={headerLede}
  >
    {#snippet actions()}
      <a href="/repos" class="btn btn-secondary">Repository inventory</a>
    {/snippet}
  </SectionHead>

  {#if error}
    <ErrorBanner label="Contract update needs attention" message={error} />
  {/if}

  {#if loading}
    <section class="panel" aria-busy="true" aria-label="Loading settings">
      <p class="trace-label">Reading machine contract</p>
      <div class="skeleton mt-4 h-12 w-full"></div>
      <div class="skeleton mt-3 h-28 w-full"></div>
      <div class="skeleton mt-3 h-12 w-1/2"></div>
    </section>
  {:else if settings}
    <div class="settings-layout">
      <section
        class="panel settings-form"
        aria-label="Global release note defaults"
      >
        <SectionHead
          label="LLM endpoint"
          title="Connection"
          compact
          headingLevel="h2"
        >
          {#snippet actions()}
            <button class="btn btn-secondary" onclick={loadModels}>
              Refresh models
            </button>
          {/snippet}
        </SectionHead>
        {#if modelsError}
          <ErrorBanner
            label="Model list needs attention"
            message={modelsError}
            actionLabel="Retry"
            onAction={loadModels}
          />
        {/if}
        <div class="grid gap-5 sm:grid-cols-2">
          <label class="field-group"
            ><span class="field-group__label">Base URL</span><input
              bind:value={baseUrl}
              placeholder="https://llm.example.com (host only, no path)"
              class="field" /><span class="field-group__hint"
              >OpenAI-compatible endpoint (scheme://host[:port]). Overrides the
              LLM_BASE_URL env value while set; clear the field to fall back
              to env.</span
            ></label
          >
          <label class="field-group"
            ><span class="field-group__label"
              >API key
              {#if settings?.llm.has_key}
              <span class="field-group__hint"
                >(saved or env: {settings.llm.api_key})</span
              >
              {:else}
              <span class="field-group__hint">(none set)</span>
              {/if}</span
            ><input
              bind:value={apiKey}
              type="password"
              oninput={() => (apiKeyTouched = true)}
              placeholder={settings?.llm.has_key ? "Leave blank to keep current key" : "sk-..."}
              class="field" /><span class="field-group__hint"
              >Overrides LLM_API_KEY while non-empty; a blanked, saved key is
              cleared on Save.</span
            >{#if apiKeyTouched && apiKey === "" && settings?.llm.has_key}
              <span class="field-group__hint" role="status"
                >Blank + save = clear the stored key.</span
              >
            {/if}</label
          >
        </div>

        <SectionHead
          label="Tone contract"
          title="Global defaults"
          compact
          headingLevel="h2"
        >
          {#snippet actions()}
            <span class="status status--healthy"
              ><span class="signal-dot signal-dot--muted" aria-hidden="true"
              ></span>Editable</span
            >
          {/snippet}
        </SectionHead>
        <div class="grid gap-5">
          <label class="field-group"
            ><span class="field-group__label">Tone</span><select
              bind:value={toneOption}
              class="field"
              ><option value="inherit">Inherit (neutral)</option
              >{#each PRESET_OPTIONS as p (p)}<option value={p}>{p}</option
                >{/each}<option value="custom">Custom…</option></select
            ><span class="field-group__hint"
              >Choose the tone used when a repository does not override it.</span
            ></label
          >
          {#if toneOption === "custom"}<label class="field-group"
              ><span class="field-group__label">Custom tone</span><input
                bind:value={customTone}
                placeholder="Freeform persona"
                class="field"
              /></label
            >{/if}
          <label class="field-group"
            ><span class="field-group__label">Instructions</span><textarea
              bind:value={instructions}
              rows="5"
              placeholder="Additional instructions for note generation"
              class="field"></textarea><span class="field-group__hint"
              >These instructions travel with the global tone contract.</span
            ></label
          >
          <div class="grid gap-5 sm:grid-cols-2">
            <label class="field-group"
              ><span class="field-group__label"
                >Model <span class="field-group__hint"
                  >(blank = server default)</span
                ></span
              ><select bind:value={model} class="field"
                ><option value="">Inherit (system default)</option
                >{#each models as m (m)}<option value={m}>{m}</option>{/each
              }{#if model && !models.includes(model)}<option value={model}>{model}
                  (not in /models list)</option
                >{/if}</select
              ><span class="field-group__hint"
                >Saved on the global contract; repositories can override per
                repo.</span
              ></label
            >
            <label class="field-group"
              ><span class="field-group__label"
                >Temperature <span class="field-group__hint"
                  >(blank = server default)</span
                ></span
              ><input
                type="number"
                step="0.1"
                min="0"
                max="2"
                bind:value={temperature}
                class="field"
              /><span class="field-group__hint"
                >0 = deterministic, 2 = very creative.</span
              >{#if temperatureError}<span
                  class="field-group__error"
                  role="alert">{temperatureError}</span
                >{/if}</label
            >
          </div>
          <label class="field-group"
            ><span class="field-group__label">Mode</span
            ><select bind:value={mode} class="field"
              ><option value="lite">lite — commit log only</option
              ><option value="deep">deep — commit log + diff</option></select
            ><span class="field-group__hint"
              >deep inspects the code diff between tags; it costs more per
              release.</span
            ></label
          >
          <fieldset class="field-group" disabled={saving}>
            <legend class="field-group__label">Commit types</legend>
            <CommitTypeSelector
              bind:selected={selectedCommitTypes}
              bind:custom={customCommitTypes}
              hint={
                !commitTypesDirty && !settings?.commit_types?.trim()
                  ? "Default selection shown for new installs. Change a checkbox to save a filter."
                  : formatCommitTypes(selectedCommitTypes, customCommitTypes)
                    ? "Selected types are included in notes."
                    : "No filter saved — all commit types are included."
              }
              ariaLabel="Global commit types"
              onDirty={() => (commitTypesDirty = true)}
            />
          </fieldset>
          <div class="flex flex-wrap items-center gap-3">
            <button onclick={save} disabled={saving} class="btn btn-primary"
              >{saving ? "Saving…" : "Save contract"}</button
            >{#if saved}<span class="status status--healthy" role="status"
                ><span class="signal-dot signal-dot--healthy" aria-hidden="true"
                ></span>Saved</span
              >{/if}
          </div>
        </div>
      </section>

      <aside class="settings-proof">
        <section class="panel panel-soft" aria-label="Machine contract">
          <SectionHead
            label="Machine contract"
            title="Connection state"
            compact
            headingLevel="h2"
          >
            {#snippet actions()}
              <span class="signal-dot signal-dot--healthy" aria-hidden="true"
              ></span>
            {/snippet}
          </SectionHead>
          <dl class="contract-list">
            <div>
              <dt>Base URL</dt>
              <dd>{settings.llm.base_url}</dd>
            </div>
            <div>
              <dt>GitHub</dt>
              <dd>
                <span
                  class="status {settings.github
                    ? 'status--healthy'
                    : 'status--quiet'}"
                  ><span
                    class="signal-dot {settings.github
                      ? 'signal-dot--healthy'
                      : 'signal-dot--muted'}"
                    aria-hidden="true"
                  ></span>{settings.github
                    ? "Configured"
                    : "Not configured"}</span
                >
              </dd>
            </div>
            <div>
              <dt>Forgejo</dt>
              <dd>
                <span
                  class="status {settings.forgejo
                    ? 'status--healthy'
                    : 'status--quiet'}"
                  ><span
                    class="signal-dot {settings.forgejo
                      ? 'signal-dot--healthy'
                      : 'signal-dot--muted'}"
                    aria-hidden="true"
                  ></span>{settings.forgejo
                    ? "Configured"
                    : "Not configured"}</span
                >
              </dd>
            </div>
          </dl>
        </section>

        <section class="panel panel-soft" aria-label="Tone proof">
          <p class="trace-label">Tone proof</p>
          <h2 class="mt-1">Current contract at a glance</h2>
          <pre class="note-paper mt-4">Tone: {toneOption === "custom"
              ? customTone.trim() || "inherit"
              : toneOption === "inherit"
                ? "neutral"
                : toneOption}
Model: {model.trim() || "server default"}
Mode: {mode}
Temperature: {temperature !== "" ? temperature : "server default"}
Instructions: {instructions.trim() || "No additional instructions."}
Commit types: {!commitTypesDirty && !settings?.commit_types?.trim()
              ? `${DEFAULT_COMMIT_TYPES.join(",")} (default, not saved)`
              : formatCommitTypes(selectedCommitTypes, customCommitTypes) ||
                "all commit types"}</pre>
          <p class="mt-3 text-xs text-ink-3">
            This proof reflects the values in the form. Repository-level
            settings can override the global contract.
          </p>
        </section>
      </aside>
    </div>
  {/if}
</div>
