<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { getSettings, putSettings, type Settings } from "$lib/api";

  const PRESET_OPTIONS = ["chronicler", "engineer", "launch"];

  let settings = $state<Settings | null>(null);
  let loading = $state(true);
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
      toneOption = !tone
        ? "inherit"
        : PRESET_OPTIONS.includes(tone)
          ? tone
          : "custom";
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
    let tone: string | null;
    if (toneOption === "inherit") {
      tone = null;
    } else if (toneOption === "custom") {
      tone = customTone.trim() ? customTone : null;
    } else {
      tone = toneOption;
    }
    try {
      settings = await putSettings({
        tone,
        instructions: instructions.trim() ? instructions : null,
        model: model.trim() ? model : null,
        temperature:
          temperature === "" ? null : parseFloat(temperature) || null,
      });
      saved = true;
      error = "";
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        goto("/setup");
        return;
      }
      error = e instanceof Error ? e.message : "Failed to save settings";
    }
  }
</script>

<div class="mx-auto max-w-2xl">
  <h1 class="mb-2 font-display text-3xl tracking-tight text-white">Settings</h1>
  <p class="mb-8 text-base text-ink-2">
    Global defaults used for any repository that doesn't override them.
  </p>

  {#if error}
    <p class="mb-4 text-sm text-alert">{error}</p>
  {/if}

  {#if loading}
    <p class="text-base text-ink-3">Loading…</p>
  {:else if settings}
    <div class="grid gap-6">
      <label class="flex flex-col gap-2">
        <span class="text-sm text-ink-2">Tone</span>
        <select
          bind:value={toneOption}
          class="rounded border border-line-strong bg-surface-1 px-4 py-2.5 text-base text-ink outline-none focus:border-focus focus:outline-2 focus:outline-focus-ring"
        >
          <option value="inherit">Inherit (neutral)</option>
          {#each PRESET_OPTIONS as p (p)}
            <option value={p}>{p}</option>
          {/each}
          <option value="custom">Custom…</option>
        </select>
      </label>

      {#if toneOption === "custom"}
        <label class="flex flex-col gap-2">
          <span class="text-sm text-ink-2">Custom tone</span>
          <input
            bind:value={customTone}
            placeholder="Freeform persona"
            class="rounded border border-line-strong bg-surface-1 px-4 py-2.5 text-base text-ink outline-none focus:border-focus focus:outline-2 focus:outline-focus-ring"
          />
        </label>
      {/if}

      <label class="flex flex-col gap-2">
        <span class="text-sm text-ink-2">Instructions</span>
        <textarea
          bind:value={instructions}
          rows="4"
          placeholder="Additional instructions for note generation"
          class="rounded border border-line-strong bg-surface-1 px-4 py-2.5 text-base text-ink outline-none focus:border-focus focus:outline-2 focus:outline-focus-ring"
        ></textarea>
      </label>

      <div class="grid gap-6 sm:grid-cols-2">
        <label class="flex flex-col gap-2">
          <span class="text-sm text-ink-2">Model (blank = server default)</span>
          <input
            bind:value={model}
            class="rounded border border-line-strong bg-surface-1 px-4 py-2.5 text-base text-ink outline-none focus:border-focus focus:outline-2 focus:outline-focus-ring"
          />
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-sm text-ink-2">Temperature (blank = server default)</span>
          <input
            type="number"
            step="0.1"
            min="0"
            max="2"
            bind:value={temperature}
            class="rounded border border-line-strong bg-surface-1 px-4 py-2.5 text-base text-ink outline-none focus:border-focus focus:outline-2 focus:outline-focus-ring"
          />
        </label>
      </div>

      <div class="flex items-center gap-4">
        <button
          onclick={save}
          class="rounded bg-mark bg-gradient-to-r from-cherry via-ember to-heat px-5 py-2.5 text-sm font-medium text-page hover:brightness-110 focus:outline-2 focus:outline-offset-2 focus:outline-focus-ring"
        >
          Save
        </button>
        {#if saved}
          <span class="inline-flex items-center gap-1.5 text-sm text-ok">
            <span class="h-2 w-2 rounded-full bg-ok"></span>
            Saved
          </span>
        {/if}
      </div>
    </div>

    <section class="mt-10 rounded border border-line bg-surface-2-warm p-5 text-sm">
      <h2 class="mb-3 font-sans text-base font-semibold text-ink">LLM endpoint</h2>
      <dl class="space-y-2 text-ink-2">
        <div class="flex justify-between gap-4">
          <dt>Base URL</dt>
          <dd class="text-ink">{settings.llm.base_url}</dd>
        </div>
        <div class="flex justify-between gap-4">
          <dt>Model</dt>
          <dd class="text-ink">{settings.llm.model}</dd>
        </div>
        <div class="flex justify-between gap-4">
          <dt>GitHub</dt>
          <dd class="inline-flex items-center gap-1.5 text-ink">
            {#if settings.github}
              <span class="h-2 w-2 rounded-full bg-ok"></span>
              Configured
            {:else}
              <span class="h-2 w-2 rounded-full bg-line-strong"></span>
              Not configured
            {/if}
          </dd>
        </div>
        <div class="flex justify-between gap-4">
          <dt>Forgejo</dt>
          <dd class="inline-flex items-center gap-1.5 text-ink">
            {#if settings.forgejo}
              <span class="h-2 w-2 rounded-full bg-ok"></span>
              Configured
            {:else}
              <span class="h-2 w-2 rounded-full bg-line-strong"></span>
              Not configured
            {/if}
          </dd>
        </div>
      </dl>
    </section>
  {/if}
</div>
