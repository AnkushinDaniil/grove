import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateSocketClient, type WebSocketLike } from "./ws";
import type { Event, Node, Session, WSStateMessage } from "../gen/types";

let instances: FakeWebSocket[] = [];

class FakeWebSocket implements WebSocketLike {
  onopen: ((ev: unknown) => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  onclose: ((ev: unknown) => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;
  closed = false;

  constructor(readonly url: string) {
    instances.push(this);
  }

  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.onclose?.(undefined);
  }

  emit(msg: WSStateMessage): void {
    this.onmessage?.({ data: JSON.stringify(msg) });
  }
}

function makeNode(id: string, overrides: Partial<Node> = {}): Node {
  return {
    id,
    parent_id: "",
    kind: "workspace",
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

function makeHandlers() {
  const calls = {
    hello: [] as [number, Node[], Session[], Event[]][],
    delta: [] as [number, Node[] | undefined, Session[] | undefined, Event[] | undefined][],
    status: [] as string[],
  };
  return {
    calls,
    handlers: {
      onHello: (rev: number, nodes: Node[], sessions: Session[], inbox: Event[]) => {
        calls.hello.push([rev, nodes, sessions, inbox]);
      },
      onDelta: (
        rev: number,
        nodes: Node[] | undefined,
        sessions: Session[] | undefined,
        events: Event[] | undefined,
      ) => {
        calls.delta.push([rev, nodes, sessions, events]);
      },
      onStatusChange: (status: string) => {
        calls.status.push(status);
      },
    },
  };
}

function connectAndHello(client: StateSocketClient, rev = 1) {
  client.connect();
  const socket = instances[instances.length - 1];
  socket.emit({ t: "hello", rev, nodes: [makeNode("n1")], sessions: [], inbox: [] });
  return socket;
}

describe("StateSocketClient", () => {
  beforeEach(() => {
    instances = [];
    vi.useFakeTimers();
    vi.spyOn(Math, "random").mockReturnValue(0.5); // deterministic jitter (1.0x)
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("applies hello and reports connecting then open", () => {
    const { handlers, calls } = makeHandlers();
    const client = new StateSocketClient({
      url: "ws://x/ws/state",
      handlers,
      wsFactory: (u) => new FakeWebSocket(u),
    });
    connectAndHello(client, 1);

    expect(calls.hello).toHaveLength(1);
    expect(calls.hello[0][0]).toBe(1);
    expect(calls.status).toContain("connecting");
    expect(calls.status[calls.status.length - 1]).toBe("open");
  });

  it("applies an in-order delta without reconnecting", () => {
    const { handlers, calls } = makeHandlers();
    const client = new StateSocketClient({
      url: "ws://x/ws/state",
      handlers,
      wsFactory: (u) => new FakeWebSocket(u),
    });
    const socket = connectAndHello(client, 1);

    socket.emit({ t: "delta", rev: 2, nodes: [makeNode("n2")] });

    expect(calls.delta).toHaveLength(1);
    expect(calls.delta[0][0]).toBe(2);
    expect(socket.closed).toBe(false);
    expect(instances).toHaveLength(1);
  });

  it("closes and reconnects on a rev gap, then resyncs from a fresh hello", () => {
    const { handlers, calls } = makeHandlers();
    const client = new StateSocketClient({
      url: "ws://x/ws/state",
      handlers,
      wsFactory: (u) => new FakeWebSocket(u),
      baseDelayMs: 300,
    });
    const socket = connectAndHello(client, 1);

    socket.emit({ t: "delta", rev: 5 }); // gap: expected rev 2

    expect(socket.closed).toBe(true);
    expect(calls.delta).toHaveLength(0); // the gapped delta must never be applied
    expect(instances).toHaveLength(1); // reconnect scheduled, not yet fired

    vi.advanceTimersByTime(300);
    expect(instances).toHaveLength(2);

    const secondSocket = instances[1];
    secondSocket.emit({ t: "hello", rev: 9, nodes: [], sessions: [], inbox: [] });
    expect(calls.hello).toHaveLength(2);
    expect(calls.hello[1][0]).toBe(9);

    secondSocket.emit({ t: "delta", rev: 10, sessions: [] });
    expect(calls.delta).toHaveLength(1);
    expect(calls.delta[0][0]).toBe(10);
  });

  it("never applies a gapped delta's payload even though it carries data", () => {
    const { handlers, calls } = makeHandlers();
    const client = new StateSocketClient({
      url: "ws://x/ws/state",
      handlers,
      wsFactory: (u) => new FakeWebSocket(u),
    });
    const socket = connectAndHello(client, 1);

    socket.emit({ t: "delta", rev: 3, nodes: [makeNode("should-not-apply")] });

    expect(calls.delta).toHaveLength(0);
  });

  it("backs off exponentially across repeated failures", () => {
    const { handlers } = makeHandlers();
    const client = new StateSocketClient({
      url: "ws://x/ws/state",
      handlers,
      wsFactory: (u) => new FakeWebSocket(u),
      baseDelayMs: 100,
      maxDelayMs: 10_000,
    });
    client.connect();
    expect(instances).toHaveLength(1);

    instances[0].close(); // fails before any hello
    vi.advanceTimersByTime(99);
    expect(instances).toHaveLength(1);
    vi.advanceTimersByTime(1); // 100 * 2^0 = 100
    expect(instances).toHaveLength(2);

    instances[1].close();
    vi.advanceTimersByTime(199);
    expect(instances).toHaveLength(2);
    vi.advanceTimersByTime(1); // 100 * 2^1 = 200
    expect(instances).toHaveLength(3);

    instances[2].close();
    vi.advanceTimersByTime(400); // 100 * 2^2 = 400
    expect(instances).toHaveLength(4);
  });

  it("caps the reconnect delay at maxDelayMs", () => {
    const { handlers } = makeHandlers();
    const client = new StateSocketClient({
      url: "ws://x/ws/state",
      handlers,
      wsFactory: (u) => new FakeWebSocket(u),
      baseDelayMs: 1000,
      maxDelayMs: 1500,
    });
    client.connect();
    instances[0].close(); // attempt 1: min(1500, 1000*2^0)=1000
    vi.advanceTimersByTime(1000);
    expect(instances).toHaveLength(2);

    instances[1].close(); // attempt 2: min(1500, 1000*2^1)=1500, not 2000
    vi.advanceTimersByTime(1500);
    expect(instances).toHaveLength(3);
  });

  it("resets backoff to the base delay after a successful hello", () => {
    const { handlers } = makeHandlers();
    const client = new StateSocketClient({
      url: "ws://x/ws/state",
      handlers,
      wsFactory: (u) => new FakeWebSocket(u),
      baseDelayMs: 100,
    });
    client.connect();
    instances[0].close();
    vi.advanceTimersByTime(100); // attempt 1 -> 100ms
    expect(instances).toHaveLength(2);

    instances[1].close();
    vi.advanceTimersByTime(200); // attempt 2 -> 200ms
    expect(instances).toHaveLength(3);

    instances[2].emit({ t: "hello", rev: 1, nodes: [], sessions: [], inbox: [] });
    instances[2].close(); // fresh failure right after a successful hello

    vi.advanceTimersByTime(99);
    expect(instances).toHaveLength(3);
    vi.advanceTimersByTime(1); // back to base delay (100ms), not 400ms
    expect(instances).toHaveLength(4);
  });

  it("stop() prevents further reconnect attempts and reports closed", () => {
    const { handlers, calls } = makeHandlers();
    const client = new StateSocketClient({
      url: "ws://x/ws/state",
      handlers,
      wsFactory: (u) => new FakeWebSocket(u),
      baseDelayMs: 100,
    });
    client.connect();
    instances[0].close();
    vi.advanceTimersByTime(100);
    expect(instances).toHaveLength(2);

    instances[1].close();
    client.stop();
    vi.advanceTimersByTime(10_000);

    expect(instances).toHaveLength(2); // no further reconnect
    expect(calls.status[calls.status.length - 1]).toBe("closed");
  });

  it("ignores malformed JSON frames instead of throwing", () => {
    const { handlers, calls } = makeHandlers();
    const client = new StateSocketClient({
      url: "ws://x/ws/state",
      handlers,
      wsFactory: (u) => new FakeWebSocket(u),
    });
    const socket = connectAndHello(client, 1);

    expect(() => socket.onmessage?.({ data: "{not valid json" })).not.toThrow();
    expect(calls.delta).toHaveLength(0);
  });
});
