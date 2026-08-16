// Setup for `bun test` (bunfig.toml `[test] preload`): wires up jsdom
// globals and a Bun.plugin loader that compiles `.svelte` files to the
// Svelte 5 *client* runtime.
//
// Why this exists: under plain `bun test`, a bare `import ... from "svelte"`
// resolves to Svelte's *server* entry (whose `mount`/lifecycle functions
// throw), and neither bunfig options nor plugin `onResolve`/`alias` hooks
// change that for bare package specifiers — only the CLI
// `--conditions=browser` flag would, which `bun test` does not set. So:
//   - `.svelte` files are compiled with `generate: "client"`, and the
//     compiler's bare `svelte` import in the output is rewritten to the
//     client entry file URL (components only import
//     `svelte/internal/client` + the bare entry).
//   - the tests import `mount`/`unmount`/`tick` through
//     `src/lib/svelte-client`, which loads that same client entry via a
//     relative path (bypassing the exports map), so component and test
//     share one runtime universe.
//
// `$lib/...` resolves natively via the SvelteKit-generated tsconfig
// `paths`; `$app/navigation` (imported only by the auth-error path these
// tests never hit) is mocked. Vitest (`bunx vitest run`) gets the
// equivalent setup from the `test` block in vite.config.ts + the
// sveltekit() plugin, so it does not read this file.
import { JSDOM } from "jsdom";
import { compile, compileModule } from "svelte/compiler";
import { readFileSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";

const dom = new JSDOM("<!doctype html><html><head></head><body></body></html>", {
  url: "http://localhost/",
});

const w = dom.window as unknown as Record<string, unknown>;
for (const key of Object.getOwnPropertyNames(w)) {
  if (key in globalThis) continue;
  try {
    (globalThis as Record<string, unknown>)[key] = w[key];
  } catch {
    // non-configurable global: skip
  }
}
Object.defineProperty(globalThis, "navigator", {
  value: w.navigator,
  configurable: true,
});
globalThis.window = w;
globalThis.document = w.document;
globalThis.requestAnimationFrame = (cb: FrameRequestCallback) =>
  setTimeout(cb, 0) as unknown as number;

// `bun:test` import at the top level of a preload: available there.
import { mock as bunMock } from "bun:test";
bunMock.module("$app/navigation", () => ({
  goto: () => Promise.resolve(),
}));

const webRoot = new URL(".", import.meta.url);

Bun.plugin({
  name: "svelte-client-for-bun-test",
  setup(build: {
    onResolve: (
      opts: { filter: RegExp },
      cb: (args: { path: string; importer?: string }) => {
        path: string;
        namespace?: string;
      },
    ) => void;
    onLoad: (
      opts: { filter: RegExp; namespace?: string },
      cb: (args: { path: string }) => { contents: string; loader: string },
    ) => void;
  }) {
    // Resolve a specifier against its importer (a filesystem path) or
    // the web root.
    const resolveSpec = (spec: string, importer?: string): string =>
      new URL(spec, importer ? pathToFileURL(importer) : webRoot).href;

    // The client entry as a URL, for rewriting compiled imports.
    const svelteClientUrl = new URL(
      "./node_modules/svelte/src/index-client.js",
      webRoot,
    ).href;
    // Rewrite only bare `svelte` imports — leave `svelte/internal/*`
    // subpaths, which resolve to the client build unconditionally.
    const toClientImports = (code: string): string =>
      code.replace(
        /from (["'])svelte\1(?![\/"'])/g,
        `from "${svelteClientUrl}"`,
      );

    // $lib/... -> src/lib/... (must run before the generic .svelte hook,
    // otherwise $lib/components/*.svelte is resolved as a relative path).
    // Note: bun resolves bare `$lib/foo.ts` natively via the generated
    // tsconfig `paths`; the plugin hook only needs to steer `$lib/*.svelte`
    // and any `$lib` specifier the native resolver would otherwise misroute.
    build.onResolve({ filter: /^\$lib(\/|$)/ }, (args) => ({
      path: new URL(`./src/lib/${args.path.slice(4)}`, webRoot).href,
    }));
    // Compile .svelte files to the client runtime.
    build.onResolve({ filter: /\.svelte$/ }, (args) => ({
      path: resolveSpec(args.path, args.importer),
      namespace: "svelte-client",
    }));
    build.onLoad(
      { filter: /\.svelte$/, namespace: "svelte-client" },
      (args) => {
        const source = readFileSync(fileURLToPath(args.path), "utf8");
        const compiled = compile(source, {
          generate: "client",
          filename: args.path,
          runes: true,
        });
        return { contents: toClientImports(compiled.js.code), loader: "js" };
      },
    );
    // Compile .svelte.js / .svelte.ts files (runes outside components),
    // e.g. @testing-library/svelte-core's props.svelte.js.
    build.onResolve({ filter: /\.svelte\.[jt]s$/ }, (args) => ({
      path: resolveSpec(args.path, args.importer),
      namespace: "svelte-module",
    }));
    build.onLoad(
      { filter: /\.svelte\.[jt]s$/, namespace: "svelte-module" },
      (args) => {
        const path = fileURLToPath(args.path);
        const source = readFileSync(path, "utf8");
        const compiled = compileModule(source, {
          filename: path,
          runes: true,
        });
        return {
          contents: toClientImports(compiled.js.code),
          loader: path.endsWith(".ts") ? "ts" : "js",
        };
      },
    );
  },
});
