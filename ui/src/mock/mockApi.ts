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
import { prReviewWorld } from "./prReviewWorld";
import { worktreeReviewWorld } from "./worktreeReviewWorld";
import { repoWorld } from "./repoWorld";
import { profileWorld } from "./profileWorld";
import { buildFixtureStats } from "./statsFixtures";
import { feedbackWorld } from "./feedbackWorld";
import { buildFixtureMemory } from "./memoryFixtures";

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
      // A task created under a project that has repos gets a provisioned
      // worktree workspace on the real daemon; mirror that here so the Review
      // tab lights up for demo tasks created under a repo-backed project.
      const parent = world.nodesById.get(body.parent_id);
      const provisioned =
        body.kind === "task" && parent?.kind === "project" && repoWorld.hasRepos(body.parent_id);
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
        workspace_dir: provisioned ? `~/.grove/worktrees/${id}` : "",
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

    async resumeTarget(nodeId: string) {
      const node = world.snapshot().nodes.find((n) => n.id === nodeId);
      const id = node?.current_session_id
        ? (world.snapshot().sessions.find((s) => s.id === node.current_session_id)?.driver_session_id ?? "")
        : "";
      return {
        resumable: id !== "",
        driver_session_id: id,
        reason: id === "" ? "the previous conversation was not saved as a resumable transcript" : "",
      };
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

    async getPRReview(dir, pr) {
      return prReviewWorld.getReview(dir, pr);
    },

    async getReviewDrafts(dir, pr) {
      return { drafts: prReviewWorld.getDrafts(dir, pr) };
    },

    async addReviewDraft(body) {
      return prReviewWorld.addDraft(body);
    },

    async deleteReviewDraft(id) {
      prReviewWorld.removeDraft(id);
    },

    async aiDraft(req) {
      return prReviewWorld.aiDraft(req);
    },

    async submitReview(req) {
      return prReviewWorld.submitReview(req);
    },

    async replyToThread(req) {
      prReviewWorld.reply(req);
    },

    async getWorktreeReview(node, repo) {
      return worktreeReviewWorld.getReview(node, repo);
    },

    async getWorktreeComments(node, repo) {
      return { comments: worktreeReviewWorld.getComments(node, repo) };
    },

    async addWorktreeComment(body) {
      return worktreeReviewWorld.addComment(body);
    },

    async deleteWorktreeComment(id) {
      worktreeReviewWorld.removeComment(id);
    },

    async mergeWorktree(node, repo) {
      const existing = requireNode(node);
      const result = worktreeReviewWorld.merge(node, repo);
      // Mirrors the real merge closing the "needs review" loop -- clears the
      // node's review attention the same way ackNode does, so the tree
      // rail/inbox badge disappears once there's nothing left to review.
      if (result.merged && existing.attention === "review") {
        world.publish({
          nodes: [{ ...existing, attention: "none", attention_reason: "", attention_since: undefined, updated_at: nowISO() }],
        });
      }
      return result;
    },

    async addressWorktree(node, repo) {
      const existing = requireNode(node);
      const comments = worktreeReviewWorld.getComments(node, repo);
      const now = nowISO();
      const id = world.nextId("sess");
      const session: Session = {
        id,
        node_id: node,
        driver: existing.driver || "claude",
        profile_id: existing.profile_id,
        mode: "pty",
        driver_session_id: `claude-mock-${id}`,
        status: "starting",
        cwd: existing.workspace_dir || "~/.grove/worktrees/mock",
        started_at: now,
      };
      // Echoed as a "user" text event, same convention as sendPrompt (see
      // API.md's clarification), so the node's Terminal/Events tabs show
      // what the agent was asked to fix instead of a blank new session.
      const promptLines = [
        "Please address the following review comments in this worktree:",
        "",
        ...comments.map((c) => `- ${c.path}:${c.line} (${c.side}): "${c.body}"`),
      ];
      const echo: Event = {
        id: world.nextId("evt"),
        node_id: node,
        session_id: id,
        type: "text",
        payload: { text: promptLines.join("\n"), final: true, role: "user" },
        requires_attention: false,
        created_at: now,
      };
      const updatedNode: Node = { ...existing, status: "starting", current_session_id: id, updated_at: now };
      world.publish({ nodes: [updatedNode], sessions: [session], events: [echo] });
      startMockSessionLifecycle(id, node);
      return session;
    },

    async getRepos(projectId) {
      return { repos: repoWorld.list(projectId) };
    },

    async addRepo(projectId, body) {
      return repoWorld.add(projectId, body);
    },

    async deleteRepo(repoId) {
      repoWorld.remove(repoId);
    },

    async getProfiles() {
      return { profiles: profileWorld.list() };
    },

    async addProfile(body) {
      return profileWorld.add(body);
    },

    async deleteProfile(id) {
      profileWorld.remove(id);
    },

    async profileDoctor(id) {
      return profileWorld.doctor(id);
    },

    async getStats(scope, range) {
      return buildFixtureStats(scope, range ?? "7d");
    },

    async listFeedback(status) {
      return feedbackWorld.list(status);
    },

    async createFeedback(body) {
      return feedbackWorld.create(body);
    },

    async resolveFeedback(id, fixNodeId) {
      return feedbackWorld.resolve(id, fixNodeId);
    },

    async getNodeMemory(nodeId, scope) {
      return buildFixtureMemory(nodeId, scope ?? "self");
    },

    // Web push has no in-memory "world" to mutate: the daemon-side
    // subscription table doesn't exist in mock mode. The key is
    // deliberately not a real 65-byte P-256 point -- see state/push.ts --
    // so a stray click in `dev:mock` fails fast in PushManager.subscribe()
    // instead of quietly registering a live subscription with a real
    // browser push service.
    async getPushKey() {
      return { public_key: "mock-vapid-key-not-a-real-p256-point" };
    },

    async pushSubscribe() {
      // No-op: nothing to persist against in mock mode.
    },

    async pushUnsubscribe() {
      // No-op, mirrors pushSubscribe.
    },
  };
}
