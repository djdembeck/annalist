export type CommitTypeOption = {
  value: string;
  description: string;
};

export const COMMIT_TYPE_OPTIONS: CommitTypeOption[] = [
  { value: "fix", description: "Bug fixes" },
  { value: "feat", description: "New features" },
  { value: "refactor", description: "Code changes" },
  { value: "perf", description: "Performance" },
  { value: "docs", description: "Documentation" },
  { value: "test", description: "Test changes" },
  { value: "build", description: "Build system" },
  { value: "ci", description: "CI changes" },
  { value: "chore", description: "Maintenance" },
  { value: "style", description: "Style changes" },
  { value: "revert", description: "Reverted changes" },
];

export const DEFAULT_COMMIT_TYPES = ["fix", "feat", "refactor", "perf"];

// Sentinel for "no filter — include all types"; round-trips through the Go
// server (FilterCommitLog keeps all on ""). See engine.DefaultCommitTypes
// for the separate blank-means-default rule.
export const KEEP_ALL = "*";

const KNOWN_COMMIT_TYPES = new Set(
  COMMIT_TYPE_OPTIONS.map((option) => option.value),
);

export function splitCommitTypes(value: string | null | undefined): string[] {
  if (!value?.trim()) return [];
  return value
    .split(",")
    .map((type) => type.trim())
    .filter(Boolean);
}

export function isKeepAll(value: string | null | undefined): boolean {
  // Mirrors FilterCommitLog: any include set containing "*" keeps all types,
  // so mixed values like "fix,*" are keep-all, not a filtered selection.
  const parsed = splitCommitTypes(value);
  return parsed.includes(KEEP_ALL);
}

export function getCommitTypeSelection(value: string | null | undefined): {
  selected: string[];
  custom: string[];
} {
  if (isKeepAll(value)) return { selected: [], custom: [] };
  const parsed = splitCommitTypes(value);
  const selected = COMMIT_TYPE_OPTIONS.filter((option) =>
    parsed.some((type) => type.toLowerCase() === option.value),
  ).map((option) => option.value);
  const custom = parsed.filter(
    (type) => !KNOWN_COMMIT_TYPES.has(type.toLowerCase()),
  );
  return { selected, custom };
}

export function formatCommitTypes(
  selected: string[],
  custom: string,
): string {
  const selectedSet = new Set(selected);
  const known = COMMIT_TYPE_OPTIONS.filter((option) =>
    selectedSet.has(option.value),
  ).map((option) => option.value);
  const extras = splitCommitTypes(custom).filter(
    (type) => !KNOWN_COMMIT_TYPES.has(type.toLowerCase()),
  );
  const result = [...known, ...extras];
  return result.length ? result.join(",") : KEEP_ALL;
}
