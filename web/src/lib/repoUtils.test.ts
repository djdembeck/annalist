import { describe, expect, it } from "vitest";

import { parseMaxTokens } from "./repoUtils";

describe("parseMaxTokens", () => {
  it("treats blank and NaN values as inherit (null, no error)", () => {
    // A blanked Svelte number-input hands back "", null, or NaN — all mean
    // "inherit the server default". Non-numeric text is also coerced to NaN.
    for (const value of [null, "", NaN, "abc"]) {
      expect(parseMaxTokens(value)).toEqual({ value: null, error: "" });
    }
  });

  it("parses whole numbers of 1 or more, strings and numbers alike", () => {
    const cases: [string | number, number][] = [
      ["1", 1],
      ["4096", 4096],
      ["999999", 999999],
      [4096, 4096],
      [" 8192 ", 8192],
    ];
    for (const [input, expected] of cases) {
      expect(parseMaxTokens(input)).toEqual({ value: expected, error: "" });
    }
  });

  it("rejects zero, negative, and fractional values", () => {
    // "   " is not blank to Number(): it coerces to 0 and hits the range
    // check rather than the inherit path.
    for (const value of [0, "0", -1, "-1", 1.5, "1.5", "   "]) {
      expect(parseMaxTokens(value)).toEqual({
        value: null,
        error: "Enter a whole number of 1 or more.",
      });
    }
  });
});
