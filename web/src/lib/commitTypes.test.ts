import { describe, expect, it } from "vitest";
import {
  KEEP_ALL,
  formatCommitTypes,
  getCommitTypeSelection,
  isKeepAll,
  splitCommitTypes,
} from "./commitTypes";

describe("commit type selection helpers", () => {
  it("preserves null and empty values as the unset all-types state", () => {
    for (const value of [null, undefined, "", "   "]) {
      const selection = getCommitTypeSelection(value);
      expect(selection).toEqual({ selected: [], custom: [] });
      expect(
        formatCommitTypes(selection.selected, selection.custom.join(",")),
      ).toBe(KEEP_ALL);
    }
  });

  it("normalizes known types case-insensitively and keeps option order", () => {
    expect(getCommitTypeSelection(" FeAt, FIX, PERF ")).toEqual({
      selected: ["fix", "feat", "perf"],
      custom: [],
    });
  });

  it("separates custom types without losing their spelling", () => {
    expect(getCommitTypeSelection("fix,Security, release-note")).toEqual({
      selected: ["fix"],
      custom: ["Security", "release-note"],
    });
  });

  it("serializes selected known and custom types canonically", () => {
    expect(formatCommitTypes(["feat", "fix", "docs"], "Security, fix")).toBe(
      "fix,feat,docs,Security",
    );
  });

  it("serializes an empty selection as the keep-all sentinel", () => {
    expect(formatCommitTypes([], "")).toBe(KEEP_ALL);
    expect(formatCommitTypes([], " , ")).toBe(KEEP_ALL);
  });

  it("recognizes the keep-all sentinel and loads it as an empty selection", () => {
    expect(isKeepAll("*")).toBe(true);
    expect(isKeepAll(null)).toBe(false);
    expect(getCommitTypeSelection("*")).toEqual({ selected: [], custom: [] });
  });

  it("treats any mixed value containing the wildcard as keep-all, like FilterCommitLog", () => {
    expect(isKeepAll("fix,*")).toBe(true);
    expect(isKeepAll("*,fix")).toBe(true);
    expect(isKeepAll("*,fix,feat")).toBe(true);
    // Mixed wildcard values normalize to the keep-all empty selection so the
    // selector shows every box unchecked and serializes back to "*".
    expect(getCommitTypeSelection("fix,*")).toEqual({
      selected: [],
      custom: [],
    });
    expect(getCommitTypeSelection("*,Security")).toEqual({
      selected: [],
      custom: [],
    });
    const selection = getCommitTypeSelection("fix,*");
    expect(
      formatCommitTypes(selection.selected, selection.custom.join(",")),
    ).toBe(KEEP_ALL);
  });

  it("round-trips a configured filter without changing its meaning", () => {
    const selection = getCommitTypeSelection("FEAT,fix,Security");
    expect(
      formatCommitTypes(selection.selected, selection.custom.join(",")),
    ).toBe("fix,feat,Security");
  });

  it("splits and trims comma-separated values", () => {
    expect(splitCommitTypes(" fix, ,feat ,docs ")).toEqual([
      "fix",
      "feat",
      "docs",
    ]);
  });
});
