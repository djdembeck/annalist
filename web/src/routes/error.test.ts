// Component behavior tests for the render-less error route: the recovery
// CTA must re-navigate to the exact failed location (path + query + hash)
// and 404s must offer the repository list instead of a no-op retry.
//
// Runs under both `bun test` (jsdom + .svelte client compilation via
// bunfig.toml preload) and `bunx vitest run` (jsdom + the sveltekit()
// vite plugin), mounting with svelte's client `mount` and querying with
// @testing-library/dom directly.
import { afterEach, afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import { fireEvent, screen } from "@testing-library/dom";
import { mount, unmount } from "$lib/svelte-client";

import { goto } from "$app/navigation";
import ErrorPage from "./+error.svelte";

type PageState = { status: number; url: URL };

// The $app/stores.page stub. Under `bunx vitest run` the sveltekit()
// plugin aliases $app/stores, so the factory below serves the store.
// Under `bun test` test-setup.ts registers the same global as a module
// mock instead. Each test points the stub at its status before mounting;
// the component reads $page once on mount, which the single-shot
// subscribe covers. (The viMock* prefix is required so vitest keeps the
// factory's closure reference alive when it hoists the mock.)
const viMockPage: PageState = {
  status: 500,
  url: new URL("http://localhost/x?y=1#z"),
};
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
  beforeNavigate: vi.fn(),
}));

// tryAgain() reads the global location, not $page.url, so the stub must
// match the page state the test asserts against. Under `bun test` the
// preload's JSDOM already loads at that URL (location is unconfigurable
// there), so the stub is only installed where it can be.
const originalLocation = window.location;
const locationConfigurable =
  Object.getOwnPropertyDescriptor(window, "location")?.configurable === true;

beforeAll(() => {
  if (!locationConfigurable) return;
  Object.defineProperty(window, "location", {
    value: {
      pathname: "/x",
      search: "?y=1",
      hash: "#z",
      origin: "http://localhost",
    },
    configurable: true,
  });
});

afterAll(() => {
  if (!locationConfigurable) return;
  Object.defineProperty(window, "location", {
    value: originalLocation,
    configurable: true,
  });
});

let component: Record<string, unknown> | undefined;
let container: HTMLElement | undefined;

afterEach(async () => {
  if (component) await unmount(component);
  component = undefined;
  container?.remove();
  container = undefined;
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

function mountErrorPage(): Promise<void> {
  container = document.createElement("div");
  document.body.append(container);
  component = mount(ErrorPage, { target: container });
  return Promise.resolve();
}

describe("render-less error page", () => {
  it("reloads the exact failed location when Try again is clicked", async () => {
    viMockPage.status = 500;
    await mountErrorPage();
    expect(screen.getByRole("alert")).toBeInstanceOf(HTMLElement);
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(goto).toHaveBeenCalledWith("/x?y=1#z");
  });

  it("offers the repositories list for 404s without a retry button", async () => {
    viMockPage.status = 404;
    await mountErrorPage();
    expect(
      screen.queryByRole("button", { name: "Try again" }),
    ).toBeNull();
    expect(screen.getByRole("link", { name: "View repositories" }))
      .toBeInstanceOf(HTMLAnchorElement);
    expect(goto).not.toHaveBeenCalled();
  });
});
