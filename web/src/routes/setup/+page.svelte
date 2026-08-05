<script lang="ts">
  import { goto } from "$app/navigation";
  import { getStatus, setToken, getToken, type Status } from "$lib/api";

  let adminToken = $state<string>(getToken());
  let status = $state<Status | null>(null);
  let error = $state<string>("");

  async function loadStatus(): Promise<void> {
    error = "";
    if (!adminToken) {
      status = null;
      return;
    }
    try {
      status = await getStatus();
    } catch (e) {
      error = e instanceof Error ? e.message : "Failed to load status";
      status = null;
    }
  }

  function login(): void {
    setToken(adminToken);
    loadStatus();
    goto("/repos");
  }
</script>

<div class="mx-auto max-w-2xl">
  <h1 class="mb-2 font-display text-2xl text-white">Setup</h1>
  <p class="mb-6 text-sm text-ink-2">
    Set your admin token to unlock the dashboard.
  </p>

  <form
    class="mb-8 flex items-end gap-3"
    onsubmit={(e) => {
      e.preventDefault();
      login();
    }}
  >
    <label class="flex flex-1 flex-col gap-1">
      <span class="text-sm text-ink-2">ADMIN_TOKEN</span>
      <input
        type="password"
        bind:value={adminToken}
        placeholder="Paste your admin token"
        class="rounded border border-line-strong bg-surface-1 px-3 py-2 text-ink outline-none focus:border-focus"
      />
    </label>
    <button
      type="submit"
      class="rounded bg-mark bg-gradient-to-r from-cherry via-ember to-heat px-4 py-2 text-sm font-medium text-page hover:brightness-110"
    >
      Continue
    </button>
  </form>

  {#if error}
    <p class="mb-4 text-sm text-alert">{error}</p>
  {/if}

  {#if status}
    <section class="grid gap-4 sm:grid-cols-2">
      <div
        class="rounded border border-line bg-surface-1 p-4"
        class:opacity-60={!status.github}
      >
        <div class="flex items-center gap-2">
          <span
            class="h-2.5 w-2.5 rounded-full"
            class:bg-ok={status.github}
            class:bg-alert={!status.github}
          ></span>
          <span class="font-medium text-ink">GitHub</span>
        </div>
        <p class="mt-2 text-sm text-ink-2">
          {status.github ? "Configured" : "Not configured"}
        </p>
      </div>
      <div
        class="rounded border border-line bg-surface-1 p-4"
        class:opacity-60={!status.forgejo}
      >
        <div class="flex items-center gap-2">
          <span
            class="h-2.5 w-2.5 rounded-full"
            class:bg-ok={status.forgejo}
            class:bg-alert={!status.forgejo}
          ></span>
          <span class="font-medium text-ink">Forgejo</span>
        </div>
        <p class="mt-2 text-sm text-ink-2">
          {status.forgejo ? "Configured" : "Not configured"}
        </p>
      </div>
    </section>

    <section class="mt-8 space-y-6 text-sm text-ink-2">
      <div>
        <h2 class="mb-2 font-display text-base text-ink">GitHub</h2>
        <ul class="list-inside list-disc space-y-1 text-ink-2">
          <li>
            Install your GitHub App from
            <span class="text-ink">https://github.com/apps/&lt;your-app&gt;</span>
            and grant it access to the repositories you want to manage.
          </li>
          <li>
            Add a webhook (or use the app's webhook) for the
            <span class="text-ink">release</span> event with the configured
            secret.
          </li>
        </ul>
      </div>
      <div>
        <h2 class="mb-2 font-display text-base text-ink">Forgejo</h2>
        <ul class="list-inside list-disc space-y-1 text-ink-2">
          <li>
            Add a repository or organization webhook for the
            <span class="text-ink">Release</span> event.
          </li>
          <li>
            Set the webhook secret to the value of
            <span class="text-ink">FORGEJO_WEBHOOK_SECRET</span>.
          </li>
        </ul>
      </div>
    </section>
  {:else if !adminToken}
    <p class="text-sm text-ink-3">Enter a token above to see platform status.</p>
  {/if}
</div>
