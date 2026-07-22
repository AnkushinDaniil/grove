import { describe, expect, it } from "vitest";
import { draftsForAnchor, resolveLineAnchor, threadsForAnchor } from "./reviewDiff";
import type { DiffLine, DraftComment, ReviewThread } from "../gen/types";

function line(partial: Partial<DiffLine>): DiffLine {
  return { op: " ", old_line: 0, new_line: 0, text: "", ...partial };
}

function thread(partial: Partial<ReviewThread>): ReviewThread {
  return { id: "t1", path: "a.ts", line: 1, side: "RIGHT", is_resolved: false, diff_hunk: "", comments: [], ...partial };
}

function draft(partial: Partial<DraftComment>): DraftComment {
  return { id: "d1", dir: "/x", pr: 1, path: "a.ts", line: 1, side: "RIGHT", body: "", created_at: "", ...partial };
}

describe("resolveLineAnchor", () => {
  it("anchors an added line to RIGHT/new_line", () => {
    expect(resolveLineAnchor(line({ op: "+", new_line: 10 }))).toEqual({ side: "RIGHT", line: 10 });
  });

  it("anchors a context line to RIGHT/new_line", () => {
    expect(resolveLineAnchor(line({ op: " ", old_line: 5, new_line: 10 }))).toEqual({ side: "RIGHT", line: 10 });
  });

  it("anchors a pure deletion to LEFT/old_line", () => {
    expect(resolveLineAnchor(line({ op: "-", old_line: 7, new_line: 0 }))).toEqual({ side: "LEFT", line: 7 });
  });

  it("returns null when neither line number is set", () => {
    expect(resolveLineAnchor(line({ old_line: 0, new_line: 0 }))).toBeNull();
  });
});

describe("threadsForAnchor / draftsForAnchor", () => {
  const threads = [
    thread({ id: "t-right-10", path: "a.ts", side: "RIGHT", line: 10 }),
    thread({ id: "t-left-7", path: "a.ts", side: "LEFT", line: 7 }),
    thread({ id: "t-other-file", path: "b.ts", side: "RIGHT", line: 10 }),
  ];
  const drafts = [
    draft({ id: "d-right-10", path: "a.ts", side: "RIGHT", line: 10 }),
    draft({ id: "d-left-7", path: "a.ts", side: "LEFT", line: 7 }),
  ];

  it("matches only the thread(s) at the exact file+side+line", () => {
    expect(threadsForAnchor(threads, "a.ts", { side: "RIGHT", line: 10 }).map((t) => t.id)).toEqual(["t-right-10"]);
  });

  it("does not cross LEFT/RIGHT sides at the same line number", () => {
    expect(threadsForAnchor(threads, "a.ts", { side: "RIGHT", line: 7 })).toEqual([]);
  });

  it("does not cross files", () => {
    expect(threadsForAnchor(threads, "z.ts", { side: "RIGHT", line: 10 })).toEqual([]);
  });

  it("matches drafts the same way", () => {
    expect(draftsForAnchor(drafts, "a.ts", { side: "LEFT", line: 7 }).map((d) => d.id)).toEqual(["d-left-7"]);
    expect(draftsForAnchor(drafts, "a.ts", { side: "RIGHT", line: 7 })).toEqual([]);
  });
});
