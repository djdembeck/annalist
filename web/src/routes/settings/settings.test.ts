// Component behavior tests for global settings drafts and save contracts.
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
import { setToken, getToken } from "$lib/api";
import { beforeNavigate, goto } from "$app/navigation";
import SettingsPage from "./+page.svelte";

vi.mock("$app/navigation", () => ({
  goto: vi.fn(async () => {}),
  beforeNavigate: vi.fn(),
}));

let navigationGuard: Parameters<typeof beforeNavigate>[0];

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

type JsonInit = { method?: string; body?: string; headers?: Record<string, string> };

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
let failPut = false;
let unauthorizedPut = false;
let unauthorizedModels = false;
let loadedSettings: Settings = settingsFixture;

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
  failPut = false;
  unauthorizedPut = false;
  unauthorizedModels = false;
  setToken("");
  const s = { ...settingsFixture, ...overrides };
  loadedSettings = s;
  (beforeNavigate as typeof beforeNavigate & {
    mockImplementation: (implementation: typeof beforeNavigate) => void;
  }).mockImplementation((callback) => { navigationGuard = callback; });
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
      if (unauthorizedPut) return new Response(null, { status: 401 });
      if (failPut) return new Response(JSON.stringify({ error: "Save rejected" }), { status: 500 });
      return jsonResponse(settings);
    }
    if (method === "GET" && url.endsWith("/api/models")) {
      if (unauthorizedModels) return new Response(null, { status: 401 });
      return jsonResponse(s.model ? [s.model] : []);
    }
    return jsonResponse(s);
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
}

function resolvePutSettings(overrides: Partial<Settings> = {}): void {
  putResolve?.({ ...loadedSettings, ...overrides });
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
  setToken("");
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

async function mountPage() {
  container = document.createElement("div");
  document.body.append(container);
  component = mount(SettingsPage, { target: container });
  await waitFor(() => {
    expect(screen.getByLabelText(/Instructions/)).toBeInstanceOf(
      HTMLTextAreaElement,
    );
    expect(
      screen.getByRole("button", { name: /Save settings/ }),
    ).toBeInstanceOf(HTMLButtonElement);
  });
}

describe("settings page commit type contract", () => {
  it("preserves an unset commit_types filter on an unrelated save", async () => {
    mockSettings();
    await mountPage();

    // Change an unrelated field so the save is a real edit, but do not
    // touch any commit type checkbox.
    const instructions = screen.getByLabelText(/Instructions/);
    await fireEvent.input(instructions, { target: { value: "be terse" } });

    await fireEvent.click(
      screen.getByRole("button", { name: /Save settings/ }),
    );

    await waitForPut();
    const payload = JSON.parse(putBody as string) as SettingsUpdate;
    // Unrelated save: commit_types is omitted entirely, so the stored
    // filter can neither be pinned by the displayed default nor flipped.
    expect(payload.commit_types).toBeUndefined();
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
      screen.getByRole("button", { name: /Save settings/ }),
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

    await fireEvent.input(screen.getByLabelText(/Instructions/), {
      target: { value: "pending edit" },
    });
    await fireEvent.click(
      screen.getByRole("button", { name: /Save settings/ }),
    );

    await waitForPut();
    expect(saveInFlight).toBe(true);
    expect(screen.getByLabelText(/Instructions/).matches(":disabled")).toBe(true);
    expect(screen.getByLabelText(/Base URL/).matches(":disabled")).toBe(true);
    expect(screen.getByLabelText(/API key/).matches(":disabled")).toBe(true);
    expect(screen.getByLabelText(/Tone/, { selector: "select" }).matches(":disabled")).toBe(true);

    // The button shows its saving state and is disabled — the guard
    // against re-entrant saves.
    const saveButton = screen.getByRole("button", { name: /Saving/ });
    expect(saveButton).toBeInstanceOf(HTMLButtonElement);
    expect((saveButton as HTMLButtonElement).disabled).toBe(true);

    const commitTypes = screen.getByRole("checkbox", { name: /fix/i });
    expect(commitTypes.matches(":disabled")).toBe(true);

    // Only one request went out despite the in-flight state.
    expect(putCallCount).toBe(1);
    resolvePutSettings();
  });

  // Typing a value into the free-text "Additional types" input must mark
  // the selector dirty: without it, a save after an unrelated edit
  // re-sends the stale saved value (null here), silently dropping the
  // custom type the user just typed.
  it("persists a custom type typed into the free-text field on save", async () => {
    mockSettings();
    await mountPage();

    // Type a custom (non-known) type into the free-text field — this is
    // what must mark the selector dirty. The checkboxes stay at the
    // visible default four (fix, feat, refactor, perf).
    const customInput = screen.getByLabelText(
      /Additional types/,
    ) as HTMLInputElement;
    await fireEvent.input(customInput, { target: { value: "security" } });

    // Change an unrelated field so the save is a real edit, then save.
    const instructions = screen.getByLabelText(/Instructions/);
    await fireEvent.input(instructions, { target: { value: "be terse" } });

    await fireEvent.click(
      screen.getByRole("button", { name: /Save settings/ }),
    );

    await waitForPut();
    const payload = JSON.parse(putBody as string) as SettingsUpdate;
    // The custom type survives the save: visible default four plus the
    // typed extra — NOT the stale null the selector would re-send if the
    // field edit had not marked it dirty.
    expect(payload.commit_types).toBe("fix,feat,refactor,perf,security");
    expect(payload.instructions).toBe("be terse");
    resolvePutSettings();
  });

  // Unchecking every box leaves the effective selection empty; since the
  // user touched the selector, the save must send the keep-all sentinel
  // rather than re-sending the stored value.
  it("saves the keep-all sentinel when every checkbox is unchecked", async () => {
    mockSettings();
    await mountPage();

    for (const name of ["fix", "feat", "refactor", "perf"]) {
      await fireEvent.click(
        screen.getByRole("checkbox", { name: new RegExp(name, "i") }),
      );
    }

    await fireEvent.click(
      screen.getByRole("button", { name: /Save settings/ }),
    );

    await waitForPut();
    const payload = JSON.parse(putBody as string) as SettingsUpdate;
    expect(payload.commit_types).toBe("*");
    resolvePutSettings();
  });

  it("shows every box unchecked when the saved value is the keep-all sentinel", async () => {
    mockSettings({ commit_types: "*" });
    await mountPage();

    for (const name of ["fix", "feat", "refactor", "perf"]) {
      const box = screen.getByRole("checkbox", {
        name: new RegExp(name, "i"),
      }) as HTMLInputElement;
      expect(box.checked).toBe(false);
    }
    const customInput = screen.getByLabelText(
      /Additional types/,
    ) as HTMLInputElement;
    expect(customInput.value).toBe("");
    resolvePutSettings();
  });

  it("saves max_tokens when edited", async () => {
    mockSettings();
    await mountPage();

    const maxTokens = screen.getByLabelText(/Max output tokens/);
    await fireEvent.input(maxTokens, { target: { value: "8192" } });

    await fireEvent.click(
      screen.getByRole("button", { name: /Save settings/ }),
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
      screen.getByRole("button", { name: /Save settings/ }),
    );

    await waitForPut();
    const payload = JSON.parse(putBody as string) as SettingsUpdate;
    expect(payload.max_tokens).toBe(null);
    resolvePutSettings();
  });

  it("shows a validation error and skips the PUT when max_tokens is invalid", async () => {
    mockSettings();
    await mountPage();

    const maxTokens = screen.getByLabelText(/Max output tokens/);
    for (const bad of ["0", "1.5"]) {
      await fireEvent.input(maxTokens, { target: { value: bad } });
      await fireEvent.click(
        screen.getByRole("button", { name: /Save settings/ }),
      );

      await waitFor(() => {
        expect(document.activeElement).toBe(maxTokens);
        expect(maxTokens.closest("details")?.open).toBe(true);
        expect(screen.getAllByRole("alert").length).toBeGreaterThan(0);
      });
      expect(putCallCount).toBe(0);
      expect(putBody).toBeNull();
    }
  });

  it("saves thinking_level selection", async () => {
    mockSettings();
    await mountPage();

    const thinking = screen.getByLabelText(/Thinking level/);
    await fireEvent.change(thinking, { target: { value: "high" } });

    await fireEvent.click(
      screen.getByRole("button", { name: /Save settings/ }),
    );

    await waitForPut();
    const payload = JSON.parse(putBody as string) as SettingsUpdate;
    expect(payload.thinking_level).toBe("high");
    resolvePutSettings();
  });

  it("saves off literally so it suppresses a config/env level", async () => {
    // The saved row carries a level; the UI select shows it.
    mockSettings({ thinking_level: "medium" });
    await mountPage();

    const thinking = screen.getByLabelText(
      /Thinking level/,
    ) as HTMLSelectElement;
    expect(thinking.value).toBe("medium");
    // Switch to off (option value "").
    await fireEvent.change(thinking, { target: { value: "" } });

    await fireEvent.click(
      screen.getByRole("button", { name: /Save settings/ }),
    );

    await waitForPut();
    const payload = JSON.parse(putBody as string) as SettingsUpdate;
    // The UI's off option sends the literal "off" so it is stored as a value
    // that suppresses a level set via config/env; null would clear to "" and
    // re-inherit.
    expect(payload.thinking_level).toBe("off");
    resolvePutSettings();
  });

  it("maps a stored off back to the off option on load", async () => {
    mockSettings({ thinking_level: "off" });
    await mountPage();

    const thinking = screen.getByLabelText(
      /Thinking level/,
    ) as HTMLSelectElement;
    expect(thinking.value).toBe("");
    resolvePutSettings();
  });
});

function attemptNavigation(willUnload = false, from?: string, to?: string) {
  const cancel = vi.fn();
  const target = (path?: string): Parameters<typeof navigationGuard>[0]["from"] => {
    if (!path) return null;
    const url = new URL(path, "https://annalist.example");
    const internal = url.origin === "https://annalist.example";
    const routeId = internal && url.pathname === "/settings" ? "/settings"
      : internal && url.pathname === "/setup" ? "/setup"
      : internal && url.pathname === "/repos" ? "/repos" : null;
    return {
      url,
      params: routeId ? {} : null,
      route: { id: routeId },
      scroll: null,
    };
  };
  const navigation: Parameters<typeof navigationGuard>[0] = {
    type: "goto",
    willUnload,
    cancel,
    complete: willUnload ? Promise.withResolvers<void>().promise : Promise.resolve(),
    from: target(from),
    to: target(to),
  };
  navigationGuard(navigation);
  return cancel;
}

function saveButton(): HTMLButtonElement {
  return screen.getByRole("button", { name: /Save settings/ }) as HTMLButtonElement;
}

function unloadIsPrevented(): boolean {
  const event = new window.Event("beforeunload", { cancelable: true });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}

describe("settings session recovery", () => {
  it("discards an unauthorized draft without retaining its preservation notice or restoring expired credentials", async () => {
    mockSettings({ instructions: "Saved instructions", model: "saved-model", llm: { ...settingsFixture.llm, has_key: true } });
    setToken("expired-token");
    await mountPage();
    const instructions = screen.getByLabelText(/Instructions/) as HTMLTextAreaElement;
    const key = screen.getByLabelText(/API key/) as HTMLInputElement;
    await fireEvent.input(instructions, { target: { value: "Unsaved draft" } });
    await fireEvent.input(key, { target: { value: "unsaved-provider-secret" } });
    unauthorizedPut = true;
    await fireEvent.click(saveButton());
    await waitForPut();
    resolvePutSettings();
    await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/session expired/i));
    expect(getToken()).toBe("");
    expect(instructions.value).toBe("Unsaved draft");
    expect(putCallCount).toBe(1);

    await fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    await waitFor(() => expect(saveButton().disabled).toBe(true));
    expect(instructions.value).toBe("Saved instructions");
    expect((screen.getByLabelText(/Model \(/) as HTMLSelectElement).value).toBe("saved-model");
    expect(key.value).toBe("");
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByRole("link", { name: /Reauthenticate in setup/ })).toBeNull();
    expect(unloadIsPrevented()).toBe(false);
    expect(getToken()).toBe("");
    expect(goto).not.toHaveBeenCalled();
    expect(putCallCount).toBe(1);
  });

  for (const operation of ["save", "model refresh"]) {
    it(`preserves a dirty draft after unauthorized ${operation} and retries with the reauthenticated token`, async () => {
      mockSettings();
      setToken("expired-token");
      await mountPage();
      const instructions = screen.getByLabelText(/Instructions/) as HTMLTextAreaElement;
      const key = screen.getByLabelText(/API key/) as HTMLInputElement;
      await fireEvent.input(instructions, { target: { value: "Keep my instructions" } });
      await fireEvent.input(key, { target: { value: "private-provider-key" } });
      const persist = vi.spyOn(Storage.prototype, "setItem");
      const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
      if (operation === "save") {
        unauthorizedPut = true;
        await fireEvent.click(saveButton());
        await waitForPut();
        resolvePutSettings();
      } else {
        unauthorizedModels = true;
        (key.closest("details") as HTMLDetailsElement).open = true;
        await fireEvent.click(screen.getByRole("button", { name: "Refresh models" }));
      }
      await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/session expired/i));
      expect(getToken()).toBe("");
      expect(goto).not.toHaveBeenCalled();
      expect(confirm).not.toHaveBeenCalled();
      expect(instructions.value).toBe("Keep my instructions");
      expect(key.value).toBe("private-provider-key");
      expect(saveButton().disabled).toBe(false);
      expect(unloadIsPrevented()).toBe(true);
      expect(persist).not.toHaveBeenCalled();
      const recovery = screen.getByRole("link", { name: /Reauthenticate in setup/ });
      expect(recovery.getAttribute("href")).toBe("/setup");
      expect(recovery.getAttribute("target")).toBe("_blank");
      expect(recovery.getAttribute("rel")).toBe("noopener");
      expect(attemptNavigation(false, "/settings", "/setup")).toHaveBeenCalledOnce();
      expect(confirm).toHaveBeenCalledOnce();
      expect(instructions.value).toBe("Keep my instructions");

      // Setup stores the new token in same-origin localStorage; the existing
      // settings tab must read it on its next request, without remounting.
      setToken("renewed-token");
      unauthorizedPut = false;
      unauthorizedModels = false;
      if (operation === "save") {
        putBody = null;
        await fireEvent.click(saveButton());
        await waitForPut();
        expect(JSON.parse(String(putBody)).llm_api_key).toBe("private-provider-key");
        resolvePutSettings({ instructions: "Keep my instructions", llm: { ...settingsFixture.llm, has_key: true } });
        await waitFor(() => expect(saveButton().disabled).toBe(true));
      } else {
        await fireEvent.click(screen.getByRole("button", { name: "Refresh models" }));
        await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
        expect(instructions.value).toBe("Keep my instructions");
        expect(key.value).toBe("private-provider-key");
        expect(unloadIsPrevented()).toBe(true);
      }
      const requests = fetchMock.mock.calls.filter(([url, init]) =>
        operation === "save" ? init?.method === "PUT" : url.endsWith("/api/models"),
      );
      expect(requests[requests.length - 1][1]?.headers?.Authorization).toBe("Bearer renewed-token");
      expect(screen.queryByRole("link", { name: /Reauthenticate in setup/ })).toBeNull();
    });
  }

  it("redirects a clean unauthorized model refresh to setup without a draft warning", async () => {
    mockSettings();
    await mountPage();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    unauthorizedModels = true;
    (screen.getByLabelText(/API key/).closest("details") as HTMLDetailsElement).open = true;
    await fireEvent.click(screen.getByRole("button", { name: "Refresh models" }));
    await waitFor(() => expect(goto).toHaveBeenCalledWith("/setup"));
    expect(attemptNavigation(false, "/settings", "/setup")).not.toHaveBeenCalled();
    expect(confirm).not.toHaveBeenCalled();
    expect(unloadIsPrevented()).toBe(false);
    expect(screen.queryByRole("link", { name: /Reauthenticate in setup/ })).toBeNull();
  });
});

describe("settings draft lifecycle", () => {
  it("allows summary and back hash navigation on the same path and query without prompting", async () => {
    mockSettings();
    await mountPage();
    await fireEvent.input(screen.getByLabelText(/Instructions/), { target: { value: "Keep draft" } });
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    for (const name of ["Form summary", "Back to Save and Discard"]) {
      for (const link of screen.getAllByRole("link", { name })) {
        const href = link.getAttribute("href");
        expect(href?.startsWith("#")).toBe(true);
        expect(attemptNavigation(false, "/settings?view=defaults", `/settings?view=defaults${href}`)).not.toHaveBeenCalled();
      }
    }
    expect(confirm).not.toHaveBeenCalled();
    expect(unloadIsPrevented()).toBe(true);
    expect(attemptNavigation(false, "/settings?view=defaults", "/settings?view=other#settings-summary")).toHaveBeenCalledOnce();
    expect(attemptNavigation(false, "/settings?view=defaults", "/repos?view=defaults#settings-summary")).toHaveBeenCalledOnce();
    expect(confirm).toHaveBeenCalledTimes(2);
  });

  it("uses only the browser unload prompt for dirty full-page exits", async () => {
    mockSettings();
    await mountPage();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    expect(attemptNavigation(true, "/settings", "https://external.example/")).not.toHaveBeenCalled();
    expect(unloadIsPrevented()).toBe(false);
    await fireEvent.input(screen.getByLabelText(/Instructions/), { target: { value: "Unsaved" } });
    expect(attemptNavigation(true, "/settings", "https://external.example/")).not.toHaveBeenCalled();
    expect(confirm).not.toHaveBeenCalled();
    expect(unloadIsPrevented()).toBe(true);
    expect(confirm).not.toHaveBeenCalled();
    await fireEvent.input(screen.getByLabelText(/Instructions/), { target: { value: "" } });
    expect(unloadIsPrevented()).toBe(false);
  });

  it("becomes clean after restoring every control to its original semantic value", async () => {
    mockSettings({ model: "original-model", temperature: 0.85, max_tokens: 1200, thinking_level: "medium" });
    await mountPage();
    const inputEdits = [
      { label: /Instructions/, edited: "draft", original: "" },
      { label: /Base URL/, edited: "https://other.example.com", original: "https://api.example.com" },
      { label: /API key/, edited: "temporary-secret", original: "" },
      { label: /Temperature/, edited: "1.25", original: "0.85" },
      { label: /Max output tokens/, edited: "3000", original: "1200" },
      { label: /Additional types/, edited: "Security", original: "" },
    ];
    for (const edit of inputEdits) {
      const field = screen.getByLabelText(edit.label);
      await fireEvent.input(field, { target: { value: edit.edited } });
      expect(saveButton().disabled).toBe(false);
      await fireEvent.input(field, { target: { value: edit.original } });
      await waitFor(() => expect(saveButton().disabled).toBe(true));
    }
    for (const edit of [
      { label: /Model \(/, edited: "", original: "original-model" },
      { label: /^Mode(?!l)/, edited: "deep", original: "lite" },
      { label: /Thinking level/, edited: "high", original: "medium" },
    ]) {
      const field = screen.getByLabelText(edit.label);
      await fireEvent.change(field, { target: { value: edit.edited } });
      expect(saveButton().disabled).toBe(false);
      await fireEvent.change(field, { target: { value: edit.original } });
      await waitFor(() => expect(saveButton().disabled).toBe(true));
    }
    const tone = screen.getByLabelText(/Tone/, { selector: "select" });
    await fireEvent.change(tone, { target: { value: "custom" } });
    await fireEvent.input(screen.getByLabelText(/Custom tone/), { target: { value: "Temporary voice" } });
    expect(saveButton().disabled).toBe(false);
    await fireEvent.change(tone, { target: { value: "inherit" } });
    await waitFor(() => expect(saveButton().disabled).toBe(true));
    const refactor = screen.getByRole("checkbox", { name: /refactor/i });
    await fireEvent.click(refactor);
    expect(saveButton().disabled).toBe(false);
    await fireEvent.click(refactor);
    await waitFor(() => expect(saveButton().disabled).toBe(true));
    expect(unloadIsPrevented()).toBe(false);
  });

  it("tracks provider, model, mode, thinking, and custom-tone edits through discard", async () => {
    mockSettings({ tone: "Personal voice", model: "saved-model", thinking_level: "medium" });
    await mountPage();
    const edits = [
      { label: /Base URL/, event: "input", value: "https://other.example.com" },
      { label: /Model \(/, event: "change", value: "" },
      { label: /^Mode(?!l)/, event: "change", value: "deep" },
      { label: /Thinking level/, event: "change", value: "high" },
      { label: /Custom tone/, event: "input", value: "Another voice" },
      { label: /Max output tokens/, event: "input", value: "8000" },
    ];
    for (const edit of edits) {
      const field = screen.getByLabelText(edit.label);
      if (edit.event === "input") await fireEvent.input(field, { target: { value: edit.value } });
      else await fireEvent.change(field, { target: { value: edit.value } });
      expect(unloadIsPrevented()).toBe(true);
      await fireEvent.click(screen.getByRole("button", { name: "Discard" }));
      expect(unloadIsPrevented()).toBe(false);
      expect(saveButton().disabled).toBe(true);
    }
  });

  it("saves an unrelated edit with a precise temperature inside collapsed tuning", async () => {
    mockSettings({ temperature: 0.85 });
    await mountPage();
    const temperature = screen.getByLabelText(/Temperature/) as HTMLInputElement;
    expect(temperature.closest("details")?.open).toBe(false);
    await fireEvent.input(screen.getByLabelText(/Instructions/), {
      target: { value: "Keep the configured provider temperature" },
    });
    await fireEvent.click(saveButton());
    await waitForPut();
    const payload = JSON.parse(putBody as string) as SettingsUpdate;
    expect(payload.instructions).toBe("Keep the configured provider temperature");
    expect(payload.temperature).toBe(0.85);
    resolvePutSettings({ temperature: 0.85, instructions: "Keep the configured provider temperature" });
    await waitFor(() => expect(saveButton().disabled).toBe(true));
    expect(unloadIsPrevented()).toBe(false);
  });

  it("saves blank temperature as inherited rather than invalid or zero", async () => {
    mockSettings({ temperature: 0.8 });
    await mountPage();
    await fireEvent.input(screen.getByLabelText(/Temperature/), { target: { value: "" } });
    await fireEvent.click(saveButton());
    await waitForPut();
    expect(JSON.parse(putBody as string).temperature).toBeNull();
    resolvePutSettings({ temperature: null });
    await waitFor(() => expect(saveButton().disabled).toBe(true));
  });

  it("retains edits when SPA navigation is cancelled and allows a confirmed discard", async () => {
    mockSettings();
    await mountPage();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    const input = screen.getByLabelText(/Instructions/) as HTMLTextAreaElement;
    await fireEvent.input(input, { target: { value: "Keep this draft" } });
    expect(attemptNavigation()).toHaveBeenCalledOnce();
    expect(input.value).toBe("Keep this draft");
    expect(unloadIsPrevented()).toBe(true);
    confirm.mockReturnValue(true);
    expect(attemptNavigation()).not.toHaveBeenCalled();
    expect(confirm).toHaveBeenCalledTimes(2);
  });

  it("returns to clean when ordinary edits are restored, including numeric blank inheritance", async () => {
    mockSettings();
    await mountPage();
    const input = screen.getByLabelText(/Instructions/);
    await fireEvent.input(input, { target: { value: "draft" } });
    expect(saveButton().disabled).toBe(false);
    await fireEvent.input(input, { target: { value: "" } });
    await waitFor(() => expect(saveButton().disabled).toBe(true));
    const temperature = screen.getByLabelText(/Temperature/);
    await fireEvent.input(temperature, { target: { value: "0.7" } });
    expect(saveButton().disabled).toBe(false);
    await fireEvent.input(temperature, { target: { value: "" } });
    await waitFor(() => expect(saveButton().disabled).toBe(true));
    expect(unloadIsPrevented()).toBe(false);
  });

  it("discards back to loaded values and clears pending key edits without saving", async () => {
    mockSettings({ instructions: "Original", llm: { base_url: "https://api.example.com", api_key: "masked", has_key: true } });
    await mountPage();
    const persist = vi.spyOn(Storage.prototype, "setItem");
    const instructions = screen.getByLabelText(/Instructions/) as HTMLTextAreaElement;
    const key = screen.getByLabelText(/API key/) as HTMLInputElement;
    await fireEvent.input(instructions, { target: { value: "Changed" } });
    await fireEvent.input(key, { target: { value: "temporary-secret" } });
    await fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(instructions.value).toBe("Original");
    expect(key.value).toBe("");
    expect(saveButton().disabled).toBe(true);
    expect(unloadIsPrevented()).toBe(false);
    expect(putCallCount).toBe(0);
    expect(document.body.textContent).not.toContain("temporary-secret");
    expect(persist).not.toHaveBeenCalled();
  });

  it("baselines a successful save and later discards to that saved value", async () => {
    mockSettings();
    await mountPage();
    const input = screen.getByLabelText(/Instructions/) as HTMLTextAreaElement;
    await fireEvent.input(input, { target: { value: "Accepted" } });
    await fireEvent.click(saveButton());
    await waitForPut();
    expect(unloadIsPrevented()).toBe(true);
    resolvePutSettings({ instructions: "Accepted" });
    await waitFor(() => expect(saveButton().disabled).toBe(true));
    expect(unloadIsPrevented()).toBe(false);
    await fireEvent.input(input, { target: { value: "Another edit" } });
    await fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(input.value).toBe("Accepted");
    expect(unloadIsPrevented()).toBe(false);
  });

  it("keeps failed saves dirty and retains the draft for retry", async () => {
    mockSettings();
    failPut = true;
    await mountPage();
    const input = screen.getByLabelText(/Instructions/) as HTMLTextAreaElement;
    await fireEvent.input(input, { target: { value: "Retry this" } });
    await fireEvent.click(saveButton());
    await waitForPut();
    resolvePutSettings();
    await waitFor(() => expect(saveButton().disabled).toBe(false));
    expect(input.value).toBe("Retry this");
    expect(unloadIsPrevented()).toBe(true);
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    expect(attemptNavigation()).toHaveBeenCalledOnce();
    expect(confirm).toHaveBeenCalledOnce();
  });

  it("omits an untouched key on an unrelated save", async () => {
    mockSettings({ llm: { base_url: "https://api.example.com", api_key: "masked", has_key: true } });
    await mountPage();
    await fireEvent.input(screen.getByLabelText(/Instructions/), { target: { value: "Edited" } });
    await fireEvent.click(saveButton());
    await waitForPut();
    expect(JSON.parse(putBody as string)).not.toHaveProperty("llm_api_key");
    expect(JSON.parse(putBody as string)).not.toHaveProperty("llm_base_url");
    resolvePutSettings();
  });

  it("sends null for a touched-empty key and clears key edits after save", async () => {
    mockSettings({ llm: { base_url: "https://api.example.com", api_key: "masked", has_key: true } });
    await mountPage();
    const key = screen.getByLabelText(/API key/) as HTMLInputElement;
    await fireEvent.input(key, { target: { value: "replacement" } });
    await fireEvent.input(key, { target: { value: "" } });
    expect(unloadIsPrevented()).toBe(true);
    await fireEvent.click(saveButton());
    await waitForPut();
    expect(JSON.parse(putBody as string).llm_api_key).toBeNull();
    resolvePutSettings();
    await waitFor(() => expect(saveButton().disabled).toBe(true));
    expect(key.value).toBe("");
    expect(unloadIsPrevented()).toBe(false);
  });

  it("retains uncommon and custom types through disclosure collapse and an unrelated save", async () => {
    mockSettings({ commit_types: "fix,docs,Security" });
    await mountPage();
    const custom = screen.getByLabelText(/Additional types/) as HTMLInputElement;
    const details = custom.closest("details") as HTMLDetailsElement;
    expect(details.open).toBe(true);
    const docs = screen.getByRole("checkbox", { name: /docs/i }) as HTMLInputElement;
    expect(docs.checked).toBe(true);
    const style = screen.getByRole("checkbox", { name: /style/i });
    await fireEvent.click(style);
    details.open = false;
    await fireEvent(details, new window.Event("toggle"));
    await fireEvent.input(screen.getByLabelText(/Instructions/), { target: { value: "Edited" } });
    await fireEvent.click(saveButton());
    await waitForPut();
    expect(JSON.parse(putBody as string).commit_types).toBe("fix,docs,style,Security");
    expect(custom.value).toBe("Security");
    expect(docs.checked).toBe(true);
    resolvePutSettings();
  });

  it("restores a type selection to clean without pinning an inherited filter", async () => {
    mockSettings();
    await mountPage();
    const refactor = screen.getByRole("checkbox", { name: /refactor/i });
    await fireEvent.click(refactor);
    expect(saveButton().disabled).toBe(false);
    await fireEvent.click(refactor);
    await waitFor(() => expect(saveButton().disabled).toBe(true));
    await fireEvent.input(screen.getByLabelText(/Instructions/), { target: { value: "Edited" } });
    await fireEvent.click(saveButton());
    await waitForPut();
    // Restored to clean: nothing is sent, so the inherited filter is kept.
    expect(JSON.parse(putBody as string).commit_types).toBeUndefined();
    resolvePutSettings();
  });
});
