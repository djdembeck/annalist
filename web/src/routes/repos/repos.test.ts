import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/dom";
import { mount, unmount, tick } from "$lib/svelte-client";
import type { GenerationRun, Repo } from "$lib/api";
import RepositoriesPage from "./+page.svelte";

const failedRun: GenerationRun = {
  id: 1,
  platform: "github",
  owner: "team",
  repo: "app",
  source: "manual",
  to_tag: "v2.0.0",
  from_tag: "v1.0.0",
  profile: "",
  display_version: "",
  status: "failed",
  error_code: "publication",
  warning_code: null,
  started_at: "2026-10-26T10:00:00Z",
  finished_at: "2026-10-26T10:01:00Z",
};

function repository(
  latest_run: GenerationRun | null = null,
  overrides: Partial<Repo> = {},
): Repo {
  return {
    platform: "github",
    owner: "team",
    repo: "app",
    enabled: false,
    tone: null,
    instructions: null,
    model: null,
    temperature: null,
    trigger: "auto",
    commit_types: null,
    mode: null,
    max_tokens: 0,
    thinking_level: null,
    latest_run,
    effective: {
      tone: null,
      instructions: null,
      model: null,
      temperature: null,
      source: null,
      commit_types: null,
      mode: null,
      max_tokens: 0,
      thinking_level: null,
    },
    ...overrides,
  };
}

function json(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "content-type": "application/json" },
  });
}

const originalVisibility = Object.getOwnPropertyDescriptor(document, "visibilityState");
let component: Record<string, unknown> | undefined;
let container: HTMLElement | undefined;

async function mountPage(afterMount?: () => void): Promise<void> {
  container = document.createElement("div");
  document.body.append(container);
  component = mount(RepositoriesPage, { target: container });
  await tick();
  afterMount?.();
  await waitFor(() =>
    expect(
      screen.getByRole("checkbox", { name: "Enable team/app" }),
    ).toBeInstanceOf(HTMLInputElement),
  );
}

afterEach(async () => {
  if (component) await unmount(component);
  component = undefined;
  container?.remove();
  container = undefined;
  vi.restoreAllMocks();
  if (originalVisibility) {
    Object.defineProperty(document, "visibilityState", originalVisibility);
  } else {
    Reflect.deleteProperty(document, "visibilityState");
  }
});

describe("repository operator transitions", () => {
  it("queues a fresh post-write read behind an older poll without losing drafts", async () => {
    let visibility: DocumentVisibilityState = "hidden";
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => visibility,
    });
    let poll: (() => void) | undefined;
    const interval = vi.spyOn(window, "setInterval");
    let server = repository();
    let repoReads = 0;
    let modelReads = 0;
    let instructionReads = 0;
    let writes = 0;
    const delayedPoll = Promise.withResolvers<Response>();
    let releasePoll: (() => void) | undefined;
    vi.spyOn(globalThis, "fetch").mockImplementation(
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (init?.method === "PUT") {
          writes++;
          server = { ...server, ...JSON.parse(String(init.body)) };
          return json(server);
        }
        if (url.endsWith("/api/repos")) {
          repoReads++;
          if (repoReads === 2) {
            const oldSnapshot = { ...server };
            releasePoll = () => delayedPoll.resolve(json([oldSnapshot]));
            return delayedPoll.promise;
          }
          return json([server]);
        }
        if (url.endsWith("/api/models")) {
          modelReads++;
          return json([]);
        }
        if (url.endsWith("/in-repo-instructions")) {
          instructionReads++;
          return json({ instructions: null });
        }
        throw new Error(`Unexpected request: ${url}`);
      }),
    );
    await mountPage(() => {
      const [handler, , ...args] = interval.mock.calls[0] ?? [];
      if (typeof handler === "function") poll = () => handler(...args);
      interval.mockRestore();
    });
    await fireEvent.click(screen.getByRole("button", { name: "Settings" }));
    const instructions = screen.getByLabelText(
      /Instructions/,
    ) as HTMLTextAreaElement;
    await fireEvent.input(instructions, {
      target: { value: "unsaved operator draft" },
    });
    await waitFor(() => expect(instructionReads).toBe(1));
    poll?.();
    expect(repoReads).toBe(1);
    visibility = "visible";
    poll?.();
    await waitFor(() => expect(repoReads).toBe(2));
    expect(modelReads).toBe(1);
    expect(instructionReads).toBe(1);
    const enabled = screen.getByRole("checkbox", {
      name: "Enable team/app",
    }) as HTMLInputElement;
    await fireEvent.click(enabled);
    await waitFor(() => expect(writes).toBe(1));
    expect(server.enabled).toBe(true);
    expect(repoReads).toBe(2);
    expect(enabled.disabled).toBe(true);
    releasePoll?.();
    await waitFor(() => {
      expect(repoReads).toBe(3);
      expect(enabled.disabled).toBe(false);
      expect(enabled.checked).toBe(true);
    });
    expect(screen.getByText("On", { exact: true })).toBeInstanceOf(HTMLElement);
    expect(instructions.value).toBe("unsaved operator draft");
    expect(instructionReads).toBe(1);
    if (component) await unmount(component);
    component = undefined;
  });

  it("opens retries in preview by default and preserves explicit choices while requiring overwrite confirmation again", async () => {
    const requests: unknown[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith("/generate")) {
          requests.push(JSON.parse(String(init?.body)));
          return json({}, 500);
        }
        if (url.endsWith("/api/repos")) return json([repository(failedRun)]);
        if (url.endsWith("/api/models")) return json([]);
        throw new Error(`Unexpected request: ${url}`);
      }),
    );
    await mountPage();
    const retry = screen.getByRole("button", { name: "Retry generation" });
    await fireEvent.click(retry);
    const publish = screen.getByRole("checkbox", {
      name: "Publish to release",
    }) as HTMLInputElement;
    expect(publish.checked).toBe(false);
    expect(
      (screen.getByLabelText("Release tag") as HTMLInputElement).value,
    ).toBe("v2.0.0");
    await fireEvent.click(publish);
    await fireEvent.click(screen.getByRole("button", { name: /^Overwrite/ }));
    await fireEvent.click(
      screen.getByRole("button", { name: "Generate & publish" }),
    );
    expect(
      screen.getByRole("button", { name: "Yes, overwrite" }),
    ).toBeInstanceOf(HTMLButtonElement);
    await fireEvent.click(
      screen.getByRole("button", { name: "Yes, overwrite" }),
    );
    await waitFor(() => {
      expect(requests).toHaveLength(1);
      expect(screen.getByText(/Generation did not complete/)).toBeInstanceOf(
        HTMLElement,
      );
      expect(
        (
          screen.getByRole("button", {
            name: "Yes, overwrite",
          }) as HTMLButtonElement
        ).disabled,
      ).toBe(false);
    });
    for (let attempt = 0; attempt < 2; attempt++) {
      await fireEvent.click(retry);
      expect(
        screen.getByRole("region", { name: "Generate a note for team/app" }),
      ).toBeInstanceOf(HTMLElement);
      expect(publish.checked).toBe(true);
      expect(
        screen
          .getByRole("button", { name: /^Overwrite/ })
          .getAttribute("aria-pressed"),
      ).toBe("true");
      expect(
        screen.queryByRole("button", { name: "Yes, overwrite" }),
      ).toBeNull();
      await fireEvent.click(
        screen.getByRole("button", { name: "Generate & publish" }),
      );
      expect(
        screen.getByRole("button", { name: "Yes, overwrite" }),
      ).toBeInstanceOf(HTMLButtonElement);
      expect(requests).toHaveLength(1);
    }
    expect(requests[0]).toMatchObject({
      to_tag: "v2.0.0",
      publish: true,
      force: true,
    });
  });

  it("omits empty legacy profiles but retains named profiles in run history", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.endsWith("/api/repos")) return json([repository(failedRun)]);
        if (url.endsWith("/api/models")) return json([]);
        if (url.includes("/api/runs?"))
          return json([
            failedRun,
            { ...failedRun, id: 2, profile: "technical" },
          ]);
        throw new Error(`Unexpected request: ${url}`);
      }),
    );
    await mountPage();
    await fireEvent.click(screen.getByRole("button", { name: "Recent runs" }));
    await waitFor(() =>
      expect(screen.getByText("technical", { exact: true })).toBeInstanceOf(
        HTMLElement,
      ),
    );
    expect(screen.getAllByText("Profile", { exact: true })).toHaveLength(1);
  });

  it("forwards a failed run's from_tag/profile/display_version on retry and nulls them for a fresh generation", async () => {
    const lineageRun: GenerationRun = {
      ...failedRun,
      profile: "technical",
      display_version: "2.0",
    };
    const requests: Record<string, unknown>[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith("/generate")) {
          requests.push(JSON.parse(String(init?.body)));
          return json({
            notes: "NOTES",
            release_id: null,
            published: false,
            outcome: "generated",
            warning_code: null,
            profile: lineageRun.profile,
            display_version: "2.0",
            from_tag: "v1.0.0",
            to_tag: "v2.0.0",
            config_digest: "",
          });
        }
        if (url.endsWith("/api/repos")) return json([repository(lineageRun)]);
        if (url.endsWith("/api/models")) return json([]);
        throw new Error(`Unexpected request: ${url}`);
      }),
    );
    await mountPage();
    // Retry keeps the failed run's contract intact.
    await fireEvent.click(screen.getByRole("button", { name: "Retry generation" }));
    expect(
      (screen.getByLabelText("Release tag") as HTMLInputElement).value,
    ).toBe("v2.0.0");
    await fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(requests).toHaveLength(1));
    expect(requests[0]).toEqual({
      to_tag: "v2.0.0",
      from_tag: "v1.0.0",
      profile: "technical",
      display_version: "2.0",
      force: false,
      publish: false,
    });
    // A fresh manual generation carries no lineage. The in-flight run sets
    // the generating flag while it re-reads the list, so wait it out.
    await fireEvent.click(screen.getByRole("button", { name: "Close generation" }));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Generating…" })).toBeNull(),
    );
    await fireEvent.click(screen.getByRole("button", { name: "Generate notes" }));
    await tick();
    await fireEvent.input(screen.getByLabelText("Release tag"), {
      target: { value: "v1.3.0" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(requests).toHaveLength(2));
    expect(requests[1]).toEqual({
      to_tag: "v1.3.0",
      from_tag: null,
      profile: "",
      display_version: null,
      force: false,
      publish: false,
    });
  });

  it("keeps the manual Refresh button enabled and unlabelled while a background poll is pending", async () => {
    let poll: (() => void) | undefined;
    const interval = vi.spyOn(window, "setInterval");
    let repoReads = 0;
    const delayedPoll = Promise.withResolvers<Response>();
    let releasePoll: (() => void) | undefined;
    vi.spyOn(globalThis, "fetch").mockImplementation(
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.endsWith("/api/repos")) {
          repoReads++;
          if (repoReads === 2) {
            releasePoll = () => delayedPoll.resolve(json([repository()]));
            return delayedPoll.promise;
          }
          return json([repository()]);
        }
        if (url.endsWith("/api/models")) return json([]);
        throw new Error(`Unexpected request: ${url}`);
      }),
    );
    await mountPage(() => {
      const [handler, , ...args] = interval.mock.calls[0] ?? [];
      if (typeof handler === "function") poll = () => handler(...args);
      interval.mockRestore();
    });
    poll?.();
    await waitFor(() => expect(repoReads).toBe(2));
    const refresh = screen.getByRole("button", {
      name: "Refresh",
    }) as HTMLButtonElement;
    expect(refresh.disabled).toBe(false);
    expect(refresh.textContent).toBe("Refresh");
    // A manual click during a pending background flight just queues the next
    // read; it must not disable or relabel the button.
    await fireEvent.click(refresh);
    expect(refresh.disabled).toBe(false);
    expect(refresh.textContent).toBe("Refresh");
    releasePoll?.();
    await waitFor(() => expect(repoReads).toBe(3));
    expect(refresh.disabled).toBe(false);
    expect(refresh.textContent).toBe("Refresh");
  });

  it("keeps the load error banner in place when a background poll also fails", async () => {
    let poll: (() => void) | undefined;
    const interval = vi.spyOn(window, "setInterval");
    let repoReads = 0;
    vi.spyOn(globalThis, "fetch").mockImplementation(
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.endsWith("/api/repos")) {
          repoReads++;
          return json({ error: "upstream exploded" }, 500);
        }
        if (url.endsWith("/api/models")) return json([]);
        throw new Error(`Unexpected request: ${url}`);
      }),
    );
    container = document.createElement("div");
    document.body.append(container);
    component = mount(RepositoriesPage, { target: container });
    await tick();
    const [handler, , ...args] = interval.mock.calls[0] ?? [];
    if (typeof handler === "function") poll = () => handler(...args);
    interval.mockRestore();
    await waitFor(() =>
      expect(screen.getByRole("alert")).toBeInstanceOf(HTMLElement),
    );
    poll?.();
    await waitFor(() => expect(repoReads).toBe(2));
    // The failed flight clears the banner before it re-raises it, so wait
    // for the alert to settle back in place.
    await waitFor(() =>
      expect(screen.getByRole("alert")).toBeInstanceOf(HTMLElement),
    );
    // The initial 500 must not have been swapped for a loading skeleton.
    expect(screen.queryByText("Loading repositories...")).toBeNull();
  });

  it("recovers the repository list from a failed initial load via the banner retry", async () => {
    let repoReads = 0;
    vi.spyOn(globalThis, "fetch").mockImplementation(
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.endsWith("/api/repos")) {
          repoReads++;
          if (repoReads === 1) return json({ error: "upstream exploded" }, 500);
          return json([repository()]);
        }
        if (url.endsWith("/api/models")) return json([]);
        throw new Error(`Unexpected request: ${url}`);
      }),
    );
    container = document.createElement("div");
    document.body.append(container);
    component = mount(RepositoriesPage, { target: container });
    await tick();
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Could not refresh repositories");
    // The server body is never surfaced to the client; the detail is the
    // formatted request failure (jsdom responses carry no status text).
    expect(alert.textContent).toContain("Request failed: 500");
    await fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() =>
      expect(
        screen.getByRole("checkbox", { name: "Enable team/app" }),
      ).toBeInstanceOf(HTMLInputElement),
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("re-seeds clean settings drafts from the server row after a refresh instead of a stale local draft", async () => {
    let server = repository(null, { instructions: "old" });
    let repoReads = 0;
    vi.spyOn(globalThis, "fetch").mockImplementation(
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (init?.method === "PUT") return json(server);
        if (url.endsWith("/api/repos")) {
          repoReads++;
          return json([{ ...server }]);
        }
        if (url.endsWith("/api/models")) return json([]);
        if (url.endsWith("/in-repo-instructions"))
          return json({ instructions: null });
        throw new Error(`Unexpected request: ${url}`);
      }),
    );
    await mountPage();
    await fireEvent.click(screen.getByRole("button", { name: "Settings" }));
    const openedOnce = screen.getByLabelText(/Instructions/);
    expect(openedOnce).toBeInstanceOf(HTMLTextAreaElement);
    expect((openedOnce as HTMLTextAreaElement).value).toBe("old");
    // Close without editing: the draft matches its baseline, so it is
    // discarded on reopen and the server row is the only source of truth.
    await fireEvent.click(screen.getByRole("button", { name: "Close settings" }));
    server = { ...server, instructions: "new" };
    await fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    // The read counter advances at request time; the row update commits in
    // the flight's continuation, so wait for the flight to settle.
    await waitFor(() => expect(repoReads).toBe(2));
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Refreshing…" }),
      ).toBeNull(),
    );
    await fireEvent.click(screen.getByRole("button", { name: "Settings" }));
    const reopened = screen.getByLabelText(/Instructions/) as HTMLTextAreaElement;
    expect(reopened.value).toBe("new");
  });

  it.each([
    ["published", "Notes published to the release."],
    ["generated", "Preview generated. Nothing was published."],
    ["cached", "Cached notes returned. No new publication was made."],
    ["preserved", "The existing release body was preserved. Nothing was overwritten."],
  ] as const)("renders the %s outcome message on the generate panel", async (outcome, wantText) => {
    vi.spyOn(globalThis, "fetch").mockImplementation(
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.endsWith("/generate"))
          return json({
            notes: "NOTES",
            release_id: null,
            published: false,
            outcome,
            warning_code: null,
            profile: "",
            display_version: "",
            from_tag: "",
            to_tag: "v1.0.0",
            config_digest: "",
          });
        if (url.endsWith("/api/repos")) return json([repository()]);
        if (url.endsWith("/api/models")) return json([]);
        throw new Error(`Unexpected request: ${url}`);
      }),
    );
    await mountPage();
    await fireEvent.click(screen.getByRole("button", { name: "Generate notes" }));
    await fireEvent.input(screen.getByLabelText("Release tag"), {
      target: { value: "v1.0.0" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toBe(wantText);
    });
  });

  it("appends the cache persistence warning sentence to a published outcome", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.endsWith("/generate"))
          return json({
            notes: "NOTES",
            release_id: null,
            published: true,
            outcome: "published",
            warning_code: "cache_persistence",
            profile: "",
            display_version: "",
            from_tag: "",
            to_tag: "v1.0.0",
            config_digest: "",
          });
        if (url.endsWith("/api/repos")) return json([repository()]);
        if (url.endsWith("/api/models")) return json([]);
        throw new Error(`Unexpected request: ${url}`);
      }),
    );
    await mountPage();
    await fireEvent.click(screen.getByRole("button", { name: "Generate notes" }));
    await fireEvent.input(screen.getByLabelText("Release tag"), {
      target: { value: "v1.0.0" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toBe(
        "Notes published to the release. Published, but the local note cache " +
          "could not be saved. Check server storage before generating again.",
      );
    });
  });
});
