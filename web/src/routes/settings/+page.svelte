<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { parseTemperature, resolveTone } from "$lib/repoUtils";
  import { getSettings, putSettings, type Settings } from "$lib/api";

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
  let temperature = $state("");
  let temperatureError = $state("");

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
      temperature = s.temperature === null ? "" : String(s.temperature);
      temperatureError = "";
      error = "";
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error =
        e instanceof Error && e.message !== "Unauthorized"
          ? e.message
          : "Could not load settings — the server may be unreachable. Check your connection and try refreshing.";
    } finally {
      loading = false;
    }
  }

  onMount(load);

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
      settings = await putSettings({
        tone,
        instructions: instructions.trim() ? instructions : null,
        model: model.trim() ? model : null,
        temperature: tempResult.value,
      });
      saved = true;
      temperatureError = "";
      error = "";
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error =
        e instanceof Error && e.message !== "Unauthorized"
          ? e.message
          : "Failed to save settings — check your connection and try again.";
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
  <header class="section-head">
    <div>
      <p class="trace-label">Operations / global contract</p>
      <h1>Machine contract &amp; tone proof</h1>
      {#if settings}
        <p class="section-head__lede">
          {#if settings.github && settings.forgejo}Both GitHub and Forgejo are
            configured.{:else if settings.github}GitHub configured, Forgejo not
            configured.{:else if settings.forgejo}Forgejo configured, GitHub not
            configured.
          {:else}No platforms configured yet — add a platform below to start.{/if}
          <span class="text-ink-2"> · Model {settings.llm.model}</span>
        </p>
      {/if}
    </div>
    <a href="/repos" class="btn btn-secondary">Repository inventory</a>
  </header>

  {#if error}
    <section class="panel panel--error" role="alert">
      <p class="trace-label">Contract update needs attention</p>
      <p class="mt-2 text-sm text-alert">{error}</p>
    </section>
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
        <div class="section-head section-head--compact">
          <div>
            <p class="trace-label">Tone contract</p>
            <h2>Global defaults</h2>
          </div>
          <span class="status status--healthy"
            ><span class="signal-dot signal-dot--muted" aria-hidden="true"
            ></span>Editable</span
          >
        </div>
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
              ><input bind:value={model} class="field" /></label
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
          <div class="section-head section-head--compact">
            <div>
              <p class="trace-label">Machine contract</p>
              <h2>Connection state</h2>
            </div>
            <span class="signal-dot signal-dot--healthy" aria-hidden="true"
            ></span>
          </div>
          <dl class="contract-list">
            <div>
              <dt>Base URL</dt>
              <dd>{settings.llm.base_url}</dd>
            </div>
            <div>
              <dt>Model</dt>
              <dd>{settings.llm.model}</dd>
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
Model: {model.trim() || settings.llm.model || "server default"}
Temperature: {temperature !== "" ? temperature : "server default"}
Instructions: {instructions.trim() || "No additional instructions."}</pre>
          <p class="mt-3 text-xs text-ink-3">
            This proof reflects the values in the form. Repository-level
            settings can override the global contract.
          </p>
        </section>
      </aside>
    </div>
  {/if}
</div>
