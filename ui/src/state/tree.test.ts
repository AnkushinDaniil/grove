import { beforeEach, describe, expect, it } from "vitest";
import { useTreeStore } from "./tree";
import type { Node, Session } from "../gen/types";

function makeNode(id: string, overrides: Partial<Node> = {}): Node {
  return {
    id,
    parent_id: "",
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
    work_dir: "",
    meta: {},
    position: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function makeSession(id: string, nodeId: string, overrides: Partial<Session> = {}): Session {
  return {
    id,
    node_id: nodeId,
    driver: "claude",
    profile_id: "",
    mode: "pty",
    driver_session_id: "",
    status: "running",
    cwd: "/tmp",
    started_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("useTreeStore", () => {
  beforeEach(() => {
    useTreeStore.getState().reset();
  });

  it("applyHello indexes nodes/sessions, sorts children by position, and finds the root", () => {
    const root = makeNode("root", { kind: "workspace", parent_id: "" });
    const childB = makeNode("child-b", { parent_id: "root", position: 1 });
    const childA = makeNode("child-a", { parent_id: "root", position: 0 });
    const session = makeSession("s1", "child-a");

    useTreeStore.getState().applyHello(3, [root, childB, childA], [session]);

    const state = useTreeStore.getState();
    expect(state.rev).toBe(3);
    expect(state.loaded).toBe(true);
    expect(state.rootId).toBe("root");
    expect(state.nodesById["child-a"]).toEqual(childA);
    expect(state.sessionsById.s1).toEqual(session);
    expect(state.childrenByParent.root).toEqual(["child-a", "child-b"]);
  });

  it("excludes archived nodes from childrenByParent but keeps them addressable by id", () => {
    const root = makeNode("root", { kind: "workspace" });
    const archived = makeNode("gone", { parent_id: "root", archived_at: "2026-01-02T00:00:00Z" });
    const live = makeNode("live", { parent_id: "root", position: 1 });

    useTreeStore.getState().applyHello(1, [root, archived, live], []);

    expect(useTreeStore.getState().childrenByParent.root).toEqual(["live"]);
    expect(useTreeStore.getState().nodesById.gone).toBeDefined();
  });

  it("applyDelta upserts nodes and rebuilds children, leaving sessions untouched when omitted", () => {
    const root = makeNode("root", { kind: "workspace" });
    const child = makeNode("child", { parent_id: "root", status: "idle" });
    const session = makeSession("s1", "child");
    useTreeStore.getState().applyHello(1, [root, child], [session]);

    const updatedChild: Node = { ...child, status: "running" };
    const newGrandchild = makeNode("grandchild", { parent_id: "child" });
    useTreeStore.getState().applyDelta(2, [updatedChild, newGrandchild]);

    const state = useTreeStore.getState();
    expect(state.rev).toBe(2);
    expect(state.nodesById.child.status).toBe("running");
    expect(state.childrenByParent.child).toEqual(["grandchild"]);
    expect(state.sessionsById.s1).toEqual(session);
  });

  it("applyDelta upserts sessions independently of nodes", () => {
    const root = makeNode("root", { kind: "workspace" });
    useTreeStore.getState().applyHello(1, [root], []);

    const session = makeSession("s1", "root", { status: "awaiting_input" });
    useTreeStore.getState().applyDelta(2, undefined, [session]);

    const state = useTreeStore.getState();
    expect(state.rev).toBe(2);
    expect(state.sessionsById.s1.status).toBe("awaiting_input");
    expect(state.nodesById.root).toEqual(root);
  });

  it("applyDelta is a no-op on nodes/sessions when both are omitted, but still bumps rev", () => {
    const root = makeNode("root", { kind: "workspace" });
    useTreeStore.getState().applyHello(1, [root], []);

    useTreeStore.getState().applyDelta(2);

    const state = useTreeStore.getState();
    expect(state.rev).toBe(2);
    expect(state.nodesById.root).toEqual(root);
  });

  it("reset clears everything back to the initial state", () => {
    useTreeStore.getState().applyHello(5, [makeNode("root", { kind: "workspace" })], []);

    useTreeStore.getState().reset();

    const state = useTreeStore.getState();
    expect(state.rev).toBe(0);
    expect(state.loaded).toBe(false);
    expect(state.rootId).toBeNull();
    expect(Object.keys(state.nodesById)).toHaveLength(0);
  });
});
