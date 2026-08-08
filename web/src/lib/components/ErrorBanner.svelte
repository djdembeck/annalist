<script lang="ts">
  let {
    label,
    message,
    detail,
    variant = "error",
    actionLabel,
    onAction,
    disabled = false
  }: {
    label: string;
    message: string;
    detail?: string;
    variant?: "error" | "info" | "warning";
    actionLabel?: string;
    onAction?: () => void;
    disabled?: boolean;
  } = $props();

  let cls = $derived(variant === "error" ? "panel panel--error" : "panel");
  let role = $derived(variant === "error" ? "alert" : "status");
  let style = $derived(variant === "warning" ? "border-color: var(--trace-heat)" : undefined);
</script>

<section class={cls} role={role} style={style}>
  <p class="trace-label">{label}</p>
  <p class="mt-2 text-sm text-alert">{message}</p>
  {#if detail}
    <p class="mt-1 text-sm text-ink-2">{detail}</p>
  {/if}
  {#if actionLabel}
    <button class="btn btn-secondary mt-4" onclick={onAction} disabled={disabled}>
      {actionLabel}
    </button>
  {/if}
</section>