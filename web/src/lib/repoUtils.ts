// Shared repository utilities used by both the setup wizard and the repos/add page.
// Eliminates duplication of filter/sort, key generation, and batch add logic.

import type { AvailableRepo, Repo, Status } from "$lib/api";

export type Source = "github" | "forgejo";
export type RowStatus = "pending" | "success" | "failure";

const SOURCES: Source[] = ["github", "forgejo"];
export { SOURCES };

export const SAVE_MSG_TIMEOUT = 2500;

/** Return a stable key for a repo (platform/owner/repo). Works for both Repo and AvailableRepo. */
export function repoKey(r: Repo | AvailableRepo): string {
  return `${r.platform}/${r.owner}/${r.repo}`;
}

/** Parse and validate a temperature string value (0–2). */
export function parseTemperature(value: string): {
  value: number | null;
  error: string;
} {
  if (value === "") return { value: null, error: "" };
  const num = Number(value);
  if (isNaN(num))
    return { value: null, error: "Enter a number between 0 and 2." };
  if (num < 0 || num > 2)
    return { value: null, error: "Temperature must be between 0 and 2." };
  return { value: num, error: "" };
}

/**
 * Resolve the effective tone string or null from a UI tone selection.
 *
 * - "inherit" → null (use server default)
 * - "custom" → the custom tone string, or null if blank
 * - anything else (preset name) → the preset name itself
 */
export function resolveTone(
  toneOption: string,
  customTone: string,
): string | null {
  if (toneOption === "inherit") return null;
  if (toneOption === "custom")
    return customTone.trim() ? customTone.trim() : null;
  return toneOption;
}

/** Check if a named source platform is connected. */
export function isSourceEnabled(status: Status | null, s: Source): boolean {
  if (!status) return false;
  if (s === "github") return status.github;
  if (s === "forgejo") return status.forgejo;
  return false;
}

/** Return the list of source platforms that are currently connected. */
export function getEnabledSources(status: Status | null): Source[] {
  return SOURCES.filter((s) => isSourceEnabled(status, s));
}

/**
 * Filter and sort a list of available repos.
 * Returns a new sorted array — does not mutate `repos`.
 */
export function filterAndSortRepos(
  repos: AvailableRepo[],
  source: Source,
  query: string,
  showForks: boolean,
  showSharedNamespaces: boolean,
  sortBy: "name" | "activity",
): AvailableRepo[] {
  let result = repos.filter((r) => r.platform === source);
  if (!showSharedNamespaces)
    result = result.filter((r) => r.ownNamespace !== false);
  if (!showForks) result = result.filter((r) => r.fork !== true);
  const q = query.trim().toLowerCase();
  if (q)
    result = result.filter((r) =>
      `${r.owner}/${r.repo}`.toLowerCase().includes(q),
    );

  if (sortBy === "activity") {
    return result.sort((a, b) => {
      const aT = new Date(a.pushedAt ?? a.updatedAt ?? 0).getTime();
      const bT = new Date(b.pushedAt ?? b.updatedAt ?? 0).getTime();
      if (aT !== bT) return bT - aT;
      if (a.owner !== b.owner) return a.owner.localeCompare(b.owner);
      return a.repo.localeCompare(b.repo);
    });
  }
  return result.sort((a, b) => {
    const aOwn = a.ownNamespace === false ? 1 : 0;
    const bOwn = b.ownNamespace === false ? 1 : 0;
    if (aOwn !== bOwn) return aOwn - bOwn;
    const aFork = a.fork === true ? 1 : 0;
    const bFork = b.fork === true ? 1 : 0;
    if (aFork !== bFork) return aFork - bFork;
    if (a.owner !== b.owner) return a.owner.localeCompare(b.owner);
    return a.repo.localeCompare(b.repo);
  });
}

export type BatchResult = Record<string, { status: RowStatus; error?: string }>;

/**
 * Callback invoked when an individual add returns "Unauthorized".
 * Return true to abort the whole batch; return false to continue.
 */
export type UnauthorizedCallback = () => boolean;

/**
 * Add a batch of selected repos one-by-one.
 *
 * @returns number of repos successfully added.
 */
export async function batchAddRepos(
  targets: AvailableRepo[],
  results: BatchResult,
  addRepoFn: (r: AvailableRepo) => Promise<unknown>,
  unauthorizedCb?: UnauthorizedCallback,
): Promise<number> {
  let successCount = 0;
  for (const r of targets) {
    const key = repoKey(r);
    try {
      await addRepoFn(r);
      results[key] = { status: "success" };
      successCount++;
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        if (unauthorizedCb && unauthorizedCb()) {
          return successCount;
        }
      }
      results[key] = {
        status: "failure",
        error: e instanceof Error ? e.message : "Failed to add",
      };
    }
  }
  return successCount;
}

/**
 * Retry only the repos whose result key is in `failedKeys`.
 * Looks up each repo from `available` by key.
 *
 * @returns number of repos successfully added on retry.
 */
export async function retryFailedRepos(
  available: AvailableRepo[],
  failedKeys: string[],
  results: BatchResult,
  addRepoFn: (r: AvailableRepo) => Promise<unknown>,
  unauthorizedCb?: UnauthorizedCallback,
): Promise<number> {
  let successCount = 0;
  for (const key of failedKeys) {
    const r = available.find((r) => repoKey(r) === key);
    if (!r) {
      results[key] = {
        status: "failure",
        error: "Repository no longer found in inventory",
      };
      continue;
    }
    try {
      await addRepoFn(r);
      results[key] = { status: "success" };
      successCount++;
    } catch (e) {
      if (e instanceof Error && e.message === "Unauthorized") {
        if (unauthorizedCb && unauthorizedCb()) {
          return successCount;
        }
      }
      results[key] = {
        status: "failure",
        error: e instanceof Error ? e.message : "Failed to add",
      };
    }
  }
  return successCount;
}
