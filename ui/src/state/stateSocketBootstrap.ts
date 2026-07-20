import { StateSocketClient, type StateSocketHandlers, type StateTransport } from "./ws";
import { useTreeStore } from "./tree";
import { useInboxStore } from "./inbox";
import { useConnectionStore } from "./connection";
import { useLiveEventsStore } from "./events";

function wsURL(): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${location.host}/ws/state`;
}

let transport: StateTransport | null = null;

/** Wires a StateSocketClient (or, in mock mode, MockStateTransport) to the
 *  tree/inbox/connection/live-events stores. Call once at app startup. */
export async function startStateSocket(): Promise<void> {
  if (transport) return;

  const handlers: StateSocketHandlers = {
    onHello: (rev, nodes, sessions, inbox) => {
      useTreeStore.getState().applyHello(rev, nodes, sessions);
      useInboxStore.getState().setInitial(inbox);
      useConnectionStore.getState().setRev(rev);
    },
    onDelta: (rev, nodes, sessions, events) => {
      useTreeStore.getState().applyDelta(rev, nodes, sessions);
      if (events && events.length > 0) {
        useInboxStore.getState().upsertMany(events);
        useLiveEventsStore.getState().publish(events);
      }
      useConnectionStore.getState().setRev(rev);
    },
    onStatusChange: (status) => useConnectionStore.getState().setStatus(status),
  };

  transport =
    import.meta.env.VITE_MOCK === "1"
      ? new (await import("../mock/mockTransport")).MockStateTransport(handlers)
      : new StateSocketClient({ url: wsURL(), handlers });

  transport.connect();
}

export function stopStateSocket(): void {
  transport?.stop();
  transport = null;
}
