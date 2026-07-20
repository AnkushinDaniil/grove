import type { Event, Node, Session, WSStateMessage } from "../gen/types";

export type ConnectionStatus = "connecting" | "open" | "reconnecting" | "closed";

/** The subset of the browser WebSocket surface the state client depends on.
 *  Kept minimal and injectable so tests can supply a fake instead of a real
 *  socket. */
export interface WebSocketLike {
  onopen: ((this: WebSocketLike, ev: unknown) => void) | null;
  onmessage: ((this: WebSocketLike, ev: { data: unknown }) => void) | null;
  onclose: ((this: WebSocketLike, ev: unknown) => void) | null;
  onerror: ((this: WebSocketLike, ev: unknown) => void) | null;
  close(): void;
}

export type WebSocketFactory = (url: string) => WebSocketLike;

const defaultFactory: WebSocketFactory = (url) => new WebSocket(url) as unknown as WebSocketLike;

export interface StateSocketHandlers {
  onHello: (rev: number, nodes: Node[], sessions: Session[], inbox: Event[]) => void;
  onDelta: (rev: number, nodes: Node[] | undefined, sessions: Session[] | undefined, events: Event[] | undefined) => void;
  onStatusChange: (status: ConnectionStatus) => void;
}

export interface StateSocketOptions {
  url: string;
  handlers: StateSocketHandlers;
  wsFactory?: WebSocketFactory;
  /** First reconnect delay in ms (default 300). */
  baseDelayMs?: number;
  /** Reconnect delay cap in ms (default 10000). */
  maxDelayMs?: number;
}

export interface StateTransport {
  connect(): void;
  stop(): void;
}

/**
 * Client for /ws/state. Applies `hello` snapshots and `delta` patches per
 * docs/API.md. `rev` must advance by exactly 1 per delta; any gap (dropped
 * message, out-of-order delivery, or the socket having closed and silently
 * reopened) means the local state may be stale, so we close and reconnect
 * from scratch rather than risk applying a delta onto the wrong base --
 * reconnecting always starts from a fresh `hello`, which is self-healing by
 * construction.
 */
export class StateSocketClient implements StateTransport {
  private readonly url: string;
  private readonly handlers: StateSocketHandlers;
  private readonly wsFactory: WebSocketFactory;
  private readonly baseDelayMs: number;
  private readonly maxDelayMs: number;

  private socket: WebSocketLike | null = null;
  private rev = 0;
  private attempt = 0;
  private stopped = true;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(options: StateSocketOptions) {
    this.url = options.url;
    this.handlers = options.handlers;
    this.wsFactory = options.wsFactory ?? defaultFactory;
    this.baseDelayMs = options.baseDelayMs ?? 300;
    this.maxDelayMs = options.maxDelayMs ?? 10_000;
  }

  connect(): void {
    this.stopped = false;
    this.attempt = 0;
    this.open();
  }

  /** Stops the client and suppresses further reconnect attempts. Safe to
   *  call multiple times. */
  stop(): void {
    this.stopped = true;
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    const socket = this.socket;
    this.socket = null;
    socket?.close();
    this.handlers.onStatusChange("closed");
  }

  private open(): void {
    this.handlers.onStatusChange(this.attempt === 0 ? "connecting" : "reconnecting");
    const socket = this.wsFactory(this.url);
    this.socket = socket;
    socket.onopen = () => {
      // Status flips to "open" once `hello` arrives (we don't have a usable
      // snapshot before that), not on the raw socket-open event.
    };
    socket.onmessage = (ev) => this.handleMessage(socket, ev.data);
    socket.onclose = () => this.handleClose(socket);
    socket.onerror = () => {
      // onclose always follows onerror for browser WebSockets; reconnect
      // scheduling lives solely in handleClose to avoid double-scheduling.
    };
  }

  private handleMessage(socket: WebSocketLike, raw: unknown): void {
    if (this.socket !== socket) return; // stale socket, already superseded
    if (typeof raw !== "string") return; // state socket is JSON text frames only

    let msg: WSStateMessage;
    try {
      msg = JSON.parse(raw) as WSStateMessage;
    } catch {
      return;
    }

    if (msg.t === "hello") {
      this.rev = msg.rev;
      this.attempt = 0;
      this.handlers.onHello(msg.rev, msg.nodes, msg.sessions, msg.inbox);
      this.handlers.onStatusChange("open");
      return;
    }

    if (msg.t === "delta") {
      if (msg.rev !== this.rev + 1) {
        socket.close();
        return;
      }
      this.rev = msg.rev;
      this.handlers.onDelta(msg.rev, msg.nodes, msg.sessions, msg.events);
    }
  }

  private handleClose(socket: WebSocketLike): void {
    if (this.socket !== socket) return; // already handled via another path
    this.socket = null;
    if (this.stopped) return;

    this.attempt += 1;
    const delay = Math.min(this.maxDelayMs, this.baseDelayMs * 2 ** (this.attempt - 1));
    const jitter = delay * (0.75 + Math.random() * 0.5);
    this.handlers.onStatusChange("reconnecting");
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      if (!this.stopped) this.open();
    }, jitter);
  }
}
