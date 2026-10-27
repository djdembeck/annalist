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

function repository(latest_run: GenerationRun | null = null): Repo {
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
});
