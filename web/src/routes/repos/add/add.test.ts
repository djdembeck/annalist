// Component behavior tests for the repository intake page's manual entry:
// the platform select must default to a configured platform, and an
// operator's explicit choice must survive later asynchronous reloads.
//
// Runs under both `bun test` (jsdom + .svelte client compilation via
// bunfig.toml preload) and `bunx vitest run` (jsdom + the sveltekit()
// vite plugin). The page is mounted with svelte's client `mount` and
// queried with @testing-library/dom directly; the API is stubbed at the
// fetch boundary so the POST body equals the payload addRepo would send.
import { describe, it, expect, afterEach, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/dom";
import { mount, unmount, tick } from "$lib/svelte-client";

import AddPage from "./+page.svelte";

vi.mock("$app/navigation", () => ({
  goto: vi.fn(async () => {}),
}));

type JsonInit = { method?: string; body?: string };

/** A vi.fn() used as the fetch stub; `.mock.calls` carries [input, init]. */
type FetchMockFn = {
  (input: string, init?: JsonInit): Promise<Response>;
  mock: { calls: [string, JsonInit?][] };
};

let fetchMock: FetchMockFn;

/** Toggle: status/availability health for the stubbed API. */
let apiOk = true;
let inventoryOk = true;
let available = [{ platform: "forgejo", owner: "team", repo: "app" }];
let addResponse: ((repo: string) => Promise<Response>) | null = null;

function jsonResponse(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function stubFetch(): void {
  apiOk = true;
  inventoryOk = true;
  available = [{ platform: "forgejo", owner: "team", repo: "app" }];
  addResponse = null;
  fetchMock = vi.fn(async (input: string, init?: JsonInit) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (method === "GET" && url.endsWith("/api/status")) {
      return apiOk
        ? jsonResponse({ github: false, forgejo: true, admin: true })
        : jsonResponse({ error: "boom" }, 500);
    }
    if (method === "GET" && url.endsWith("/api/repos/available")) {
      return apiOk && inventoryOk
        ? jsonResponse(available)
        : jsonResponse({ error: "boom" }, 500);
    }
    if (method === "POST" && url.endsWith("/api/repos")) {
      if (addResponse) {
        const body = JSON.parse(init?.body ?? "{}") as { repo: string };
        return addResponse(body.repo);
      }
      return jsonResponse({ id: 1 }, 201);
    }
    throw new Error(`Unexpected request: ${url}`);
  }) as FetchMockFn;
  vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock as typeof fetch);
}

let container: HTMLElement | null = null;
let component: ReturnType<typeof mount> | null = null;

async function mountPage(): Promise<void> {
  container = document.createElement("div");
  document.body.append(container);
  component = mount(AddPage, { target: container });
}

function teardown(): void {
  if (component) {
    unmount(component);
    component = null;
  }
  container?.remove();
  container = null;
  vi.restoreAllMocks();
  vi.clearAllMocks();
}

afterEach(teardown);

describe("repository intake batch progress", () => {
  it("reconciles pending rows and retries only failures without stale pending results", async () => {
    stubFetch();
    available.push({ platform: "forgejo", owner: "team", repo: "widgets" });
    const pending = new Map<string, (response: Response) => void>();
    const posts: string[] = [];
    addResponse = (repo) => {
      posts.push(repo);
      const { promise, resolve } = Promise.withResolvers<Response>();
      pending.set(repo, resolve);
      return promise;
    };
    await mountPage();
    const rows = await screen.findAllByRole("checkbox", { name: /team\// }) as HTMLInputElement[];
    for (const row of rows) fireEvent.click(row);
    await tick();
    fireEvent.click(screen.getAllByRole("button", { name: "Add 2 selected" })[0]);
    await waitFor(() => expect(screen.getAllByText("Adding…", { selector: "span" })).toHaveLength(2));
    expect(rows.every((row) => row.disabled)).toBe(true);
    pending.get("app")!(jsonResponse({ id: 1 }, 201));
    await screen.findByText("Added", { selector: "span" });
    expect(screen.getAllByText("Adding…", { selector: "span" })).toHaveLength(1);
    pending.get("widgets")!(jsonResponse({ error: "boom" }, 500));
    await screen.findByRole("button", { name: "Retry 1 failed" });
    expect(screen.queryByText("Adding…", { selector: "span" })).toBeNull();
    expect(screen.getByText(/1 added,\s*1 failed/)).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "Retry 1 failed" }));
    await screen.findByText("Adding…", { selector: "span" });
    expect(screen.getByText("Added", { selector: "span" })).toBeDefined();
    pending.get("widgets")!(jsonResponse({ error: "still failing" }, 500));
    await screen.findByRole("button", { name: "Retry 1 failed" });
    expect(screen.queryByText("Adding…", { selector: "span" })).toBeNull();
    expect(screen.getByText("Request failed: 500")).toBeDefined();
    expect(posts).toEqual(["app", "widgets", "widgets"]);
  });
});

describe("repository intake manual entry", () => {
  it("defaults to the configured platform when status succeeds but inventory fails", async () => {
    stubFetch();
    inventoryOk = false;
    await mountPage();
    await screen.findByText("Intake failed");
    const select = screen.getByRole("combobox", { name: "Platform" }) as HTMLSelectElement;
    expect(select.value).toBe("forgejo");
    const forgejo = Array.from(select.options).find((option) => option.value === "forgejo");
    const github = Array.from(select.options).find((option) => option.value === "github");
    expect(forgejo?.disabled).toBe(false);
    expect(forgejo?.textContent).toBe("forgejo");
    expect(github).toBeDefined();
    // Unconfigured source is labeled and disabled, so manual entry cannot
    // target it into a server-side failure.
    expect(github?.disabled).toBe(true);
    expect(github?.textContent).toContain("not configured");
    inventoryOk = true;
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await screen.findByText("1 repo discoverable");
  });

  it("defaults the platform to the configured source and disables unconfigured sources", async () => {
    stubFetch();
    await mountPage();
    const select = await screen.findByRole("combobox", { name: "Platform" }) as HTMLSelectElement;

    // Forgejo-only instance: manual entry mirrors discovery's default.
    await waitFor(() => {
      expect(select.value).toBe("forgejo");
    });

    // GitHub is not configured: a selectable option would fail server-side,
    // so it stays labeled and disabled.
    const options = Array.from(
      (select as HTMLSelectElement).options,
    ) as HTMLOptionElement[];
    const github = options.find((o) => o.value === "github");
    expect(github).toBeDefined();
    expect(github?.disabled).toBe(true);
    expect(options.find((o) => o.value === "forgejo")?.disabled).toBe(false);
    // The unconfigured source is explicitly labeled, the configured one is not.
    expect(options.find((o) => o.value === "forgejo")?.textContent).toBe(
      "forgejo",
    );
    expect(github?.textContent).toContain("not configured");
  });

  it("keeps an explicit platform choice through asynchronous reloads and submits it", async () => {
    stubFetch();
    apiOk = false;
    await mountPage();
    // Failed intake: the error branch still exposes manual entry.
    const select = await screen.findByRole("combobox", { name: "Platform" }) as HTMLSelectElement;

    // Before status loads, both sources are unknown rather than known-unconfigured.
    const unknownOptions = Array.from(select.options);
    expect(unknownOptions.every((option) => !option.textContent?.includes("not configured"))).toBe(true);

    // Operator preconfigures GitHub before any platform is known.
    fireEvent.change(select, { target: { value: "github" } });

    // Discovery recovers (async update): the explicit choice must survive.
    apiOk = true;
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await screen.findByText("1 repo discoverable");
    const recoveredSelect = screen.getByRole("combobox", { name: "Platform" }) as HTMLSelectElement;
    expect((recoveredSelect as HTMLSelectElement).value).toBe("github");

    fireEvent.input(
      screen.getByRole("textbox", { name: "Owner" }),
      { target: { value: "acme" } },
    );
    fireEvent.input(
      screen.getByRole("textbox", { name: "Repository" }),
      { target: { value: "svc" } },
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Add manually" }),
    );
    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        ([url, init]) =>
          init?.method === "POST" && url.endsWith("/api/repos"),
      );
      if (!post) throw new Error("no POST observed");
      expect(JSON.parse(post[1]?.body ?? "{}")).toEqual({
        platform: "github",
        owner: "acme",
        repo: "svc",
      });
    });
  });

  it("blocks manual entry on an instance with no configured platforms", async () => {
    stubFetch();
    // Neither source is configured, so load() never defaults manualPlatform.
    const posts: string[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(
      vi.fn(async (input: string, init?: JsonInit) => {
        const url = String(input);
        const method = init?.method ?? "GET";
        if (method === "GET" && url.endsWith("/api/status"))
          return jsonResponse({ github: false, forgejo: false, admin: true });
        if (method === "GET" && url.endsWith("/api/repos/available"))
          return jsonResponse([]);
        if (method === "POST" && url.endsWith("/api/repos")) {
          posts.push(url);
          return jsonResponse({ id: 1 }, 201);
        }
        throw new Error(`Unexpected request: ${url}`);
      }) as typeof fetch,
    );
    await mountPage();
    await screen.findByText("No platform is configured.");
    const select = screen.getByRole("combobox", { name: "Platform" });
    expect((select as HTMLSelectElement).value).toBe("");
    fireEvent.input(screen.getByRole("textbox", { name: "Owner" }), {
      target: { value: "acme" },
    });
    fireEvent.input(screen.getByRole("textbox", { name: "Repository" }), {
      target: { value: "svc" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add manually" }));
    await waitFor(() =>
      expect(
        screen.getByText("Choose a platform before adding."),
      ).toBeInstanceOf(HTMLElement),
    );
    expect(posts).toHaveLength(0);
  });
});
