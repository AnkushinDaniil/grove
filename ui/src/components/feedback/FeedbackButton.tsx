import clsx from "clsx";
import { ThumbsDown } from "lucide-react";
import { FOCUS_RING } from "../../lib/constants";

interface FeedbackButtonProps {
  active: boolean;
  onClick: () => void;
  className?: string;
  iconSize?: number;
}

/** Thumbs-down trigger, stateless -- the caller owns the open/closed state
 *  and decides where the FeedbackComposer it toggles actually renders (a
 *  node header's chip row and an EventsTab row have different layouts, so
 *  this stays a plain controlled button rather than bundling its own panel
 *  placement). */
export function FeedbackButton({ active, onClick, className, iconSize = 12 }: FeedbackButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-expanded={active}
      aria-label="Report feedback"
      title="Report feedback about this"
      className={clsx(
        "flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ink-faint hover:bg-hover hover:text-ink",
        active && "bg-hover text-accent",
        FOCUS_RING,
        className,
      )}
    >
      <ThumbsDown size={iconSize} />
    </button>
  );
}
