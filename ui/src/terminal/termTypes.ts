import type { TermControlMessage } from "../gen/types";

/**
 * Transport abstraction for a /ws/term/{session_id} connection, real or
 * mocked (see src/mock/mockTerm.ts). Binary frames carry terminal bytes in
 * both directions; JSON text frames carry the resize/live/exit control
 * protocol described in docs/API.md.
 */
export interface TermTransport {
  /** Opens the connection (or starts the scripted mock). Per the API
   *  contract the first client message must be the resize frame -- callers
   *  pass the initial size in, rather than calling resize() separately. */
  start(
    cols: number,
    rows: number,
    onBinary: (data: Uint8Array) => void,
    onControl: (msg: TermControlMessage) => void,
  ): void;
  resize(cols: number, rows: number): void;
  sendInput(data: Uint8Array): void;
  stop(): void;
}

export type TermTransportFactory = (sessionId: string) => TermTransport;
