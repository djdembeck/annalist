<script lang="ts">
  let {
    label,
    heading,
    description,
    actionLabel,
    href,
    onClick,
    disabled = false
  }: {
    label: string;
    heading: string;
    description: string;
    actionLabel: string;
    href?: string;
    onClick?: () => void;
    disabled?: boolean;
  } = $props();
</script>

<section class="panel empty-state text-left">
  <p class="trace-label">{label}</p>
  <h2 class="mt-2">{heading}</h2>
  <p class="mt-2 max-w-xl text-sm text-ink-2">{description}</p>
  {#if href}
    <a href={href} class="btn btn-primary mt-5">{actionLabel}</a>
  {:else}
    <button class="btn btn-primary mt-5" onclick={onClick} disabled={disabled}>
      {actionLabel}
    </button>
  {/if}
</section>
