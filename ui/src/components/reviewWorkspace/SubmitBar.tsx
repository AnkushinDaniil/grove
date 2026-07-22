import { useState } from "react";
import clsx from "clsx";
import { AlertTriangle, Check, MessageCircle, ThumbsDown, ThumbsUp } from "lucide-react";
import { apiClient } from "../../state/api";
import { loadReviewWorkspace } from "../../state/reviewWorkspace";
import { AiDraftField } from "./AiDraftField";
import { FOCUS_RING } from "../../lib/constants";
import type { DraftComment, SubmitReviewEvent } from "../../gen/types";

interface SubmitBarProps {
  dir: string;
  pr: number;
  drafts: DraftComment[];
}

/** Sticky bottom bar: an overall review-summary composer (with its own
 *  "Draft with AI") and the three ways to submit a review, batching every
 *  pending draft along with whichever event is chosen. */
export function SubmitBar({ dir, pr, drafts }: SubmitBarProps) {
  const draftIds = drafts.map((d) => d.id);
  const [body, setBody] = useState("");
  const [busyEvent, setBusyEvent] = useState<SubmitReviewEvent | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<{ url: string } | null>(null);

  async function submit(event: SubmitReviewEvent) {
    setBusyEvent(event);
    setError(null);
    try {
      const res = await apiClient.submitReview({ dir, pr, event, body: body.trim(), draft_ids: draftIds });
      setResult(res);
      setBody("");
      // The server's response is just {url} -- reload to pick up the new
      // threads, cleared drafts, and updated review_decision it produced.
      await loadReviewWorkspace(dir, pr);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusyEvent(null);
    }
  }

  return (
    <div className="shrink-0 space-y-2 border-t border-border bg-surface px-4 py-3">
      {result && (
        <div className="flex items-center gap-2 rounded-md border border-accent/30 bg-accent-soft px-2.5 py-1.5 text-xs text-accent">
          <Check size={13} className="shrink-0" />
          <span className="min-w-0 flex-1">Review submitted.</span>
          <a
            href={result.url}
            target="_blank"
            rel="noreferrer"
            className="shrink-0 underline underline-offset-2 hover:no-underline"
          >
            View on GitHub
          </a>
          <button type="button" onClick={() => setResult(null)} className="shrink-0 text-accent/70 hover:text-accent">
            dismiss
          </button>
        </div>
      )}
      {error && (
        <div className="flex items-center gap-2 rounded-md border border-status-failed/40 bg-status-failed/10 px-2.5 py-1.5 text-xs text-status-failed">
          <AlertTriangle size={13} className="shrink-0" />
          <span className="min-w-0 flex-1 break-words">{error}</span>
          <button type="button" onClick={() => setError(null)} className="shrink-0 text-2xs text-ink-faint hover:text-ink">
            dismiss
          </button>
        </div>
      )}
      <div className="flex flex-col gap-2 lg:flex-row lg:items-end">
        <div className="min-w-0 flex-1">
          <AiDraftField
            value={body}
            onChange={setBody}
            ariaLabel="Review summary"
            placeholder="Leave an overall summary for this review (optional)…"
            rows={2}
            disabled={busyEvent !== null}
            onRequestDraft={async () => {
              const res = await apiClient.aiDraft({ dir, pr, kind: "comment", instruction: body.trim() || undefined });
              return res.text;
            }}
          />
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <span className="mr-1 text-2xs text-ink-faint">
            {draftIds.length} draft{draftIds.length === 1 ? "" : "s"}
          </span>
          <button
            type="button"
            onClick={() => void submit("COMMENT")}
            disabled={busyEvent !== null}
            className={clsx(
              "flex min-h-9 items-center gap-1.5 rounded-md border border-border-strong px-2.5 py-1.5 text-xs text-ink-muted hover:bg-hover hover:text-ink disabled:opacity-40",
              FOCUS_RING,
            )}
          >
            <MessageCircle size={12} />
            Comment
          </button>
          <button
            type="button"
            onClick={() => void submit("REQUEST_CHANGES")}
            disabled={busyEvent !== null}
            className={clsx(
              "flex min-h-9 items-center gap-1.5 rounded-md border border-status-failed/40 px-2.5 py-1.5 text-xs text-status-failed hover:bg-status-failed/10 disabled:opacity-40",
              FOCUS_RING,
            )}
          >
            <ThumbsDown size={12} />
            Request changes
          </button>
          <button
            type="button"
            onClick={() => void submit("APPROVE")}
            disabled={busyEvent !== null}
            className={clsx(
              "flex min-h-9 items-center gap-1.5 rounded-md bg-accent px-2.5 py-1.5 text-xs font-medium text-accent-ink hover:bg-accent-strong disabled:opacity-40",
              FOCUS_RING,
            )}
          >
            <ThumbsUp size={12} />
            Approve
          </button>
        </div>
      </div>
    </div>
  );
}
