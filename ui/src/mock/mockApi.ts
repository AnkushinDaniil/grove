import type { ApiClient } from "../state/api";
import { ApiError } from "../state/api";
import type {
  CreateNodeRequest,
  CreateSessionRequest,
  Event,
  Node,
  PatchNodeRequest,
  Session,
} from "../gen/types";
import { world } from "./world";
import { startMockSessionLifecycle } from "./scenarios";
import { buildFixtureUsage } from "./fixtures";
import { suggestDirsMock } from "./fakeFs";
import { reviewWorld } from "./reviewWorld";

function nowISO(): string {
  return new Date().toISOString();
}

function requireNode(id: string): Node {
  const n = world.nodesById.get(id);
  if (!n) throw new ApiError(404, `node ${id} not found`);
  return n;
}

/** In-memory ApiClient over src/mock/world.ts. Mirrors the real daemon's
 *  "mutate then broadcast" behavior closely enough to demo and manually
 *  verify the UI with zero backend (VITE_MOCK=1). */
export async function createMockApiClient(): Promise<ApiClient> {
  return {
    async getTree() {
      return world.snapshot();
    },

    async createNode(body: CreateNodeRequest) {
      const now = nowISO();
      const id = world.nextId("node");
      const siblingCount = world.childrenOf(body.parent_id).length;
      const created: Node = {
        id,
        parent_id: body.parent_id,
        kind: body.kind,
        title: body.title,
        brief: body.brief ?? "",
        status: "idle",
        attention: "none",
        attention_reason: "",
        driver: body.driver ?? "",
        profile_id: body.profile_id ?? "",
        current_session_id: "",
        workspace_dir: "",
        work_dir: body.work_dir ?? "",
        meta: {},
        position: siblingCount,
        created_at: now,
        updated_at: now,
      };
      world.publish({ nodes: [created] });
      return created;
    },

    async patchNode(id, body: PatchNodeRequest) {
      const existing = requireNode(id);
      const updated: Node = {
        ...existing,
        title: body.title ?? existing.title,
        brief: body.brief ?? existing.brief,
        driver: body.driver ?? existing.driver,
        profile_id: body.profile_id ?? existing.profile_id,
        work_dir: body.work_dir ?? existing.work_dir,
        meta: body.meta ?? existing.meta,
        updated_at: nowISO(),
      };
      world.publish({ nodes: [updated] });
      return updated;
    },

    async archiveNode(id) {
      const root = requireNode(id);
      const now = nowISO();
      const archivedIds: string[] = [];
      const updates: Node[] = [];
      const stack = [root.id];
      while (stack.length > 0) {
        const curId = stack.pop();
        if (curId === undefined) continue;
        const cur = world.nodesById.get(curId);
        if (!cur || cur.archived_at) continue;
        archivedIds.push(curId);
        updates.push({ ...cur, archived_at: now, updated_at: now });
        for (const child of world.childrenOf(curId)) stack.push(child.id);
      }
      world.publish({ nodes: updates });
      return { archived: archivedIds };
    },

    async ackNode(id) {
      const existing = requireNode(id);
      const now = nowISO();
      const updated: Node = {
        ...existing,
        attention: "none",
        attention_reason: "",
        attention_since: undefined,
        updated_at: now,
      };
      const ackedEvents: Event[] = [...world.eventsById.values()]
        .filter((e) => e.node_id === id && e.requires_attention && !e.acked_at)
        .map((e) => ({ ...e, acked_at: now }));
      world.publish({ nodes: [updated], events: ackedEvents });
      return updated;
    },

    async createSession(nodeId, body: CreateSessionRequest) {
      const existing = requireNode(nodeId);
      const id = world.nextId("sess");
      const now = nowISO();
      const session: Session = {
        id,
        node_id: nodeId,
        driver: existing.driver || "claude",
        profile_id: existing.profile_id,
        mode: body.mode,
        driver_session_id: `claude-mock-${id}`,
        status: "starting",
        cwd: existing.workspace_dir || "~/.grove/worktrees/mock",
        started_at: now,
      };
      const updatedNode: Node = { ...existing, status: "starting", current_session_id: id, updated_at: now };
      world.publish({ nodes: [updatedNode], sessions: [session] });
      startMockSessionLifecycle(id, nodeId);
      return session;
    },

    async sendPrompt(nodeId, text) {
      const existing = requireNode(nodeId);
      const now = nowISO();
      // Echo the injected prompt as a "user" text event before it reaches
      // the agent, per API.md's clarification -- lets headless mode read
      // like a conversation in the Events tab.
      const echo: Event = {
        id: world.nextId("evt"),
        node_id: nodeId,
        session_id: existing.current_session_id,
        type: "text",
        payload: { text, final: true, role: "user" },
        requires_attention: false,
        created_at: now,
      };
      world.publish({ nodes: [{ ...existing, updated_at: now }], events: [echo] });
    },

    async stopSession(sessionId) {
      const session = world.sessionsById.get(sessionId);
      if (!session) throw new ApiError(404, `session ${sessionId} not found`);
      const now = nowISO();
      const updatedSession: Session = { ...session, status: "exited", exit_code: 0, ended_at: now };
      const node = world.nodesById.get(session.node_id);
      world.publish({
        sessions: [updatedSession],
        nodes: node ? [{ ...node, status: "done", updated_at: now }] : [],
      });
    },

    async getEvents(nodeId, after, limit) {
      return world.eventsForNode(nodeId, after, limit);
    },

    async getInbox() {
      return world.inbox();
    },

    async getVersion() {
      return { version: "0.0.0-mock", commit: "mock" };
    },

    async getUsage(window) {
      return { profiles: buildFixtureUsage(window) };
    },

    async suggestDirs(prefix) {
      return suggestDirsMock(prefix);
    },

    async authSession() {
      // Mock mode has no real auth handshake to perform.
    },

    async authMe() {
      return true;
    },

    async getReviews() {
      return reviewWorld.reviews();
    },

    async getReviewSources() {
      return { dirs: reviewWorld.dirs };
    },

    async setReviewSources(dirs) {
      return { dirs: reviewWorld.setDirs(dirs) };
    },

    async startReview(dir, prNumber, title) {
      const root = [...world.nodesById.values()].find((n) => n.kind === "workspace" && !n.archived_at);
      if (!root) throw new ApiError(404, "no workspace to attach a review to");
      const repoName = reviewWorld.getRepoName(dir) ?? dir;
      const foundPR = reviewWorld.findPR(dir, prNumber);
      const now = nowISO();
      const id = world.nextId("node");
      const created: Node = {
        id,
        parent_id: root.id,
        kind: "task",
        title: title ?? `Review ${repoName}#${prNumber}`,
        brief: `Read-only review of ${repoName}#${prNumber}${foundPR ? `: ${foundPR.title}` : ""}. Leave review comments on GitHub; do not push changes to the branch.`,
        status: "idle",
        attention: "none",
        attention_reason: "",
        driver: "",
        profile_id: "",
        current_session_id: "",
        workspace_dir: "",
        work_dir: dir,
        meta: {},
        position: world.childrenOf(root.id).length,
        created_at: now,
        updated_at: now,
      };
      world.publish({ nodes: [created] });
      return created;
    },
  };
}
