import { useState } from "react";
import clsx from "clsx";
import { X } from "lucide-react";
import { apiClient } from "../../state/api";
import { useReviewWorkspaceStore } from "../../state/reviewWorkspace";
import { FOCUS_RING } from "../../lib/constants";
import type { DraftComment } from "../../gen/types";

interface DraftPendingCardProps {
  draft: DraftComment;
  /** Rail entries show the path:line location; inline (anchored in the
   *  diff) cards omit it since the surrounding diff already shows where. */
  showLocation?: boolean;
}

/** A pending (not-yet-submitted) draft comment. Rendered both inline in the
 *  diff at its anchor line and in the drafts rail -- same component, so
 *  removing it from either place stays in sync via the shared store. */
export function DraftPendingCard({ draft, showLocation }: DraftPendingCardProps) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function remove() {
    setBusy(true);
    setError(null);
    try {
      await apiClient.deleteReviewDraft(draft.id);
      useReviewWorkspaceStore.getState().removeDraftLocal(draft.id);
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
            <span className="font-medium text-accent">Draft</span>
            {showLocation && (
              <span className="truncate font-mono text-ink-faint" title={draft.path}>
                {draft.path}:{draft.line}
              </span>
            )}
          </div>
          <p className="mt-1 line-clamp-3 whitespace-pre-wrap font-sans text-xs text-ink-muted">{draft.body}</p>
        </div>
        <button
          type="button"
          onClick={() => void remove()}
          disabled={busy}
          aria-label="Remove draft"
          title="Remove draft"
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
