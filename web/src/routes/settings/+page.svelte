<script lang="ts">
  import { onMount, tick } from "svelte";
  import { beforeNavigate } from "$app/navigation";
  import {
    parseMaxTokens,
    parseTemperature,
    resolveTone,
  } from "$lib/repoUtils";
  import {
    DEFAULT_COMMIT_TYPES,
    formatCommitTypes,
    getCommitTypeSelection,
    isKeepAll,
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
  const THINKING_LEVELS = ["low", "medium", "high"];

  let settings = $state<Settings | null>(null);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state("");
  let sessionExpired = $state(false);
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
  let temperature = $state<string | number | undefined>("");
  let temperatureError = $state("");
  let maxTokens = $state<string | number | null | undefined>("");
  let maxTokensError = $state("");
  let thinkingLevel = $state("");
  let selectedCommitTypes = $state<string[]>(DEFAULT_COMMIT_TYPES);
  let customCommitTypes = $state("");
  let baseline = $state("");
  let baselineCommitTypes = $state("");
  let providerOpen = $state(false);
  let tuningOpen = $state(false);
  let form = $state<HTMLFormElement>();
  let temperatureInput = $state<HTMLInputElement>();
  let maxTokensInput = $state<HTMLInputElement>();
  const commitTypeValue = $derived(
    formatCommitTypes(selectedCommitTypes, customCommitTypes),
  );
  const commitTypesDirty = $derived(commitTypeValue !== baselineCommitTypes);

  function snapshot(): string {
    return JSON.stringify({
      tone: resolveTone(toneOption, customTone),
      instructions: instructions.trim() ? instructions : "",
      model: model.trim(), baseUrl: baseUrl.trim(), mode,
      temperature: temperature == null || temperature === "" ? "" : String(temperature),
      maxTokens: maxTokens == null || maxTokens === "" ? "" : String(maxTokens),
      thinkingLevel, commitTypeValue,
      apiKeyTouched: apiKeyTouched && (apiKey !== "" || Boolean(settings?.llm.has_key)), apiKey,
    });
  }
  const dirty = $derived(Boolean(baseline) && snapshot() !== baseline);

  beforeNavigate((navigation) => {
    // Browser exits use beforeunload; SPA transitions use the same dirty state.
    if (navigation.willUnload || !dirty) return;
    if (
      navigation.from?.url.pathname === navigation.to?.url.pathname &&
      navigation.from?.url.search === navigation.to?.url.search &&
      navigation.from?.url.hash !== navigation.to?.url.hash
    ) return;
    if (!window.confirm("Discard unsaved settings and leave this page?")) {
      navigation.cancel();
    }
  });

  function beforeUnload(event: BeforeUnloadEvent): void {
    if (!dirty) return;
    event.preventDefault();
    event.returnValue = "";
  }

  function discard(): void {
    if (!settings || saving) return;
    applySettings(settings);
    sessionExpired = false;
    saved = false;
    error = "";
  }

  let platformStatus = $derived(
    settings
      ? settings.github && settings.forgejo
        ? "Both GitHub and Forgejo are configured."
        : settings.github
          ? "GitHub configured, Forgejo not configured."
          : settings.forgejo
            ? "Forgejo configured, GitHub not configured."
            : "No platforms configured yet. Configure GitHub or Forgejo on the server, then return to setup."
      : undefined,
  );
  let headerLede = $derived(settings ? platformStatus : undefined);

  function applySettings(s: Settings): void {
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
      maxTokens =
        s.max_tokens === 0 || s.max_tokens == null ? "" : String(s.max_tokens);
      thinkingLevel =
        s.thinking_level === "off" ? "" : (s.thinking_level ?? "");
      const commitTypeSelection = getCommitTypeSelection(s.commit_types);
      const hasSavedCommitTypes = Boolean(s.commit_types?.trim());
      selectedCommitTypes = hasSavedCommitTypes
        ? commitTypeSelection.selected
        : [...DEFAULT_COMMIT_TYPES];
      customCommitTypes = commitTypeSelection.custom.join(", ");
      baselineCommitTypes = formatCommitTypes(selectedCommitTypes, customCommitTypes);
      temperatureError = "";
      maxTokensError = "";
      baseline = snapshot();
  }

  function recoverAuth(e: unknown): boolean {
    return handleAuthError(e, dirty ? () => {
      sessionExpired = true;
      saved = false;
    } : undefined);
  }

  async function load(): Promise<void> {
    try {
      settings = await getSettings();
      applySettings(settings);
      error = "";
    } catch (e) {
      if (recoverAuth(e)) return;
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
      sessionExpired = false;
    } catch (e) {
      if (recoverAuth(e)) return;
      modelsError = formatError(
        e,
        "Could not load the model list — check the base URL and API key below.",
      );
      providerOpen = true;
    }
  }

  async function save(): Promise<void> {
    if (saving || !dirty || !form) return;
    saved = false;
    temperatureError = "";
    maxTokensError = "";
    const tempResult = parseTemperature(temperature == null ? "" : String(temperature));
    const tokensResult = parseMaxTokens(maxTokens ?? null);
    if (tempResult.error || tokensResult.error) {
      temperatureError = tempResult.error ?? "";
      maxTokensError = tokensResult.error ?? "";
      tuningOpen = true;
      await tick();
      (tempResult.error ? temperatureInput : maxTokensInput)?.focus();
      return;
    }
    const invalid = form.querySelector<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>(
      "input:invalid, select:invalid, textarea:invalid",
    );
    if (invalid) {
      const disclosure = invalid.closest("details");
      if (disclosure) disclosure.open = true;
      invalid.reportValidity();
      return;
    }
    saving = true;
    const tone = resolveTone(toneOption, customTone);
    try {
      const payload: SettingsUpdate = {
        tone,
        instructions: instructions.trim() ? instructions : null,
        model: model.trim() ? model : null,
        mode,
        temperature: tempResult.value,
        max_tokens: tokensResult.value,
        // The UI's off option (value "") sends the literal "off" so it is
        // stored as a value that suppresses a level set via config/env;
        // "" (inherit/clear) is the empty string.
        thinking_level: thinkingLevel === "" ? "off" : thinkingLevel,
      };
      if (commitTypesDirty) {
        payload.commit_types = formatCommitTypes(
          selectedCommitTypes,
          customCommitTypes,
        );
      }
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
      applySettings(settings);
      sessionExpired = false;
      saved = true;
      temperatureError = "";
      maxTokensError = "";
      error = "";
    } catch (e) {
      if (recoverAuth(e)) return;
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
    content="Configure global release-note defaults and the model provider."
  />
</svelte:head>

<svelte:window onbeforeunload={beforeUnload} />

<div class="workspace">
  <SectionHead title="Settings" lede={headerLede}>
    {#snippet actions()}
      <a href="/repos" class="btn btn-secondary">Repositories</a>
    {/snippet}
  </SectionHead>

  {#if sessionExpired}
    <section class="banner banner-error" role="alert">
      <p>Your session expired. Your unsaved settings are still here. Reauthenticate in setup in a new tab, then return here and retry Save settings or Refresh models. Keep this tab open to preserve your draft.</p>
      <a href="/setup" target="_blank" rel="noopener" class="btn btn-secondary mt-4">Reauthenticate in setup (new tab)</a>
    </section>
  {/if}

  {#if error}
    <ErrorBanner message={error} />
  {/if}

  {#if loading}
    <section class="panel" aria-busy="true" aria-label="Loading settings">
      <p class="field-caption">Loading settings…</p>
      <div class="skeleton mt-4 h-12 w-full"></div>
      <div class="skeleton mt-3 h-28 w-full"></div>
      <div class="skeleton mt-3 h-12 w-1/2"></div>
    </section>
  {:else if settings}
    <div class="settings-layout">
      <form
        class="panel settings-form"
        aria-label="Global release note defaults"
        bind:this={form}
        novalidate
        onsubmit={(event) => { event.preventDefault(); save(); }}
      >
        <div id="settings-actions" class="settings-actions">
          <span role="status">{saving ? "Saving settings…" : dirty ? (error ? "Not saved — review the error and try again." : "Unsaved changes") : saved ? "Settings saved" : "No unsaved changes"}</span>
          <div class="settings-actions__buttons">
            <button type="button" onclick={discard} disabled={saving || !dirty} class="btn btn-secondary">Discard</button>
            <button type="submit" disabled={saving || !dirty} class="btn btn-primary">{saving ? "Saving…" : "Save settings"}</button>
            <a href="#settings-summary" class="btn btn-ghost">Form summary</a>
          </div>
        </div>
        <fieldset class="settings-fields" disabled={saving}>
        <SectionHead title="Global defaults" compact headingLevel="h2">
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
              >When set, these instructions replace the default writing
              instructions. Repository instructions take priority.</span
            ></label
          >
          <details class="settings-disclosure" bind:open={providerOpen}>
            <summary>Provider connection and API key</summary>
        <SectionHead title="Connection" compact headingLevel="h2">
          {#snippet actions()}
            <button class="btn btn-secondary" type="button" onclick={loadModels}>
              Refresh models
            </button>
          {/snippet}
        </SectionHead>
        {#if modelsError}
          <ErrorBanner
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
              class="field"
            /><span class="field-group__hint"
              >OpenAI-compatible endpoint (scheme://host[:port]). Overrides the
              LLM_BASE_URL env value while set; clear the field to fall back to
              env.</span
            ></label
          >
          <label class="field-group"
            ><span class="field-group__label"
              >API key
              {#if settings?.llm.has_key}
                <span class="field-group__hint"
                  >(saved or environment key configured)</span
                >
              {:else}
                <span class="field-group__hint">(none set)</span>
              {/if}</span
            ><input
              bind:value={apiKey}
              type="password"
              oninput={() => (apiKeyTouched = true)}
              placeholder={settings?.llm.has_key
                ? "Leave blank to keep current key"
                : "sk-..."}
              class="field"
            /><span class="field-group__hint"
              >Overrides LLM_API_KEY while non-empty; a blanked, saved key is
              cleared on Save.</span
            >{#if apiKeyTouched && apiKey === "" && settings?.llm.has_key}
              <span class="field-group__hint" role="status"
                >Blank + save = clear the stored key.</span
              >
            {/if}</label
          >
        </div>

          </details>
          <details class="settings-disclosure" bind:open={tuningOpen}>
            <summary>Advanced model tuning</summary>
          <div class="grid gap-5 sm:grid-cols-4">
            <label class="field-group"
              ><span class="field-group__label"
                >Model <span class="field-group__hint"
                  >(blank = server default)</span
                ></span
              ><select bind:value={model} class="field"
                ><option value="">Inherit (system default)</option
                >{#each models as m (m)}<option value={m}>{m}</option
                  >{/each}{#if model && !models.includes(model)}<option
                    value={model}
                    >{model}
                    (not in /models list)</option
                  >{/if}</select
              ><span class="field-group__hint"
                >Saved on the global defaults; repositories can override per
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
                step="any"
                min="0"
                max="2"
                bind:this={temperatureInput}
                bind:value={temperature}
                class="field"
              /><span class="field-group__hint"
                >0 = deterministic, 2 = very creative.</span
              >{#if temperatureError}<span
                  class="field-group__error"
                  role="alert">{temperatureError}</span
                >{/if}</label
            >
            <label class="field-group"
              ><span class="field-group__label"
                >Max output tokens <span class="field-group__hint"
                  >(blank = server default)</span
                ></span
              ><input
                type="number"
                step="1"
                min="1"
                bind:this={maxTokensInput}
                bind:value={maxTokens}
                class="field"
              /><span class="field-group__hint"
                >Cap on generated tokens per request (blank = server default;
                whole number of 1 or more).</span
              >{#if maxTokensError}<span class="field-group__error" role="alert"
                  >{maxTokensError}</span
                >{/if}</label
            >
            <label class="field-group"
              ><span class="field-group__label">Thinking level</span><select
                bind:value={thinkingLevel}
                class="field"
                ><option value="">off — no extended thinking</option
                >{#each THINKING_LEVELS as lvl (lvl)}<option value={lvl}
                    >{lvl}</option
                  >{/each}</select
              ><span class="field-group__hint"
                >Sent to the model as reasoning_effort; off sends "none" so
                extended thinking stays off. Only works on endpoints that
                support it.</span
              ></label
            >
          </div>
          </details>
          <label class="field-group"
            ><span class="field-group__label">Mode</span><select
              bind:value={mode}
              class="field"
              ><option value="lite">lite — commit log only</option><option
                value="deep">deep — commit log + diff</option
              ></select
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
              hint={!commitTypesDirty && !settings?.commit_types?.trim()
                ? "Default selection shown for new installs. Change a checkbox to save a filter."
                : isKeepAll(
                      formatCommitTypes(selectedCommitTypes, customCommitTypes),
                    )
                  ? "No filter — all commit types are included."
                  : "Selected types are included in notes."}
              ariaLabel="Global commit types"
            />
          </fieldset>
          <a href="#settings-actions" class="btn btn-secondary">Back to Save and Discard</a>
        </div>
        </fieldset>
      </form>

      <aside class="settings-proof">
        <section class="panel panel-soft" aria-label="Server configuration">
          <SectionHead title="Connection state" compact headingLevel="h2"
          ></SectionHead>
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
                  >{settings.github ? "Configured" : "Not configured"}</span
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
                  >{settings.forgejo ? "Configured" : "Not configured"}</span
                >
              </dd>
            </div>
          </dl>
        </section>

        <section id="settings-summary" class="panel panel-soft" aria-label="Form summary" tabindex="-1">
          <h2>Form summary</h2>
          <pre class="note-paper mt-4">Tone: {toneOption === "custom"
              ? customTone.trim() || "inherit"
              : toneOption === "inherit"
                ? "neutral"
                : toneOption}
Model: {model.trim() || "server default"}
Mode: {mode}
Temperature: {temperature != null && temperature !== "" ? temperature : "server default"}
Max tokens: {maxTokens || "server default"}
Thinking: {thinkingLevel || "off"}
Instructions: {instructions.trim() || "No additional instructions."}
Commit types: {!commitTypesDirty && !settings?.commit_types?.trim()
              ? `${DEFAULT_COMMIT_TYPES.join(",")} (default, not saved)`
              : isKeepAll(
                    formatCommitTypes(selectedCommitTypes, customCommitTypes),
                  )
                ? "all commit types (no filter)"
                : formatCommitTypes(
                    selectedCommitTypes,
                    customCommitTypes,
                  )}</pre>
          <a href="#settings-actions" class="btn btn-secondary mt-3">Back to Save and Discard</a>
          <p class="mt-3 text-xs text-ink-3">
            This summary reflects the values in the form. Repository-level
            settings can override the global defaults.
          </p>
        </section>
      </aside>
    </div>
  {/if}
</div>

<style>
  .settings-fields {
    border: 0;
    padding: 0;
    margin: 0;
    min-width: 0;
  }
  .settings-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding-bottom: 20px;
    border-bottom: 1px solid var(--line);
    margin-bottom: 24px;
  }
  .settings-actions__buttons {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .settings-disclosure {
    border-top: 1px solid var(--line);
    padding-top: 8px;
  }
  .settings-disclosure > summary {
    min-height: 44px;
    padding-block: 10px;
    cursor: pointer;
    font-weight: 600;
  }
  .settings-disclosure[open] > summary { margin-bottom: 16px; }
  #settings-summary { scroll-margin-top: 24px; }
</style>
