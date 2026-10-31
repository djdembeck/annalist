<script lang="ts">
  let {
    message,
    detail,
    variant = "error",
    actionLabel,
    onAction,
  }: {
    message: string;
    detail?: string;
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
  {#if detail}<p class="banner-detail">{detail}</p>{/if}
  {#if actionLabel}<button
      type="button"
      class="btn btn-secondary mt-4"
      onclick={onAction}>{actionLabel}</button
    >{/if}
</section>
