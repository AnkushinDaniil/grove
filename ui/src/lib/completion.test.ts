import { describe, expect, it } from "vitest";
import {
  decideTab,
  ensureTrailingSlash,
  longestCommonPrefix,
  nextIndex,
  prevIndex,
} from "./completion";

describe("longestCommonPrefix", () => {
  it("returns '' for an empty list", () => {
    expect(longestCommonPrefix([])).toBe("");
  });

  it("returns the single item unchanged", () => {
    expect(longestCommonPrefix(["/Users/dan/code"])).toBe("/Users/dan/code");
  });

  it("returns the shared prefix of several paths", () => {
    expect(longestCommonPrefix(["/Users/dan/code", "/Users/dan/config"])).toBe("/Users/dan/co");
  });

  it("returns '' when the very first character differs", () => {
    expect(longestCommonPrefix(["abc", "xyz"])).toBe("");
  });

  it("compares case-insensitively but preserves the first suggestion's casing", () => {
    // "Alpha" vs "alpaca": common (case-insensitive) prefix is 3 chars; casing
    // comes from the first item.
    expect(longestCommonPrefix(["Alpha", "alpaca"])).toBe("Alp");
  });

  it("keeps the whole first item when items differ only in case", () => {
    expect(longestCommonPrefix(["FOO", "foo"])).toBe("FOO");
  });

  it("handles unicode path segments", () => {
    expect(longestCommonPrefix(["/tmp/café/one", "/tmp/café/two"])).toBe("/tmp/café/");
  });
});

describe("ensureTrailingSlash", () => {
  it("appends a slash when missing", () => {
    expect(ensureTrailingSlash("/a/b")).toBe("/a/b/");
  });

  it("leaves an existing trailing slash intact", () => {
    expect(ensureTrailingSlash("/a/b/")).toBe("/a/b/");
  });
});

describe("nextIndex / prevIndex", () => {
  it("returns -1 for an empty list", () => {
    expect(nextIndex(-1, 0)).toBe(-1);
    expect(prevIndex(-1, 0)).toBe(-1);
    expect(nextIndex(2, 0)).toBe(-1);
  });

  it("steps from no-selection to the first (next) or last (prev) row", () => {
    expect(nextIndex(-1, 3)).toBe(0);
    expect(prevIndex(-1, 3)).toBe(2);
  });

  it("advances and wraps forward", () => {
    expect(nextIndex(0, 3)).toBe(1);
    expect(nextIndex(2, 3)).toBe(0);
  });

  it("retreats and wraps backward", () => {
    expect(prevIndex(2, 3)).toBe(1);
    expect(prevIndex(0, 3)).toBe(2);
  });
});

describe("decideTab", () => {
  it("completes a single suggestion with a trailing slash", () => {
    expect(decideTab("/Users/dan/co", ["/Users/dan/code"])).toEqual({
      kind: "complete",
      value: "/Users/dan/code/",
    });
  });

  it("extends to the common prefix when it grows the input", () => {
    expect(
      decideTab("/Users/dan/c", ["/Users/dan/code", "/Users/dan/config"]),
    ).toEqual({ kind: "extend", value: "/Users/dan/co" });
  });

  it("cycles when the common prefix cannot extend the input further", () => {
    // Input already equals the common prefix (base ""), so Tab cycles instead.
    expect(
      decideTab("/Users/dan/", ["/Users/dan/code", "/Users/dan/docs"]),
    ).toEqual({ kind: "cycle" });
  });
});
