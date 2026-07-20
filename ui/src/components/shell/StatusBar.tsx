import { useEffect, useState } from "react";
import clsx from "clsx";
import { useConnectionStore, type ConnectionStatus } from "../../state/connection";
import { apiClient } from "../../state/api";
import { UsageMeter } from "./UsageMeter";
import type { VersionResponse } from "../../gen/types";

const STATUS_DOT: Record<ConnectionStatus, string> = {
  connecting: "bg-status-starting animate-pulse",
  open: "bg-status-running",
  reconnecting: "bg-status-awaiting animate-pulse",
  closed: "bg-status-failed",
};

const STATUS_LABEL: Record<ConnectionStatus, string> = {
  connecting: "Connecting…",
  open: "Connected",
  reconnecting: "Reconnecting…",
  closed: "Disconnected",
};

function Key({ children }: { children: string }) {
  return (
    <kbd className="rounded border border-border-strong bg-surface-2 px-1 py-px font-mono text-[10px] text-ink-muted">
      {children}
    </kbd>
  );
}

export function StatusBar() {
  const status = useConnectionStore((s) => s.status);
  const rev = useConnectionStore((s) => s.rev);
  const [version, setVersion] = useState<VersionResponse | null>(null);

  useEffect(() => {
    let cancelled = false;
    apiClient
      .getVersion()
      .then((v) => {
        if (!cancelled) setVersion(v);
      })
      .catch(() => {
        // Non-critical: the statusbar just omits the version chip.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="hidden h-6 shrink-0 items-center justify-between border-t border-border bg-surface px-3 text-2xs text-ink-faint md:flex">
      <div className="flex min-w-0 items-center gap-3">
        <span className="flex shrink-0 items-center gap-1.5">
          <span className={clsx("h-1.5 w-1.5 rounded-full", STATUS_DOT[status])} />
          {STATUS_LABEL[status]}
        </span>
        <span className="shrink-0">rev {rev}</span>
      </div>
      <UsageMeter />
      <div className="flex shrink-0 items-center gap-3">
        <span className="hidden items-center gap-1 sm:flex">
          <Key>j</Key>
          <Key>k</Key>
          <span>navigate</span>
          <span className="mx-1 text-ink-disabled">·</span>
          <Key>↵</Key>
          <span>open</span>
          <span className="mx-1 text-ink-disabled">·</span>
          <Key>a</Key>
          <span>ack</span>
          <span className="mx-1 text-ink-disabled">·</span>
          <Key>⌘K</Key>
          <span>palette</span>
        </span>
        {version && <span>v{version.version}</span>}
      </div>
    </div>
  );
}
