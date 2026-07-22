import { useEffect, useRef } from "react";
import clsx from "clsx";
import { useLocation, useNavigate } from "react-router";
import { CircleDot, GitPullRequest, Inbox as InboxIcon, ListTree } from "lucide-react";
import { useShallow } from "zustand/react/shallow";
import { selectInboxEvents, useInboxStore } from "../../state/inbox";
import { selectNeedsAttentionCount, useReviewsStore } from "../../state/reviews";
import { AttentionBadge } from "../tree/AttentionBadge";

interface BottomTabsProps {
  onOpenTree: () => void;
}

/** Mobile-only primary navigation (`md:hidden`): Tree opens the slide-over
 *  drawer, Node/Inbox/Reviews are real routes. "Node" remembers the last
 *  path that isn't one of those two tabs' own routes, so returning to it
 *  doesn't dump you back at the workspace root. */
export function BottomTabs({ onOpenTree }: BottomTabsProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const inboxCount = useInboxStore(useShallow((s) => selectInboxEvents(s).length));
  const reviewsCount = useReviewsStore(selectNeedsAttentionCount);
  const lastMainPath = useRef("/");

  const isInbox = location.pathname === "/inbox";
  const isReviews = location.pathname === "/reviews";

  useEffect(() => {
    if (!isInbox && !isReviews) lastMainPath.current = location.pathname;
  }, [location.pathname, isInbox, isReviews]);

  return (
    <nav
      className="grid grid-cols-4 border-t border-border bg-surface pb-[env(safe-area-inset-bottom)] md:hidden"
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
          !isInbox && !isReviews ? "text-accent" : "text-ink-faint",
        )}
        aria-current={!isInbox && !isReviews ? "page" : undefined}
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
      <button
        type="button"
        onClick={() => navigate("/reviews")}
        className={clsx(
          "relative flex min-h-14 flex-col items-center justify-center gap-0.5 active:bg-hover",
          isReviews ? "text-accent" : "text-ink-faint",
        )}
        aria-current={isReviews ? "page" : undefined}
      >
        <GitPullRequest size={19} />
        <span className="text-[10px]">Reviews</span>
        {reviewsCount > 0 && <AttentionBadge count={reviewsCount} className="absolute right-[19%] top-1.5" />}
      </button>
    </nav>
  );
}
