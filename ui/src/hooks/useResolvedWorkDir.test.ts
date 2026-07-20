import { beforeEach, describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { useTreeStore } from "../state/tree";
import { useResolvedWorkDir } from "./useResolvedWorkDir";
import type { Node } from "../gen/types";

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

describe("useResolvedWorkDir", () => {
  beforeEach(() => {
    useTreeStore.getState().reset();
  });

  it("returns a node's own work_dir as not inherited", () => {
    const root = makeNode("root", { kind: "workspace" });
    const proj = makeNode("proj", { kind: "project", parent_id: "root", work_dir: "/home/user/proj" });
    useTreeStore.getState().applyHello(2, [root, proj], []);

    const { result } = renderHook(() => useResolvedWorkDir("proj"));
    expect(result.current).toEqual({ value: "/home/user/proj", inherited: false });
  });

  it("inherits the nearest non-empty ancestor's work_dir", () => {
    const root = makeNode("root", { kind: "workspace", work_dir: "/home/user/root" });
    const proj = makeNode("proj", { kind: "project", parent_id: "root" });
    const task = makeNode("task", { kind: "task", parent_id: "proj" });
    useTreeStore.getState().applyHello(2, [root, proj, task], []);

    const { result } = renderHook(() => useResolvedWorkDir("task"));
    expect(result.current).toEqual({ value: "/home/user/root", inherited: true });
  });

  it("prefers a nearer ancestor over a farther one", () => {
    const root = makeNode("root", { kind: "workspace", work_dir: "/home/user/root" });
    const proj = makeNode("proj", { kind: "project", parent_id: "root", work_dir: "/home/user/proj" });
    const task = makeNode("task", { kind: "task", parent_id: "proj" });
    useTreeStore.getState().applyHello(2, [root, proj, task], []);

    const { result } = renderHook(() => useResolvedWorkDir("task"));
    expect(result.current).toEqual({ value: "/home/user/proj", inherited: true });
  });

  it("falls back to ~ (inherited) when nothing is set anywhere", () => {
    const root = makeNode("root", { kind: "workspace" });
    const task = makeNode("task", { kind: "task", parent_id: "root" });
    useTreeStore.getState().applyHello(2, [root, task], []);

    const { result } = renderHook(() => useResolvedWorkDir("task"));
    expect(result.current).toEqual({ value: "~", inherited: true });
  });

  it("returns the home fallback for an unknown node", () => {
    const { result } = renderHook(() => useResolvedWorkDir("nope"));
    expect(result.current).toEqual({ value: "~", inherited: true });
  });
});
