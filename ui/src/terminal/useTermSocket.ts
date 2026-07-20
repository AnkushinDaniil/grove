import { useEffect, useRef } from "react";
import type { TermControlMessage } from "../gen/types";
import { useLatestRef } from "../hooks/useLatestRef";
import { createWebSocketTermTransport } from "./termTransport";
import type { TermTransport } from "./termTypes";

export interface UseTermSocketOptions {
  /** null/false disconnects (or never connects) -- used both for "no
   *  session yet" and for LRU eviction from the mount pool. */
  sessionId: string | null;
  enabled: boolean;
  getInitialSize: () => { cols: number; rows: number };
  onData: (data: Uint8Array) => void;
  onLive: () => void;
  onExit: (code: number) => void;
}

export interface TermSocketHandle {
  resize: (cols: number, rows: number) => void;
  sendInput: (data: Uint8Array) => void;
}

/**
 * Owns the lifecycle of one /ws/term/{session_id} attach: connect, send the
 * initial resize, stream replay + live binary frames to `onData`, surface
 * the `live`/`exit` control frames, and tear down on unmount or when the
 * session/enabled inputs change. Deliberately does NOT depend on cols/rows
 * in its effect deps -- resizing happens imperatively via the returned
 * handle so window/pane resizes never reconnect the socket.
 */
export function useTermSocket(options: UseTermSocketOptions): TermSocketHandle {
  const { sessionId, enabled } = options;
  const getInitialSizeRef = useLatestRef(options.getInitialSize);
  const onDataRef = useLatestRef(options.onData);
  const onLiveRef = useLatestRef(options.onLive);
  const onExitRef = useLatestRef(options.onExit);
  const transportRef = useRef<TermTransport | null>(null);

  useEffect(() => {
    if (!sessionId || !enabled) return;
    let cancelled = false;

    (async () => {
      const factory =
        import.meta.env.VITE_MOCK === "1"
          ? (await import("../mock/mockTerm")).createMockTermTransport
          : createWebSocketTermTransport;
      if (cancelled) return;

      const transport = factory(sessionId);
      transportRef.current = transport;
      const { cols, rows } = getInitialSizeRef.current();
      transport.start(
        cols,
        rows,
        (data) => onDataRef.current(data),
        (msg: TermControlMessage) => {
          if (msg.t === "live") onLiveRef.current();
          else onExitRef.current(msg.code);
        },
      );
    })();

    return () => {
      cancelled = true;
      transportRef.current?.stop();
      transportRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- callbacks/size are read via refs by design
  }, [sessionId, enabled]);

  return {
    resize: (cols, rows) => transportRef.current?.resize(cols, rows),
    sendInput: (data) => transportRef.current?.sendInput(data),
  };
}
