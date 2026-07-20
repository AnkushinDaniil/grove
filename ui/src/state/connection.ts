import { create } from "zustand";

export type ConnectionStatus = "connecting" | "open" | "reconnecting" | "closed";

interface ConnectionState {
  status: ConnectionStatus;
  rev: number;
  lastError: string | null;
  setStatus: (status: ConnectionStatus) => void;
  setRev: (rev: number) => void;
  setError: (message: string | null) => void;
}

// Deliberately separate from the tree store: the statusbar only needs
// {status, rev} and shouldn't re-render on every node mutation.
export const useConnectionStore = create<ConnectionState>((set) => ({
  status: "connecting",
  rev: 0,
  lastError: null,
  setStatus: (status) => set({ status }),
  setRev: (rev) => set({ rev }),
  setError: (lastError) => set({ lastError }),
}));
