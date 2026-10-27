<script lang="ts">
  import { COMMIT_TYPE_OPTIONS, DEFAULT_COMMIT_TYPES } from "$lib/commitTypes";

  let {
    selected = $bindable([]),
    custom = $bindable(""),
    hint,
    ariaLabel,
    showOptions = true,
    onDirty
  }: {
    selected?: string[];
    custom?: string;
    hint: string;
    ariaLabel: string;
    showOptions?: boolean;
    onDirty?: () => void;
  } = $props();

  const commonTypes = new Set(DEFAULT_COMMIT_TYPES);
  const commonOptions = COMMIT_TYPE_OPTIONS.filter((option) => commonTypes.has(option.value));
  const otherOptions = COMMIT_TYPE_OPTIONS.filter((option) => !commonTypes.has(option.value));
  const activeOtherTypes = $derived(selected.filter((value) => !commonTypes.has(value)));
  let expanded = $state(false);

  function toggle(option: string, checked: boolean): void {
    selected = checked
      ? [...selected, option]
      : selected.filter((type) => type !== option);
    onDirty?.();
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
  <div class="commit-type-grid" aria-label={ariaLabel}>
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
    <div class="commit-type-grid" aria-label={`${ariaLabel}: less-common types`}>
      {#each otherOptions as option (option.value)}
        {@render typeOption(option)}
      {/each}
    </div>
    <label class="field-group__custom-types">
      <span class="field-group__hint">Additional types (optional)</span>
      <input
        bind:value={custom}
        oninput={() => onDirty?.()}
        placeholder="security,breaking"
        class="field"
      />
    </label>
  </details>
{/if}
<span class="field-group__hint"
  >{hint} Breaking changes are always included.</span
>

<style>
  .additional-types { margin-top: 8px; }
  summary {
    min-height: 44px;
    padding-block: 10px;
    cursor: pointer;
    overflow-wrap: anywhere;
  }
  details[open] > summary { margin-bottom: 8px; }
</style>
