import clsx from "clsx";
import { Menu } from "lucide-react";
import { useConnectionStore, type ConnectionStatus } from "../../state/connection";
import { FOCUS_RING } from "../../lib/constants";

const STATUS_DOT: Record<ConnectionStatus, string> = {
  connecting: "bg-status-starting animate-pulse",
  open: "bg-status-running",
  reconnecting: "bg-status-awaiting animate-pulse",
  closed: "bg-status-failed",
};

interface MobileTopBarProps {
  onOpenTree: () => void;
}

/** Slim top bar shown only below `md`: hamburger opens the tree drawer, a
 *  connection dot stands in for the (desktop-only) statusbar. */
export function MobileTopBar({ onOpenTree }: MobileTopBarProps) {
  const status = useConnectionStore((s) => s.status);

  return (
    <div className="flex min-h-12 shrink-0 items-center gap-1 border-b border-border bg-surface px-1.5 pt-[env(safe-area-inset-top)] md:hidden">
      <button
        type="button"
        onClick={onOpenTree}
        aria-label="Open tree"
        className={clsx(
          "flex h-11 w-11 items-center justify-center rounded-md text-ink-muted active:bg-hover",
          FOCUS_RING,
        )}
      >
        <Menu size={20} />
      </button>
      <span className="flex-1 text-xs font-medium tracking-wide text-ink-faint">
        <span className="text-accent">◆</span> grove
      </span>
      <span
        className={clsx("mr-3 h-1.5 w-1.5 shrink-0 rounded-full", STATUS_DOT[status])}
        aria-hidden="true"
      />
    </div>
  );
}
