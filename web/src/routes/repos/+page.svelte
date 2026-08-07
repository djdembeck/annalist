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
  let pending = $state<Record<string, { saving?: boolean; toggling?: boolean; generating?: boolean }>>({});
  let saveMsg = $state<Record<string, string | null>>({});
  let saveErr = $state<Record<string, string | null>>({});
  let toggleErr = $state<Record<string, string | null>>({});
  let openPanel = $state<Record<string, "settings" | "generate" | undefined>>({});
  let drafts = $state<Record<string, Draft>>({});
  let force = $state<Record<string, boolean>>({});
  let notesOut = $state<Record<string, string | null>>({});
  let genError = $state<Record<string, string | null>>({});
  let generateTag = $state<Record<string, string>>({});
  let genTagError = $state<Record<string, string | null>>({});

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
      await putRepoSettings(r.platform, r.owner, r.repo, { enabled: nextEnabled });
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

  function openGenerate(r: Repo): void {
    const key = rowKey(r);
    generateTag[key] ??= "";
    openPanel[key] = openPanel[key] === "generate" ? undefined : "generate";
  }

  async function saveSettings(r: Repo): Promise<void> {
    const key = rowKey(r);
    const d = drafts[key];
    saveErr[key] = null;
    saveMsg[key] = null;
    pending[key] = { ...(pending[key] ?? {}), saving: true };
    let tone: string | null;
    if (d.toneOption === "inherit") tone = null;
    else if (d.toneOption === "custom") tone = d.customTone.trim() ? d.customTone : null;
    else tone = d.toneOption;
    try {
      await putRepoSettings(r.platform, r.owner, r.repo, {
        tone,
        instructions: d.instructions.trim() ? d.instructions : null,
        model: d.model.trim() ? d.model : null,
        temperature: d.temperature === "" ? null : parseFloat(d.temperature) || null,
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

  async function runGenerate(r: Repo): Promise<void> {
    const key = rowKey(r);
    const toTag = (generateTag[key] ?? "").trim();
    notesOut[key] = null;
    genError[key] = null;
    genTagError[key] = null;
    if (!toTag) {
      genTagError[key] = "Enter a release tag";
      return;
    }
    pending[key] = { ...pending[key], generating: true };
    try {
      const result = await generate(r.platform, r.owner, r.repo, { to_tag: toTag, force: force[key] ?? false });
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

<div class="trace-wall">
  <header class="section-head">
    <div>
      <p class="trace-label">Operations / connected sources</p>
      <h1>Repository trace inventory</h1>
      <p class="section-head__lede">Keep every connected release source legible: its switch, voice contract, and note action stay on one workpiece.</p>
    </div>
    <div class="section-head__actions">
      {#if loadState === "success" || loadState === "error"}
        <button onclick={refresh} class="btn btn-secondary" aria-label="Refresh repository inventory">Refresh</button>
      {/if}
      <a href="/repos/add" class="btn btn-primary">Add repositories</a>
    </div>
  </header>

  <section class="panel panel-soft repo-trace" aria-label="Release trace workflow">
    <div class="repo-trace-node"><span class="signal-dot" aria-hidden="true"></span><div><p class="trace-label">Source</p><p class="repo-trace-node__title">Connected repositories</p></div></div>
    <div class="repo-trace__line" aria-hidden="true"></div>
    <div class="repo-trace-node"><span class="signal-dot signal-dot--cyan" aria-hidden="true"></span><div><p class="trace-label">Shape</p><p class="repo-trace-node__title">Voice and trigger</p></div></div>
    <div class="repo-trace__line" aria-hidden="true"></div>
    <div class="repo-trace-node"><span class="signal-dot signal-dot--heat" aria-hidden="true"></span><div><p class="trace-label">Proof</p><p class="repo-trace-node__title">Generated release note</p></div></div>
  </section>

  {#if loadState === "loading" || loadState === "idle"}
    <section class="panel" aria-busy="true" aria-label="Loading repositories">
      <p class="trace-label">Reading source inventory</p>
      <div class="skeleton mt-4 h-12 w-full"></div><div class="skeleton mt-3 h-24 w-full"></div><div class="skeleton mt-3 h-24 w-full"></div>
    </section>
  {:else if loadState === "error"}
    <section class="panel panel--error" role="alert">
      <p class="trace-label">Source read failed</p><p class="mt-2 text-sm text-alert">Couldn't load repositories.</p><p class="mt-1 text-sm text-ink-2">{loadError}</p>
      <button onclick={refresh} class="btn btn-secondary mt-4">Retry</button>
    </section>
  {:else if repos.length === 0}
    <section class="panel empty-state text-left">
      <p class="trace-label">No connected sources</p><h2 class="mt-2">The trace starts with a repository.</h2>
      <p class="mt-2 max-w-xl text-sm text-ink-2">Connect a GitHub or Forgejo repository to put release commits on the wall and shape the first note.</p>
      <a href="/repos/add" class="btn btn-primary mt-5">Add your first repository</a>
    </section>
  {:else}
    <section class="console-grid repo-inventory" aria-label="Connected repository inventory">
      {#each repos as r (rowKey(r))}
        {@const key = rowKey(r)}
        <article class="panel repo-workpiece">
          <header class="repo-workpiece__header">
            <div class="min-w-0"><div class="flex flex-wrap items-center gap-2"><span class="chip">{r.platform}</span><span class="status {r.enabled ? 'status--healthy' : 'status--quiet'}"><span class="signal-dot {r.enabled ? '' : 'signal-dot--muted'}" aria-hidden="true"></span>{r.enabled ? "Enabled" : "Disabled"}</span></div><h2 class="repo-workpiece__name">{r.owner}/{r.repo}</h2></div>
            <label class="repo-switch"><span class="trace-label">In trace</span><input type="checkbox" aria-label="Enable {r.owner}/{r.repo}" checked={r.enabled} disabled={pending[key]?.toggling} onchange={() => toggleEnabled(r)} /></label>
          </header>

          <div class="repo-workpiece__details"><div><p class="trace-label">Effective voice</p><p class="mt-1 text-sm text-ink">{r.effective.tone ?? "neutral"}</p></div><div><p class="trace-label">Model</p><p class="mt-1 break-all text-sm text-ink">{r.effective.model ?? "inherit"}</p></div><div><p class="trace-label">Trigger</p><p class="mt-1 text-sm text-ink">{r.trigger ?? "auto"}</p></div></div>
          {#if toggleErr[key]}<p class="mt-3 text-xs text-alert" role="alert">{toggleErr[key]}</p>{/if}

          <div class="repo-workpiece__actions">
            <button onclick={() => openGenerate(r)} aria-expanded={openPanel[key] === "generate"} aria-controls="repo-generate-{key}" class="btn btn-primary">{openPanel[key] === "generate" ? "Close generation" : "Generate note"}</button>
            <button onclick={() => openSettings(r)} aria-expanded={openPanel[key] === "settings"} aria-controls="repo-settings-{key}" class="btn btn-secondary">{openPanel[key] === "settings" ? "Close settings" : "Settings"}</button>
          </div>

          {#if openPanel[key] === "settings"}
            {@const d = drafts[key]}
            <section id="repo-settings-{key}" class="trace-panel" aria-label="Settings for {r.owner}/{r.repo}">
              <p class="trace-label">Voice contract / {r.owner}/{r.repo}</p>
              <div class="grid gap-4 md:grid-cols-2">
                <label class="field-group"><span class="field-group__label">Tone</span><select bind:value={d.toneOption} class="field"><option value="inherit">Inherit</option>{#each PRESET_OPTIONS as p (p)}<option value={p}>{p}</option>{/each}</select><span class="field-group__hint">Use a preset or choose a custom persona.</span></label>
                {#if d.toneOption === "custom"}<label class="field-group"><span class="field-group__label">Custom tone</span><input bind:value={d.customTone} placeholder="Freeform persona" class="field" /></label>{/if}
                <label class="field-group md:col-span-2"><span class="field-group__label">Instructions</span><textarea bind:value={d.instructions} rows="3" class="field"></textarea><span class="field-group__hint">Extra guidance the writer follows for this repository.</span></label>
                <label class="field-group"><span class="field-group__label">Model <span class="field-group__hint">(blank = inherit)</span></span><input bind:value={d.model} class="field" /></label>
                <label class="field-group"><span class="field-group__label">Temperature <span class="field-group__hint">(blank = inherit)</span></span><input type="number" step="0.1" min="0" max="2" bind:value={d.temperature} class="field" /><span class="field-group__hint">0 = deterministic, 2 = very creative.</span></label>
                <label class="field-group"><span class="field-group__label">Trigger</span><select bind:value={d.trigger} class="field"><option value="auto">auto</option><option value="manual">manual</option></select><span class="field-group__hint">Auto runs on release webhooks; manual disables webhooks.</span></label>
              </div>
              <p class="mt-4 text-xs text-ink-3">Effective: tone <span class="text-ink">{r.effective.tone ?? "neutral"}</span> · model <span class="text-ink">{r.effective.model ?? "inherit"}</span> · temperature <span class="text-ink">{r.effective.temperature ?? "inherit"}</span></p>
              <div class="mt-4 flex flex-wrap items-center gap-3"><button onclick={() => saveSettings(r)} disabled={pending[key]?.saving} class="btn btn-primary">{pending[key]?.saving ? "Saving…" : "Save settings"}</button><span class="text-xs text-ink-3">Save before generating; the writer uses the saved voice.</span></div>
              <div aria-live="polite" class="mt-3">{#if saveMsg[key]}<p class="text-sm text-ok">{saveMsg[key]}</p>{/if}{#if saveErr[key]}<p class="text-sm text-alert" role="alert">{saveErr[key]}</p>{/if}</div>
            </section>
          {/if}

          {#if openPanel[key] === "generate"}
            <section id="repo-generate-{key}" class="trace-panel" aria-label="Generate a note for {r.owner}/{r.repo}">
              <p class="trace-label">Note proof / {r.owner}/{r.repo}</p><p class="mt-2 max-w-2xl text-sm text-ink-2">Generate a note for a release tag. A normal run preserves an existing hand-edited note; overwrite replaces it.</p>
              <div class="mt-4 grid gap-3 sm:grid-cols-2" aria-label="Generate mode for {r.owner}/{r.repo}">
                <button type="button" aria-pressed={!force[key]} onclick={() => (force[key] = false)} class="mode-choice {!force[key] ? 'mode-choice--active' : ''}"><span class="flex items-center gap-2 text-sm font-medium text-ink"><span class="signal-dot {!force[key] ? 'signal-dot--heat' : 'signal-dot--muted'}" aria-hidden="true"></span>Generate</span><span class="mt-1 text-xs leading-relaxed text-ink-3">Reuse the note already on file and never overwrite a release body edited by hand.</span></button>
                <button type="button" aria-pressed={force[key] ?? false} onclick={() => (force[key] = true)} class="mode-choice {force[key] ? 'mode-choice--danger' : ''}"><span class="flex items-center gap-2 text-sm font-medium text-ink"><span class="signal-dot {force[key] ? 'signal-dot--error' : 'signal-dot--muted'}" aria-hidden="true"></span>Overwrite</span><span class="mt-1 text-xs leading-relaxed text-ink-3">Throw out the existing note and run the writer again, even for a hand-edited body.</span></button>
              </div>
              <div class="mt-4 flex flex-wrap items-end gap-3"><label class="field-group min-w-0 flex-1 sm:max-w-xs"><span class="field-group__label">Release tag</span><input bind:value={generateTag[key]} placeholder="v1.0.0" class="field" />{#if genTagError[key]}<span class="field-group__error" role="alert">{genTagError[key]}</span>{/if}</label><button onclick={() => runGenerate(r)} disabled={pending[key]?.generating} class="btn btn-primary w-full sm:w-auto">{pending[key]?.generating ? "Generating…" : force[key] ? "Overwrite" : "Generate"}</button></div>
              <div aria-live="polite" class="mt-4">{#if genError[key]}<p class="text-sm text-alert" role="alert">{genError[key]}</p>{/if}{#if notesOut[key]}<div class="mt-4"><p class="trace-label">Generated note / release proof</p><pre class="note-paper mt-2 max-h-96 overflow-auto">{notesOut[key]}</pre></div>{/if}</div>
            </section>
          {/if}
        </article>
      {/each}
    </section>
  {/if}
</div>
