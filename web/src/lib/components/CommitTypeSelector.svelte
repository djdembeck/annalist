<script lang="ts">
  import { COMMIT_TYPE_OPTIONS } from "$lib/commitTypes";

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

  function toggle(option: string, checked: boolean): void {
    selected = checked
      ? [...selected, option]
      : selected.filter((type) => type !== option);
    onDirty?.();
  }
</script>

{#if showOptions}
  <div class="commit-type-grid" aria-label={ariaLabel}>
    {#each COMMIT_TYPE_OPTIONS as option (option.value)}
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
        <span>
          <strong>{option.value}</strong>
          <small>{option.description}</small>
        </span>
      </label>
    {/each}
  </div>
  <label class="field-group__custom-types">
    <span class="field-group__hint">Additional types (optional)</span>
    <input
      bind:value={custom}
      spellcheck={false}
      placeholder="security,breaking"
      class="field"
    />
  </label>
{/if}
<span class="field-group__hint"
  >{hint} Breaking changes are always included.</span
>
