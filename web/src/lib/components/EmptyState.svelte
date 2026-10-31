<script lang="ts">
  let {
    heading,
    description,
    actionLabel,
    href,
  }: {
    heading: string;
    description: string;
    actionLabel: string;
    href: string;
  } = $props();
</script>

<section class="panel empty-state text-left">
  <h2>{heading}</h2>
  <p class="mt-2 max-w-xl text-sm text-ink-2">{description}</p>
  <a {href} class="btn btn-primary mt-5">{actionLabel}</a>
</section>
