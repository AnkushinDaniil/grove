import type { Event, EventID, Node, NodeID, Session, SessionID } from "../gen/types";
import { buildFixtureEvents, buildFixtureNodes, buildFixtureSessions } from "./fixtures";

export interface DeltaPatch {
  nodes?: Node[];
  sessions?: Session[];
  events?: Event[];
}

type Listener = (rev: number, patch: DeltaPatch) => void;

/**
 * Single mutable in-memory tree shared by mockApi (REST) and
 * mockTransport (WS): REST mutations call publish(), which bumps rev and
 * fans out to WS subscribers -- mirroring "REST mutates -> WS broadcasts"
 * on the real daemon closely enough for demo/manual verification purposes.
 */
class MockWorld {
  rev = 1;
  readonly nodesById = new Map<NodeID, Node>();
  readonly sessionsById = new Map<SessionID, Session>();
  readonly eventsById = new Map<EventID, Event>();

  private listeners = new Set<Listener>();
  private idSeq = 0;

  constructor() {
    for (const n of buildFixtureNodes()) this.nodesById.set(n.id, n);
    for (const s of buildFixtureSessions()) this.sessionsById.set(s.id, s);
    for (const e of buildFixtureEvents()) this.eventsById.set(e.id, e);
  }

  nextId(prefix: string): string {
    this.idSeq += 1;
    return `${prefix}-mock-${this.idSeq}`;
  }

  snapshot(): { rev: number; nodes: Node[]; sessions: Session[] } {
    return { rev: this.rev, nodes: [...this.nodesById.values()], sessions: [...this.sessionsById.values()] };
  }

  inbox(): Event[] {
    return [...this.eventsById.values()]
      .filter((e) => e.requires_attention && !e.acked_at)
      .sort((a, b) => b.created_at.localeCompare(a.created_at));
  }

  eventsForNode(nodeId: NodeID, after?: EventID, limit?: number): Event[] {
    let list = [...this.eventsById.values()]
      .filter((e) => e.node_id === nodeId)
      .sort((a, b) => a.created_at.localeCompare(b.created_at));
    if (after) {
      const idx = list.findIndex((e) => e.id === after);
      list = idx >= 0 ? list.slice(idx + 1) : list;
    }
    if (limit !== undefined) list = list.slice(0, limit);
    return list;
  }

  childrenOf(parentId: NodeID): Node[] {
    return [...this.nodesById.values()].filter((n) => n.parent_id === parentId && !n.archived_at);
  }

  subscribe(fn: Listener): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  /** Applies a patch, bumps rev, and notifies subscribers -- the mock
   *  equivalent of the daemon persisting a mutation and broadcasting it. */
  publish(patch: DeltaPatch): void {
    this.rev += 1;
    for (const n of patch.nodes ?? []) this.nodesById.set(n.id, n);
    for (const s of patch.sessions ?? []) this.sessionsById.set(s.id, s);
    for (const e of patch.events ?? []) this.eventsById.set(e.id, e);
    for (const fn of this.listeners) fn(this.rev, patch);
  }
}

export const world = new MockWorld();
