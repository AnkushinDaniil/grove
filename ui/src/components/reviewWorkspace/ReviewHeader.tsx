import { ExternalLink } from "lucide-react";
import { ChecksPill } from "../reviews/ChecksPill";
import { Pill } from "../common/Pill";
import type { PRReview, ReviewDecision } from "../../gen/types";

const DECISION_LABEL: Record<ReviewDecision, string> = {
  REVIEW_REQUIRED: "Review required",
  APPROVED: "Approved",
  CHANGES_REQUESTED: "Changes requested",
  "": "",
};

const DECISION_TONE: Record<ReviewDecision, "neutral" | "accent" | "muted"> = {
  REVIEW_REQUIRED: "neutral",
  APPROVED: "accent",
  CHANGES_REQUESTED: "neutral",
  "": "muted",
};

interface ReviewHeaderProps {
  review: PRReview;
  dir: string;
}

/** PR identity + signals: number/title/author, checks + review-decision
 *  pills, base<-head, a link out to GitHub, and the watched repo dir. */
export function ReviewHeader({ review, dir }: ReviewHeaderProps) {
  return (
    <div className="shrink-0 space-y-1.5 border-b border-border px-5 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="shrink-0 font-mono text-xs text-ink-faint">#{review.number}</span>
        <h1 className="min-w-0 flex-1 basis-64 truncate font-sans text-sm font-medium text-ink" title={review.title}>
          {review.title}
        </h1>
        <ChecksPill checks={review.checks} />
        {review.review_decision && (
          <Pill tone={DECISION_TONE[review.review_decision]}>{DECISION_LABEL[review.review_decision]}</Pill>
        )}
        <a
          href={review.url}
          target="_blank"
          rel="noreferrer"
          className="flex shrink-0 items-center gap-1 text-2xs text-ink-faint hover:text-accent"
        >
          Open on GitHub
          <ExternalLink size={11} />
        </a>
      </div>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-2xs text-ink-faint">
        <span title={`Author: ${review.author}`}>by {review.author}</span>
        <span className="font-mono" title={`${review.base_ref} ← ${review.head_sha}`}>
          {review.base_ref} ← {review.head_sha.slice(0, 7)}
        </span>
        <span className="min-w-0 truncate font-mono" title={dir}>
          {dir}
        </span>
      </div>
    </div>
  );
}
