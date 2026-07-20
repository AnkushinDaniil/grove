import { beforeEach, describe, expect, it } from "vitest";
import { _resetRollupCacheForTests, computeRollup } from "./rollup";
import type { Node } from "../gen/types";

function makeNode(id: string, parentId: string, overrides: Partial<Node> = {}): Node {
  return {
    id,
    parent_id: parentId,
    kind: "task",
    title: id,
    brief: "",
    status: "idle",
    attention: "none",
    attention_reason: "",
    driver: "",
    profile_id: "",
    current_session_id: "",
    workspace_dir: "",
    meta: {},
    position: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

// root
// ├─ a (running)
// │  └─ a1 (done, attention=done)
// └─ b (failed, attention=error)
const nodesById: Record<string, Node> = {
  root: makeNode("root", "", { kind: "workspace" }),
  a: makeNode("a", "root", { status: "running" }),
  a1: makeNode("a1", "a", { status: "done", attention: "done" }),
  b: makeNode("b", "root", { status: "failed", attention: "error" }),
};
const childrenByParent: Record<string, string[]> = {
  root: ["a", "b"],
  a: ["a1"],
};

describe("computeRollup", () => {
  beforeEach(() => {
    _resetRollupCacheForTests();
  });

  it("counts all descendants (not just direct children) by status", () => {
    const rollup = computeRollup("root", childrenByParent, nodesById, 1);
    expect(rollup.total).toBe(3);
    expect(rollup.byStatus.running).toBe(1);
    expect(rollup.byStatus.done).toBe(1);
    expect(rollup.byStatus.failed).toBe(1);
    expect(rollup.byStatus.idle).toBe(0);
  });

  it("counts attention across all descendants", () => {
    const rollup = computeRollup("root", childrenByParent, nodesById, 1);
    expect(rollup.attentionCount).toBe(2);
  });

  it("scopes to the given node's own subtree, not the whole tree", () => {
    const rollup = computeRollup("a", childrenByParent, nodesById, 1);
    expect(rollup.total).toBe(1);
    expect(rollup.attentionCount).toBe(1);
    expect(rollup.byStatus.done).toBe(1);
    expect(rollup.byStatus.failed).toBe(0);
  });

  it("returns an empty rollup for a leaf with no children", () => {
    const rollup = computeRollup("a1", childrenByParent, nodesById, 1);
    expect(rollup.total).toBe(0);
    expect(rollup.attentionCount).toBe(0);
  });

  it("returns an empty rollup for an id absent from childrenByParent", () => {
    const rollup = computeRollup("unknown", childrenByParent, nodesById, 1);
    expect(rollup.total).toBe(0);
  });

  it("memoizes within a rev: repeated calls return the same object reference", () => {
    const first = computeRollup("root", childrenByParent, nodesById, 7);
    const second = computeRollup("root", childrenByParent, nodesById, 7);
    expect(first).toBe(second);
  });

  it("invalidates the memo when rev changes, recomputing to equal-but-distinct data", () => {
    const first = computeRollup("root", childrenByParent, nodesById, 1);
    const second = computeRollup("root", childrenByParent, nodesById, 2);
    expect(first).not.toBe(second);
    expect(first).toEqual(second);
  });
});
