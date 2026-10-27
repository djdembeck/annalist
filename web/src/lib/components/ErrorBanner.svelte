<script lang="ts">
  let {
    message,
    variant = "error",
    actionLabel,
    onAction,
  }: {
    message: string;
    variant?: "error" | "warning";
    actionLabel?: string;
    onAction?: () => void;
  } = $props();
</script>

<section
  class="banner banner-{variant}"
  role={variant === "error" ? "alert" : "status"}
>
  <p>{message}</p>
  {#if actionLabel}<button
      type="button"
      class="btn btn-secondary mt-4"
      onclick={onAction}>{actionLabel}</button
    >{/if}
</section>
