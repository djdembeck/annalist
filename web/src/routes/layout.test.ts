// Component behavior tests for the root layout's theme switch: the switch
// must keep the document root, the browser-chrome theme-color meta, and the
// stored choice in step, and follow the OS preference until an explicit
// choice is stored.
//
// Runs under both `bun test` (jsdom + .svelte client compilation via
// bunfig.toml preload) and `bunx vitest run` (jsdom + the sveltekit()
// vite plugin), mounting with svelte's client `mount` and querying with
// @testing-library/dom directly.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen } from "@testing-library/dom";
import { mount, unmount, tick } from "$lib/svelte-client";

import LayoutPage from "./+layout.svelte";

type PageState = { url: URL };

// The $app/stores.page stub (the viMock* prefix lets vitest keep the
// factory's closure reference alive when it hoists the mock).
const viMockPage: PageState = { url: new URL("http://localhost/repos") };
(globalThis as Record<string, unknown>).__annalist_page_state = viMockPage;

vi.mock("$app/stores", () => ({
  page: {
    subscribe: (run: (value: PageState) => void) => {
      run(viMockPage);
      return () => {};
    },
  },
}));

vi.mock("$app/navigation", () => ({
  goto: vi.fn(async () => {}),
  afterNavigate: vi.fn(),
}));

// jsdom has no matchMedia; the layout's system listener needs a live
// MediaQueryList whose `matches` the tests can flip to fire its change
// handler (the listener re-reads `media.matches`, ignoring the event).
let mediaMatches = false;
const systemListeners = new Set<(event: unknown) => void>();
let meta: HTMLMetaElement;

beforeEach(() => {
  mediaMatches = false;
  systemListeners.clear();
  window.matchMedia = vi.fn().mockImplementation(
    (): MediaQueryList =>
      ({
        get matches() {
          return mediaMatches;
        },
        addEventListener: (type: string, fn: (event: unknown) => void) => {
          if (type === "change") systemListeners.add(fn);
        },
        removeEventListener: (type: string, fn: (event: unknown) => void) => {
          if (type === "change") systemListeners.delete(fn);
        },
      }) as unknown as MediaQueryList,
  ) as unknown as typeof window.matchMedia;
  meta = document.createElement("meta");
  meta.setAttribute("name", "theme-color");
  document.head.appendChild(meta);
});

/** The change handler runs synchronously; its DOM effect needs a tick. */
async function fireSystemChange(): Promise<void> {
  for (const listener of [...systemListeners]) listener({});
  await tick();
}

let component: Record<string, unknown> | undefined;
let container: HTMLElement | undefined;

afterEach(async () => {
  if (component) await unmount(component);
  component = undefined;
  container?.remove();
  container = undefined;
  delete document.documentElement.dataset.theme;
  meta.remove();
  localStorage.clear();
  delete (window as unknown as Record<string, unknown>).matchMedia;
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

async function mountLayout(): Promise<void> {
  container = document.createElement("div");
  document.body.append(container);
  // The layout renders the routed page into `children`; the tests only
  // exercise the chrome, so a no-op snippet is enough to render.
  component = mount(LayoutPage, {
    target: container,
    props: { children: () => () => {} },
  });
  await tick();
}

/** The combined state applyTheme() must keep in step at every call site. */
function themeState(): {
  dataset: string;
  meta: string;
  checked: string;
  stored: string | null;
} {
  return {
    dataset: document.documentElement.dataset.theme ?? "",
    meta: meta.content,
    checked:
      screen
        .getByRole("switch", { name: "Dark theme" })
        .getAttribute("aria-checked") ?? "",
    stored: localStorage.getItem("annalist-theme"),
  };
}

describe("layout theme switch", () => {
  it("keeps the document, chrome color, storage, and switch in step on click", async () => {
    await mountLayout();
    expect(themeState()).toEqual({
      dataset: "light",
      meta: "#f5eee5",
      checked: "false",
      stored: null,
    });
    await fireEvent.click(screen.getByRole("switch", { name: "Dark theme" }));
    expect(themeState()).toEqual({
      dataset: "dark",
      meta: "#211d19",
      checked: "true",
      stored: "dark",
    });
    await fireEvent.click(screen.getByRole("switch", { name: "Dark theme" }));
    expect(themeState()).toEqual({
      dataset: "light",
      meta: "#f5eee5",
      checked: "false",
      stored: "light",
    });
  });

  it("re-themes the page with the OS preference until a choice is stored", async () => {
    await mountLayout();
    mediaMatches = true;
    await fireSystemChange();
    expect(themeState()).toEqual({
      dataset: "dark",
      meta: "#211d19",
      checked: "true",
      stored: null,
    });
    mediaMatches = false;
    await fireSystemChange();
    expect(themeState()).toEqual({
      dataset: "light",
      meta: "#f5eee5",
      checked: "false",
      stored: null,
    });
  });

  it("keeps an explicit stored choice over a later OS flip", async () => {
    // Mirror the static/theme.js bootstrap: the stored dark choice is
    // painted on the root before the app mounts.
    localStorage.setItem("annalist-theme", "dark");
    document.documentElement.dataset.theme = "dark";
    mediaMatches = true;
    await mountLayout();
    expect(themeState()).toEqual({
      dataset: "dark",
      meta: "#211d19",
      checked: "true",
      stored: "dark",
    });
    mediaMatches = false;
    await fireSystemChange();
    // The OS flipped to light; the explicit choice must win.
    expect(themeState()).toEqual({
      dataset: "dark",
      meta: "#211d19",
      checked: "true",
      stored: "dark",
    });
  });
});
