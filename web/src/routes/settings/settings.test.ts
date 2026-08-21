// Component test for the settings page's commit-type save contract.
//
// Runs under both `bun test` (jsdom + .svelte client compilation via
// bunfig.toml preload) and `bunx vitest run` (jsdom + the sveltekit()
// vite plugin). The page is mounted with svelte's client `mount` and
// queried with @testing-library/dom directly — this avoids
// @testing-library/svelte's svelte-core, which imports `svelte` from
// inside node_modules where the test runner cannot steer it to the
// client build. The API is stubbed at the fetch boundary: `putSettings`
// serializes with JSON.stringify, so the request body is the exact
// payload the page would PUT.
import { describe, it, expect, afterEach, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/dom";
import { mount, unmount } from "$lib/svelte-client";

import type { Settings, SettingsUpdate } from "$lib/api";
import SettingsPage from "./+page.svelte";

const settingsFixture: Settings = {
  tone: null,
  instructions: null,
  model: null,
  temperature: null,
  commit_types: null,
  mode: "lite",
  max_tokens: 0,
  thinking_level: null,
  llm: { base_url: "https://api.example.com", api_key: "", has_key: false },
  github: false,
  forgejo: false,
};

type JsonInit = { method?: string; body?: string };

/** A vi.fn() used as the fetch stub; `.mock.calls` carries [input, init]. */
type FetchMockFn = {
  (input: string, init?: JsonInit): Promise<Response>;
  mock: { calls: [string, JsonInit?][] };
};

let fetchMock: FetchMockFn;
let putBody: string | null = null;
let putResolve: ((settings: Settings) => void) | null = null;
let saveInFlight = false;
let putCallCount = 0;

function jsonResponse(data: unknown): Response {
  return new Response(JSON.stringify(data), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function mockSettings(overrides: Partial<Settings> = {}): void {
  putBody = null;
  putResolve = null;
  saveInFlight = false;
  putCallCount = 0;
  const s = { ...settingsFixture, ...overrides };
  fetchMock = vi.fn(async (input: string, init?: JsonInit) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (method === "PUT" && url.endsWith("/api/settings")) {
      putCallCount += 1;
      saveInFlight = true;
      putBody = init?.body ?? null;
      // Stay in flight until the test resolves the pending save, so
      // mid-flight state (disabled controls) can be asserted.
      const settings = await new Promise<Settings>((resolve) => {
        putResolve = resolve;
      });
      saveInFlight = false;
      return jsonResponse(settings);
    }
    if (method === "GET" && url.endsWith("/api/models")) {
      return jsonResponse([]);
    }
    return jsonResponse(s);
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
}

function resolvePutSettings(): void {
  putResolve?.({ ...settingsFixture });
}

/** Wait until putSettings' request has arrived, then record its body. */
async function waitForPut(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
  for (let i = 0; i < 100 && putBody === null; i++) {
    await new Promise((resolve) => setTimeout(resolve, 1));
  }
  expect(putBody).not.toBeNull();
}

let container: HTMLElement | null = null;
let component: ReturnType<typeof mount> | null = null;

afterEach(() => {
  // svelte's unmount is synchronous; no tick needed.
  if (component) {
    unmount(component);
    component = null;
  }
  container?.remove();
  container = null;
  vi.clearAllMocks();
});

async function mountPage() {
  container = document.createElement("div");
  document.body.append(container);
  component = mount(SettingsPage, { target: container });
  await waitFor(() => {
    expect(container?.textContent).toMatch(/Default selection shown/);
  });
}

describe("settings page commit type contract", () => {
  it("shows the default-selection hint when commit_types is unset", async () => {
    mockSettings();
    await mountPage();

    expect(screen.getByText(/Default selection shown for new installs/));
    // The tone-proof block shows the four recommended types as a
    // not-saved default (interpolated alongside the other lines, so
    // match the block's full text).
    expect(
      screen.getByText((content) =>
        content.includes("fix,feat,refactor,perf (default, not saved)"),
      ),
    );
  });

  it("preserves an unset commit_types filter on an unrelated save", async () => {
    mockSettings();
    await mountPage();

    // Change an unrelated field so the save is a real edit, but do not
    // touch any commit type checkbox.
    const instructions = screen.getByLabelText(/Instructions/);
    await fireEvent.input(instructions, { target: { value: "be terse" } });

    await fireEvent.click(
      screen.getByRole("button", { name: /Save contract/ }),
    );

    await waitForPut();
    const payload = JSON.parse(putBody as string) as SettingsUpdate;
    // The page's preserve-on-unrelated-save behavior: null stays null,
    // NOT the formatted default the checkboxes visually display.
    expect(payload.commit_types).toBe(null);
    // And the other fields carried the real edit.
    expect(payload.instructions).toBe("be terse");
    resolvePutSettings();
  });

  it("saves the formatted selection after toggling a checkbox", async () => {
    mockSettings();
    await mountPage();

    const refactorCheckbox = screen.getByRole("checkbox", {
      name: /refactor/i,
    });
    await fireEvent.click(refactorCheckbox);
    expect(putBody).toBeNull(); // no save yet

    await fireEvent.click(
      screen.getByRole("button", { name: /Save contract/ }),
    );

    await waitForPut();
    const payload = JSON.parse(putBody as string) as SettingsUpdate;
    // Default four minus the unchecked `refactor`.
    expect(payload.commit_types).toBe("fix,feat,perf");
    resolvePutSettings();
  });

  it("disables the form controls while a save is in flight", async () => {
    mockSettings();
    await mountPage();

    await fireEvent.click(
      screen.getByRole("button", { name: /Save contract/ }),
    );

    // The save is still pending (putResolve is not called in this test).
    await waitForPut();
    expect(saveInFlight).toBe(true);

    // The button shows its saving state and is disabled — the guard
    // against re-entrant saves.
    const saveButton = screen.getByRole("button", { name: /Saving/ });
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    expect((saveButton as HTMLButtonElement).disabled).toBe(true);

    // The commit-type group is disabled via the wrapping
    // <fieldset disabled={saving}>; the form-disabling semantics make
    // its inputs non-interactive. (jsdom does not reflect that on each
    // input's own `disabled` property, so assert the fieldset — the
    // page's actual contract.)
    const commitTypes = screen.getByRole("checkbox", { name: /fix/i });
    const fieldset = commitTypes.closest("fieldset");
    expect(fieldset).toBeInstanceOf(HTMLFieldSetElement);
    expect((fieldset as HTMLFieldSetElement).disabled).toBe(true);

    // Only one request went out despite the in-flight state.
    expect(putCallCount).toBe(1);
    resolvePutSettings();
  });

  it("saves max_tokens when edited", async () => {
    mockSettings();
    await mountPage();

    const maxTokens = screen.getByLabelText(/Max output tokens/);
    await fireEvent.input(maxTokens, { target: { value: "8192" } });

    await fireEvent.click(
      screen.getByRole("button", { name: /Save contract/ }),
    );

    await waitForPut();
    const payload = JSON.parse(putBody as string) as SettingsUpdate;
    expect(payload.max_tokens).toBe(8192);
    resolvePutSettings();
  });

  it("blank max_tokens sends null (inherit)", async () => {
    mockSettings({ max_tokens: 1234 });
    await mountPage();

    // The field starts populated from the saved value; blank it.
    const maxTokens = screen.getByLabelText(/Max output tokens/);
    await waitFor(() => {
      expect((maxTokens as HTMLInputElement).value).toBe("1234");
    });
    await fireEvent.input(maxTokens, { target: { value: "" } });

    await fireEvent.click(
      screen.getByRole("button", { name: /Save contract/ }),
    );

    await waitForPut();
    const payload = JSON.parse(putBody as string) as SettingsUpdate;
    expect(payload.max_tokens).toBe(null);
    resolvePutSettings();
  });

  it("saves thinking_level selection", async () => {
    mockSettings();
    await mountPage();

    const thinking = screen.getByLabelText(/Thinking level/);
    await fireEvent.change(thinking, { target: { value: "high" } });

    await fireEvent.click(
      screen.getByRole("button", { name: /Save contract/ }),
    );

    await waitForPut();
    const payload = JSON.parse(putBody as string) as SettingsUpdate;
    expect(payload.thinking_level).toBe("high");
    resolvePutSettings();
  });
});
