<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
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

  async function load(): Promise<void> {
    try {
      settings = await getSettings();
      const s = settings;
      const tone = s.tone ?? "";
      toneOption = !tone ? "inherit" : PRESET_OPTIONS.includes(tone) ? tone : "custom";
      customTone = PRESET_OPTIONS.includes(tone) ? "" : tone;
      instructions = s.instructions ?? "";
      model = s.model ?? "";
      temperature = s.temperature === null ? "" : String(s.temperature);
      error = "";
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error = e instanceof Error ? e.message : "Failed to load settings";
    } finally {
      loading = false;
    }
  }

  onMount(load);

  async function save(): Promise<void> {
    saved = false;
    saving = true;
    let tone: string | null;
    if (toneOption === "inherit") tone = null;
    else if (toneOption === "custom") tone = customTone.trim() ? customTone : null;
    else tone = toneOption;
    try {
      settings = await putSettings({
        tone,
        instructions: instructions.trim() ? instructions : null,
        model: model.trim() ? model : null,
        temperature: temperature === "" ? null : parseFloat(temperature) || null,
      });
      saved = true;
      error = "";
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error = e instanceof Error ? e.message : "Failed to save settings";
    } finally {
      saving = false;
    }
  }
</script>

<div class="trace-wall">
  <header class="section-head">
    <div>
      <p class="trace-label">Operations / global contract</p>
      <h1>Machine contract &amp; voice proof</h1>
      <p class="section-head__lede">Set the defaults that shape release notes, then verify the machine connection that will carry them.</p>
    </div>
    <a href="/repos" class="btn btn-secondary">Repository inventory</a>
  </header>

  <section class="panel panel-soft settings-trace" aria-label="Settings trace workflow">
    <div class="settings-node"><span class="signal-dot signal-dot--cyan" aria-hidden="true"></span><div><p class="trace-label">Contract</p><p class="settings-node__title">Global defaults</p></div></div>
    <div class="settings-trace__line" aria-hidden="true"></div>
    <div class="settings-node"><span class="signal-dot" aria-hidden="true"></span><div><p class="trace-label">Machine</p><p class="settings-node__title">Endpoint and sources</p></div></div>
    <div class="settings-trace__line" aria-hidden="true"></div>
    <div class="settings-node"><span class="signal-dot signal-dot--heat" aria-hidden="true"></span><div><p class="trace-label">Proof</p><p class="settings-node__title">What generation will use</p></div></div>
  </section>

  {#if error}
    <section class="panel panel--error" role="alert"><p class="trace-label">Contract update needs attention</p><p class="mt-2 text-sm text-alert">{error}</p></section>
  {/if}

  {#if loading}
    <section class="panel" aria-busy="true" aria-label="Loading settings"><p class="trace-label">Reading machine contract</p><div class="skeleton mt-4 h-12 w-full"></div><div class="skeleton mt-3 h-28 w-full"></div><div class="skeleton mt-3 h-12 w-1/2"></div></section>
  {:else if settings}
    <div class="settings-layout">
      <section class="panel settings-form" aria-label="Global release note defaults">
        <div class="section-head section-head--compact"><div><p class="trace-label">Voice contract</p><h2>Global defaults</h2></div><span class="status status--healthy"><span class="signal-dot signal-dot--cyan" aria-hidden="true"></span>Editable</span></div>
        <div class="grid gap-5">
          <label class="field-group"><span class="field-group__label">Tone</span><select bind:value={toneOption} class="field"><option value="inherit">Inherit (neutral)</option>{#each PRESET_OPTIONS as p (p)}<option value={p}>{p}</option>{/each}<option value="custom">Custom…</option></select><span class="field-group__hint">Choose the voice used when a repository does not override it.</span></label>
          {#if toneOption === "custom"}<label class="field-group"><span class="field-group__label">Custom tone</span><input bind:value={customTone} placeholder="Freeform persona" class="field" /></label>{/if}
          <label class="field-group"><span class="field-group__label">Instructions</span><textarea bind:value={instructions} rows="5" placeholder="Additional instructions for note generation" class="field"></textarea><span class="field-group__hint">These instructions travel with the global voice contract.</span></label>
          <div class="grid gap-5 sm:grid-cols-2">
            <label class="field-group"><span class="field-group__label">Model <span class="field-group__hint">(blank = server default)</span></span><input bind:value={model} class="field" /></label>
            <label class="field-group"><span class="field-group__label">Temperature <span class="field-group__hint">(blank = server default)</span></span><input type="number" step="0.1" min="0" max="2" bind:value={temperature} class="field" /><span class="field-group__hint">0 = deterministic, 2 = very creative.</span></label>
          </div>
          <div class="flex flex-wrap items-center gap-3"><button onclick={save} disabled={saving} class="btn btn-primary">{saving ? "Saving…" : "Save contract"}</button>{#if saved}<span class="status status--healthy" role="status"><span class="signal-dot signal-dot--healthy" aria-hidden="true"></span>Saved</span>{/if}</div>
        </div>
      </section>

      <aside class="settings-proof">
        <section class="panel panel-soft" aria-label="Machine contract">
          <div class="section-head section-head--compact"><div><p class="trace-label">Machine contract</p><h2>Connection state</h2></div><span class="signal-dot signal-dot--healthy" aria-hidden="true"></span></div>
          <dl class="contract-list">
            <div><dt>Base URL</dt><dd>{settings.llm.base_url}</dd></div>
            <div><dt>Model</dt><dd>{settings.llm.model}</dd></div>
            <div><dt>GitHub</dt><dd><span class="status {settings.github ? 'status--healthy' : 'status--quiet'}"><span class="signal-dot {settings.github ? 'signal-dot--healthy' : 'signal-dot--muted'}" aria-hidden="true"></span>{settings.github ? "Configured" : "Not configured"}</span></dd></div>
            <div><dt>Forgejo</dt><dd><span class="status {settings.forgejo ? 'status--healthy' : 'status--quiet'}"><span class="signal-dot {settings.forgejo ? 'signal-dot--healthy' : 'signal-dot--muted'}" aria-hidden="true"></span>{settings.forgejo ? "Configured" : "Not configured"}</span></dd></div>
          </dl>
        </section>

        <section class="panel panel-soft" aria-label="Voice proof">
          <p class="trace-label">Voice proof</p>
          <h2 class="mt-1">Current contract at a glance</h2>
          <pre class="note-paper mt-4">Tone: {toneOption === "custom" ? (customTone.trim() || "inherit") : toneOption === "inherit" ? "neutral" : toneOption}
Model: {model.trim() || settings.llm.model || "server default"}
Temperature: {temperature || "server default"}
Instructions: {instructions.trim() || "No additional instructions."}</pre>
          <p class="mt-3 text-xs text-ink-3">This proof reflects the values in the form. Repository-level settings can override the global contract.</p>
        </section>
      </aside>
    </div>
  {/if}
</div>
