import { useEffect, useRef } from "react";
import clsx from "clsx";
import { useLocation, useNavigate } from "react-router";
import { CircleDot, Inbox as InboxIcon, ListTree } from "lucide-react";
import { useShallow } from "zustand/react/shallow";
import { selectInboxEvents, useInboxStore } from "../../state/inbox";
import { AttentionBadge } from "../tree/AttentionBadge";

interface BottomTabsProps {
  onOpenTree: () => void;
}

/** Mobile-only primary navigation (`md:hidden`): Tree opens the slide-over
 *  drawer, Node/Inbox are real routes. "Node" remembers the last non-inbox
 *  path so returning to it doesn't dump you back at the workspace root. */
export function BottomTabs({ onOpenTree }: BottomTabsProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const inboxCount = useInboxStore(useShallow((s) => selectInboxEvents(s).length));
  const lastMainPath = useRef("/");

  const isInbox = location.pathname === "/inbox";

  useEffect(() => {
    if (!isInbox) lastMainPath.current = location.pathname;
  }, [location.pathname, isInbox]);

  return (
    <nav
      className="grid grid-cols-3 border-t border-border bg-surface pb-[env(safe-area-inset-bottom)] md:hidden"
      aria-label="Primary"
    >
      <button
        type="button"
        onClick={onOpenTree}
        className="flex min-h-14 flex-col items-center justify-center gap-0.5 text-ink-faint active:bg-hover"
      >
        <ListTree size={19} />
        <span className="text-[10px]">Tree</span>
      </button>
      <button
        type="button"
        onClick={() => navigate(lastMainPath.current)}
        className={clsx(
          "flex min-h-14 flex-col items-center justify-center gap-0.5 active:bg-hover",
          !isInbox ? "text-accent" : "text-ink-faint",
        )}
        aria-current={!isInbox ? "page" : undefined}
      >
        <CircleDot size={19} />
        <span className="text-[10px]">Node</span>
      </button>
      <button
        type="button"
        onClick={() => navigate("/inbox")}
        className={clsx(
          "relative flex min-h-14 flex-col items-center justify-center gap-0.5 active:bg-hover",
          isInbox ? "text-accent" : "text-ink-faint",
        )}
        aria-current={isInbox ? "page" : undefined}
      >
        <InboxIcon size={19} />
        <span className="text-[10px]">Inbox</span>
        {inboxCount > 0 && <AttentionBadge count={inboxCount} className="absolute right-[27%] top-1.5" />}
      </button>
    </nav>
  );
}
