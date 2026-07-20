import { world } from "./world";

function nowISO(): string {
  return new Date().toISOString();
}

/** After POST /nodes/{id}/sessions, walk the session starting -> running so
 *  the mock shows real motion instead of freezing at "starting" forever. */
export function startMockSessionLifecycle(sessionId: string, nodeId: string): void {
  setTimeout(() => {
    const session = world.sessionsById.get(sessionId);
    const node = world.nodesById.get(nodeId);
    if (!session || !node || session.status !== "starting") return;
    const now = nowISO();
    world.publish({
      sessions: [{ ...session, status: "running" }],
      nodes: [{ ...node, status: "running", updated_at: now }],
      events: [
        {
          id: world.nextId("evt"),
          node_id: nodeId,
          session_id: sessionId,
          type: "session_started",
          payload: { driver_session_id: session.driver_session_id, model: "claude-opus-4-8" },
          requires_attention: false,
          created_at: now,
        },
      ],
    });
  }, 900);
}
