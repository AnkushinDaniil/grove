import { useState } from "react";
import clsx from "clsx";
import { X } from "lucide-react";
import { apiClient } from "../../../state/api";
import { useWorktreeReviewStore } from "../../../state/worktreeReview";
import { RelativeTime } from "../../common/RelativeTime";
import { FOCUS_RING } from "../../../lib/constants";
import type { WorktreeComment } from "../../../gen/types";

interface WorktreeCommentCardProps {
  comment: WorktreeComment;
}

/** A local worktree review note -- unlike PR threads/drafts there's no
 *  author or reply/resolve concept, just a note keyed to a line and a
 *  delete affordance. Visually mirrors DraftPendingCard (same "pending,
 *  locally-held" family) since it plays the same role: something the user
 *  will act on later, either by deleting it or via "Address with agent". */
export function WorktreeCommentCard({ comment }: WorktreeCommentCardProps) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function remove() {
    setBusy(true);
    setError(null);
    try {
      await apiClient.deleteWorktreeComment(comment.id);
      useWorktreeReviewStore.getState().removeCommentLocal(comment.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }

  return (
    <div className="rounded-md border border-dashed border-accent/40 bg-accent-soft/40 px-2.5 py-2">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5 text-2xs">
            <span className="font-medium text-accent">Note</span>
            <RelativeTime iso={comment.created_at} className="text-ink-faint" />
          </div>
          <p className="mt-1 whitespace-pre-wrap font-sans text-xs text-ink-muted">{comment.body}</p>
        </div>
        <button
          type="button"
          onClick={() => void remove()}
          disabled={busy}
          aria-label="Remove comment"
          title="Remove comment"
          className={clsx(
            "flex h-6 w-6 shrink-0 items-center justify-center rounded text-ink-faint hover:bg-hover hover:text-status-failed disabled:opacity-40",
            FOCUS_RING,
          )}
        >
          <X size={12} />
        </button>
      </div>
      {error && <p className="mt-1 text-2xs break-words text-status-failed">{error}</p>}
    </div>
  );
}
