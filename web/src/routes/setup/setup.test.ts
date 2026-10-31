// Component behavior tests for the first-run wizard's containers: the
// repository step must never trap the operator (loaded branch, zero
// results, load failure) and a finished add/retry must not gain a second
// forward action.
//
// Runs under both `bun test` (jsdom + .svelte client compilation via
// bunfig.toml preload) and `bunx vitest run` (jsdom + the sveltekit()
// vite plugin). The page is mounted with svelte's client `mount` and
// queried with @testing-library/dom directly; the API is stubbed at the
// fetch boundary.
import { describe, it, expect, beforeAll, afterEach, vi } from "vitest";
import { fireEvent, screen } from "@testing-library/dom";
import { mount, unmount, tick } from "$lib/svelte-client";

import SetupPage from "./+page.svelte";

type JsonInit = { method?: string; body?: string };

/** A vi.fn() used as the fetch stub; `.mock.calls` carries [input, init]. */
type FetchMockFn = {
  (input: string, init?: JsonInit): Promise<Response>;
  mock: { calls: [string, JsonInit?][] };
};

const originalMatchMedia = window.matchMedia;
let fetchMock: FetchMockFn;

/** Toggle: /api/repos/available fails until set true. */
let availableOk = true;
let available: unknown[] = [];
let forgejoConfigured = true;
let addResponse: ((repo: string) => Promise<Response>) | null = null;
const availCalls: string[] = [];

function jsonResponse(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function stubFetch(): void {
  availableOk = true;
  available = [];
  forgejoConfigured = true;
  addResponse = null;
  availCalls.length = 0;
  fetchMock = vi.fn(async (input: string, init?: JsonInit) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (method === "GET" && url.endsWith("/api/status")) {
      return jsonResponse({ github: false, forgejo: forgejoConfigured, admin: true });
    }
    if (method === "GET" && url.endsWith("/api/repos/available")) {
      availCalls.push(url);
      if (!availableOk) return jsonResponse({ error: "boom" }, 500);
      return jsonResponse(available);
    }
    if (method === "POST" && url.endsWith("/api/repos")) {
      const body = JSON.parse(init?.body ?? "{}") as { repo?: string };
      if (addResponse) return addResponse(body.repo ?? "");
      if (body.repo === "widgets")
        return jsonResponse({ error: "boom" }, 500);
      return jsonResponse({ id: 1 }, 201);
    }
    if (method === "PUT" && url.endsWith("/api/settings")) {
      return jsonResponse({ tone: "engineer" });
    }
    throw new Error(`Unexpected request: ${url}`);
  }) as FetchMockFn;
  // jsdom does not implement matchMedia; the page's scroll effect guards
  // prefers-reduced-motion through it.
  window.matchMedia = (query: string) => ({
    media: query,
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  }) as unknown as MediaQueryList;
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock as typeof fetch);
}

function forgeRepo(owner: string, repo: string): unknown {
  return { platform: "forgejo", owner, repo };
}

// Guided advance through steps 0 and 1 using the wizard's own controls,
// ending at the repository step.
async function advanceToRepos(): Promise<void> {
  await tick();
  const token = screen.getByLabelText("ADMIN_TOKEN");
  fireEvent.input(token, { target: { value: "test-token" } });
  await tick();
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));

  await screen.findByRole("heading", { name: "Configure platforms" });
  fireEvent.click(screen.getByRole("button", { name: "Continue to repositories" }));
}

let container: HTMLElement;
let component: ReturnType<typeof mount> | null = null;

beforeAll(() => {
  container = document.createElement("div");
  document.body.append(container);
});

function mountPage(): void {
  container.replaceChildren();
  document.body.replaceChildren(container);
  component = mount(SetupPage, { target: container });
}

async function teardown(): Promise<void> {
  if (component) {
    await unmount(component);
    component = null;
  }
  window.matchMedia = originalMatchMedia;
  vi.restoreAllMocks();
  vi.clearAllMocks();
}

afterEach(teardown);

describe("setup wizard repository step", () => {
  for (const configured of [false, true]) {
    it(`continues without discovery when a platform is ${configured ? "configured" : "not configured"}`, async () => {
      stubFetch();
      forgejoConfigured = configured;
      mountPage();
      await advanceToRepos();
      if (!configured) {
        fireEvent.click(await screen.findByRole("button", { name: "Continue anyway" }));
        await screen.findByText(/No platforms are configured, so there are no repositories/);
      } else {
        await screen.findByRole("button", { name: "Load repositories" });
      }
      const onward = screen.getAllByRole("button", { name: "Continue to tone" }) as HTMLButtonElement[];
      expect(onward).toHaveLength(1);
      expect(onward[0].disabled).toBe(false);
      fireEvent.click(onward[0]);
      await screen.findByRole("heading", { name: "Choose a default tone." });
      expect(availCalls).toHaveLength(0);
    });
  }

  it("updates each pending add and retry reactively while guarding onward navigation", async () => {
    stubFetch();
    available = [forgeRepo("team", "app"), forgeRepo("team", "widgets")];
    const pending = new Map<string, (response: Response) => void>();
    const posts: string[] = [];
    addResponse = (repo) => {
      posts.push(repo);
      const { promise, resolve } = Promise.withResolvers<Response>();
      pending.set(repo, resolve);
      return promise;
    };
    mountPage();
    await advanceToRepos();
    fireEvent.click(screen.getByRole("button", { name: "Load repositories" }));
    for (const row of await screen.findAllByRole("checkbox")) fireEvent.click(row);
    await tick();
    fireEvent.click(screen.getByRole("button", { name: "Add 2 selected" }));
    await screen.findByLabelText("Adding team/app");
    await screen.findByLabelText("Adding team/widgets");
    const onward = screen.getByRole("button", { name: "Continue to tone" }) as HTMLButtonElement;
    expect(onward.disabled).toBe(true);
    fireEvent.click(onward);
    expect(screen.queryByRole("heading", { name: "Choose a default tone." })).toBeNull();
    pending.get("app")!(jsonResponse({ id: 1 }, 201));
    await screen.findByLabelText("team/app added");
    expect(screen.getByLabelText("Adding team/widgets")).toBeDefined();
    expect(onward.disabled).toBe(true);
    pending.get("widgets")!(jsonResponse({ error: "boom" }, 500));
    await screen.findByText("1 failed");
    expect(screen.getByText("1 added successfully")).toBeDefined();
    expect(onward.disabled).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Retry 1 failed" }));
    await screen.findByLabelText("Adding team/widgets");
    expect(screen.getByLabelText("team/app added")).toBeDefined();
    expect(onward.disabled).toBe(true);
    expect(posts).toEqual(["app", "widgets", "widgets"]);
    pending.get("widgets")!(jsonResponse({ id: 2 }, 201));
    await screen.findByRole("heading", { name: "Choose a default tone." });
  });
  it("loaded discovery: no trap without a selection, single forward action after add, step-back preserved", async () => {
    stubFetch();
    available = [forgeRepo("team", "app"), forgeRepo("team", "widgets")];
    mountPage();
    await advanceToRepos();
    fireEvent.click(screen.getByRole("button", { name: "Load repositories" }));

    const rows = await screen.findAllByRole("checkbox");
    expect(rows).toHaveLength(2);

    // No selection: Add is disabled, but the forward action is present and
    // enabled — this is the trap the fix removes.
    const add = screen.getByRole("button", { name: "Add 0 selected" }) as HTMLButtonElement;
    expect(add.disabled).toBe(true);
    const continues = screen.getAllByRole("button", { name: "Continue to tone" }) as HTMLButtonElement[];
    expect(continues).toHaveLength(1);
    expect(continues[0].disabled).toBe(false);

    // Partial add (widgets fails): the reconciliation's forward action stays
    // the only one, and it still leads through to a saved setting.
    const boxes = screen.getAllByRole("checkbox");
    fireEvent.click(boxes[0]);
    fireEvent.click(boxes[1]);
    await tick();
    fireEvent.click(screen.getByRole("button", { name: "Add 2 selected" }));

    await screen.findByText("1 failed");
    expect(screen.getByRole("button", { name: "Retry 1 failed" })).toBeDefined();

    // Filtering every row out hides reconciliation details, but the shared
    // forward action must remain available after partial success.
    fireEvent.input(screen.getByRole("searchbox", { name: "Search repositories" }), {
      target: { value: "no matching repository" },
    });
    await screen.findByText(/No available forgejo repositories found/);
    const afterFilter = screen.getAllByRole("button", { name: "Continue to tone" }) as HTMLButtonElement[];
    expect(afterFilter).toHaveLength(1);
    expect(afterFilter[0].disabled).toBe(false);

    fireEvent.click(afterFilter[0]);
    await screen.findByRole("heading", { name: "Choose a default tone." });

    // Stepping back to a completed step works and does not re-fetch or trap.
    fireEvent.click(
      screen.getByRole("button", {
        name: "Return to completed step 3: Add repositories",
      }),
    );
    fireEvent.input(await screen.findByRole("searchbox", { name: "Search repositories" }), {
      target: { value: "" },
    });
    const preserved = await screen.findAllByRole("checkbox");
    expect(preserved).toHaveLength(2);
    expect(availCalls).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Continue to tone" }));
    await screen.findByRole("heading", { name: "Choose a default tone." });

  });

  it("zero discovery results: reachable without adding", async () => {
    stubFetch();
    available = [];
    mountPage();
    await advanceToRepos();
    fireEvent.click(screen.getByRole("button", { name: "Load repositories" }));

    await screen.findByText(/No available forgejo repositories found/);
    expect(
      (screen.getByRole("button", { name: "Add 0 selected" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Continue to tone" }));
    await screen.findByRole("heading", { name: "Choose a default tone." });

  });

  it("failed discovery: continuation remains available", async () => {
    stubFetch();
    available = [forgeRepo("team", "app")];
    availableOk = false;
    mountPage();
    await advanceToRepos();
    fireEvent.click(screen.getByRole("button", { name: "Load repositories" }));

    const continueButton = await screen.findByRole("button", {
      name: "Continue to tone",
    }) as HTMLButtonElement;
    expect(continueButton.disabled).toBe(false);
    expect(availCalls).toHaveLength(1);

    fireEvent.click(continueButton);
    await screen.findByRole("heading", { name: "Choose a default tone." });

  });
});
