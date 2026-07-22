import clsx from "clsx";
import { ThumbsDown } from "lucide-react";
import { FEEDBACK_KIND_LABEL } from "../../lib/constants";
import type { StatsFeedbackSummary } from "../../gen/types";

interface FeedbackLeaderboardProps {
  items: StatsFeedbackSummary[];
  onOpenFeedback: () => void;
}

/** Recurring pain points, highest-open first -- the "what keeps biting me"
 *  view. Rows link into the Feedback tab rather than duplicating its list
 *  UI here. */
export function FeedbackLeaderboard({ items, onOpenFeedback }: FeedbackLeaderboardProps) {
  const sorted = [...items].sort((a, b) => b.open - a.open || b.total - a.total).slice(0, 6);

  return (
    <section aria-labelledby="feedback-heading" className="space-y-2">
      <div className="flex items-center justify-between">
        <h2 id="feedback-heading" className="flex items-center gap-1.5 font-sans text-xs font-semibold text-ink">
          <ThumbsDown size={13} className="text-ink-faint" />
          Feedback
        </h2>
        <button type="button" onClick={onOpenFeedback} className="font-sans text-2xs text-ink-faint hover:text-accent">
          View all →
        </button>
      </div>
      {sorted.length === 0 ? (
        <p className="font-sans text-2xs text-ink-faint">No feedback recorded yet.</p>
      ) : (
        <ul className="space-y-0.5">
          {sorted.map((item) => (
            <li key={`${item.kind}-${item.subject}`}>
              <button
                type="button"
                onClick={onOpenFeedback}
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-hover"
              >
                <span className="shrink-0 rounded-md border border-border-strong bg-surface-2 px-1.5 py-0.5 text-2xs text-ink-muted">
                  {FEEDBACK_KIND_LABEL[item.kind]}
                </span>
                <span className="min-w-0 flex-1 truncate text-ink">{item.subject || "(unspecified)"}</span>
                <span className={clsx("shrink-0 text-2xs", item.open > 0 ? "text-accent" : "text-ink-faint")}>
                  {item.open} open / {item.total}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
