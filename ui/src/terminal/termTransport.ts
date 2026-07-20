import type { TermControlMessage, TermResizeMessage } from "../gen/types";
import type { TermTransport } from "./termTypes";

function wsTermURL(sessionId: string): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${location.host}/ws/term/${encodeURIComponent(sessionId)}`;
}

/** Real /ws/term/{session_id} client. */
class WebSocketTermTransport implements TermTransport {
  private socket: WebSocket | null = null;
  private pendingSize: { cols: number; rows: number } | null = null;

  constructor(private readonly sessionId: string) {}

  start(
    cols: number,
    rows: number,
    onBinary: (data: Uint8Array) => void,
    onControl: (msg: TermControlMessage) => void,
  ): void {
    const socket = new WebSocket(wsTermURL(this.sessionId));
    socket.binaryType = "arraybuffer";
    this.socket = socket;

    socket.onopen = () => {
      // First message must be the resize control frame per docs/API.md.
      socket.send(JSON.stringify({ t: "resize", cols, rows } satisfies TermResizeMessage));
      if (this.pendingSize) {
        socket.send(JSON.stringify({ t: "resize", ...this.pendingSize } satisfies TermResizeMessage));
        this.pendingSize = null;
      }
    };

    socket.onmessage = (ev) => {
      if (typeof ev.data === "string") {
        try {
          onControl(JSON.parse(ev.data) as TermControlMessage);
        } catch {
          // malformed control frame -- ignore
        }
        return;
      }
      onBinary(new Uint8Array(ev.data as ArrayBuffer));
    };
  }

  resize(cols: number, rows: number): void {
    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify({ t: "resize", cols, rows } satisfies TermResizeMessage));
    } else {
      // Socket not open yet (still connecting) -- apply once onopen fires.
      this.pendingSize = { cols, rows };
    }
  }

  sendInput(data: Uint8Array): void {
    if (this.socket && this.socket.readyState === WebSocket.OPEN) {
      this.socket.send(data);
    }
  }

  stop(): void {
    this.socket?.close();
    this.socket = null;
  }
}

export function createWebSocketTermTransport(sessionId: string): TermTransport {
  return new WebSocketTermTransport(sessionId);
}
