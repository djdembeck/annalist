<script lang="ts">
  import { COMMIT_TYPE_OPTIONS, DEFAULT_COMMIT_TYPES } from "$lib/commitTypes";

  let {
    selected = $bindable([]),
    custom = $bindable(""),
    hint,
    ariaLabel,
    showOptions = true,
  }: {
    selected?: string[];
    custom?: string;
    hint: string;
    ariaLabel: string;
    showOptions?: boolean;
  } = $props();

  const commonTypes = new Set(DEFAULT_COMMIT_TYPES);
  const commonOptions = COMMIT_TYPE_OPTIONS.filter((option) => commonTypes.has(option.value));
  const otherOptions = COMMIT_TYPE_OPTIONS.filter((option) => !commonTypes.has(option.value));
  const activeOtherTypes = $derived(selected.filter((value) => !commonTypes.has(value)));
  let expanded = $state(false);
  // Auto-open rule: the <details> stays open while an uncommon type is
  // selected or the custom field has text, so newly active values stay
  // visible (the open attribute below short-circuits manual collapse via
  // ||); once they clear, expanded holds the user's last toggle and
  // user collapse is respected.

  function toggle(option: string, checked: boolean): void {
    selected = checked
      ? [...selected, option]
      : selected.filter((type) => type !== option);
  }
</script>

{#snippet typeOption(option: (typeof COMMIT_TYPE_OPTIONS)[number])}
  <label
    class="check-control commit-type-option"
    class:commit-type-option--selected={selected.includes(option.value)}
  >
    <input
      class="check-input"
      type="checkbox"
      checked={selected.includes(option.value)}
      onchange={(event) => toggle(option.value, event.currentTarget.checked)}
    />
    <span><strong>{option.value}</strong><small>{option.description}</small></span>
  </label>
{/snippet}

{#if showOptions}
  <div class="commit-type-grid" role="group" aria-label={ariaLabel}>
    {#each commonOptions as option (option.value)}
      {@render typeOption(option)}
    {/each}
  </div>
  <details
    class="additional-types"
    open={expanded || activeOtherTypes.length > 0 || Boolean(custom.trim())}
    ontoggle={(event) => (expanded = event.currentTarget.open)}
  >
    <summary>Less-common and custom types{#if activeOtherTypes.length || custom.trim()}: {[...activeOtherTypes, custom.trim()].filter(Boolean).join(", ")}{/if}</summary>
    <div
      class="commit-type-grid"
      role="group"
      aria-label={`${ariaLabel}: less-common types`}
    >
      {#each otherOptions as option (option.value)}
        {@render typeOption(option)}
      {/each}
    </div>
    <label class="field-group__custom-types">
      <span class="field-group__hint">Additional types (optional)</span>
      <input bind:value={custom} placeholder="security,breaking" class="field" />
    </label>
  </details>
{/if}
<span class="field-group__hint"
  >{hint} Breaking changes are always included.</span
>

<style>
  .additional-types { margin-top: 8px; }
  /* min-height/padding/cursor come from the global summary utility; this
     component only adds wrap for long type strings and the open margin. */
  summary {
    overflow-wrap: anywhere;
  }
  details[open] > summary { margin-bottom: 8px; }
</style>
