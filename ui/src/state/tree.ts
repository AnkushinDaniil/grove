import { create } from "zustand";
import type { Node, NodeID, Session, SessionID } from "../gen/types";

function buildChildrenIndex(nodesById: Record<NodeID, Node>): Record<NodeID, NodeID[]> {
  const byParent: Record<NodeID, Node[]> = {};
  for (const node of Object.values(nodesById)) {
    if (node.archived_at) continue; // archived nodes don't appear in the live tree
    const key = node.parent_id || "";
    (byParent[key] ??= []).push(node);
  }
  const result: Record<NodeID, NodeID[]> = {};
  for (const [parentId, children] of Object.entries(byParent)) {
    children.sort(
      (a, b) => a.position - b.position || a.created_at.localeCompare(b.created_at),
    );
    result[parentId] = children.map((n) => n.id);
  }
  return result;
}

function findRoot(nodesById: Record<NodeID, Node>): NodeID | null {
  for (const node of Object.values(nodesById)) {
    if (node.kind === "workspace" && !node.archived_at) return node.id;
  }
  return null;
}

interface TreeState {
  rev: number;
  nodesById: Record<NodeID, Node>;
  childrenByParent: Record<NodeID, NodeID[]>;
  sessionsById: Record<SessionID, Session>;
  rootId: NodeID | null;
  /** True once the first hello/snapshot has been applied. Distinguishes
   *  "empty tree" from "haven't loaded yet" for empty-state rendering. */
  loaded: boolean;

  applyHello: (rev: number, nodes: Node[], sessions: Session[]) => void;
  applyDelta: (rev: number, nodes?: Node[], sessions?: Session[]) => void;
  reset: () => void;
}

const initial = {
  rev: 0,
  nodesById: {} as Record<NodeID, Node>,
  childrenByParent: {} as Record<NodeID, NodeID[]>,
  sessionsById: {} as Record<SessionID, Session>,
  rootId: null as NodeID | null,
  loaded: false,
};

export const useTreeStore = create<TreeState>((set) => ({
  ...initial,

  applyHello: (rev, nodes, sessions) => {
    const nodesById: Record<NodeID, Node> = {};
    for (const n of nodes) nodesById[n.id] = n;
    const sessionsById: Record<SessionID, Session> = {};
    for (const s of sessions) sessionsById[s.id] = s;
    set({
      rev,
      nodesById,
      sessionsById,
      childrenByParent: buildChildrenIndex(nodesById),
      rootId: findRoot(nodesById),
      loaded: true,
    });
  },

  applyDelta: (rev, nodes, sessions) => {
    set((state) => {
      let nodesById = state.nodesById;
      let childrenByParent = state.childrenByParent;
      let rootId = state.rootId;
      if (nodes && nodes.length > 0) {
        nodesById = { ...nodesById };
        for (const n of nodes) nodesById[n.id] = n;
        childrenByParent = buildChildrenIndex(nodesById);
        rootId = findRoot(nodesById);
      }

      let sessionsById = state.sessionsById;
      if (sessions && sessions.length > 0) {
        sessionsById = { ...sessionsById };
        for (const s of sessions) sessionsById[s.id] = s;
      }

      return { rev, nodesById, childrenByParent, rootId, sessionsById, loaded: true };
    });
  },

  reset: () => set({ ...initial }),
}));
