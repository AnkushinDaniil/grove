import { describe, it, expect, beforeEach } from "vitest";
import { useInboxStore, selectInboxEvents, selectInboxCountForNode } from "./inbox";
import type { Event } from "../gen/types";

function attnEvent(id: string, node: string, acked = false): Event {
  return {
    id, node_id: node, session_id: "", type: "awaiting_input",
    payload: {}, requires_attention: true,
    acked_at: acked ? "2026-07-21T00:00:00Z" : "",
    created_at: "2026-07-21T00:00:0" + id + "Z",
  } as unknown as Event;
}

describe("inbox ack flow", () => {
  beforeEach(() => useInboxStore.getState().reset());

  it("optimistic ack drops the node's events from the counts", () => {
    useInboxStore.getState().setInitial([attnEvent("1", "n1"), attnEvent("2", "n1"), attnEvent("3", "n2")]);
    expect(selectInboxEvents(useInboxStore.getState()).length).toBe(3);

    useInboxStore.getState().ackNodeOptimistic("n1");
    expect(selectInboxEvents(useInboxStore.getState()).length).toBe(1);
    expect(selectInboxCountForNode(useInboxStore.getState(), "n1")).toBe(0);
    expect(selectInboxCountForNode(useInboxStore.getState(), "n2")).toBe(1);
  });

  it("a WS delta re-publishing acked events clears them authoritatively", () => {
    useInboxStore.getState().setInitial([attnEvent("1", "n1")]);
    expect(selectInboxEvents(useInboxStore.getState()).length).toBe(1);

    // Server broadcast: same event id, now acked.
    useInboxStore.getState().upsertMany([attnEvent("1", "n1", true)]);
    expect(selectInboxEvents(useInboxStore.getState()).length).toBe(0);
  });
});
