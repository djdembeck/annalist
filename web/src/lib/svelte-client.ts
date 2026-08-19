// Test-only handle to Svelte's *client* runtime entry.
//
// Under `bun test`, a bare `import from "svelte"` resolves to the *server*
// entry (whose `mount` throws), and no bunfig option changes that — the
// client build is only selected via the CLI `--conditions=browser` flag,
// which `bun test` does not set. test-setup.ts rewrites each compiled
// component's bare `svelte` import to this same client entry, so the
// component and the test's `mount` share one runtime universe. Under
// vitest the relative path resolves to the same module the sveltekit()
// plugin uses, so one import works for both runners.
//
// The specifier is a literal, but a static import cannot bypass the
// svelte package exports map under bun test, so dynamic import is
// required here.
// @ts-expect-error -- no-dynamic-import; see comment above.
const client = await import("../../node_modules/svelte/src/index-client.js");

export const { mount, unmount, tick } = client;
