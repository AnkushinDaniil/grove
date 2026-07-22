import { useState } from "react";
import clsx from "clsx";
import { Plus } from "lucide-react";
import { apiClient } from "../../../state/api";
import { useWorktreeReviewStore } from "../../../state/worktreeReview";
import { AiDraftField } from "../../reviewWorkspace/AiDraftField";
import { FOCUS_RING } from "../../../lib/constants";
import type { ReviewCommentSide } from "../../../gen/types";

interface WorktreeCommentComposerProps {
  node: string;
  repo: string;
  /** ai-draft is the PR endpoint, reused for worktree comments by passing
   *  the worktree path as `dir` and `pr: 0` (see docs/API.md). */
  worktreePath: string;
  path: string;
  side: ReviewCommentSide;
  line: number;
  onAdded: () => void;
  onCancel: () => void;
}

/** New-comment composer for the worktree review tab -- posts immediately
 *  (no draft/staging concept, unlike PR review's CommentComposer) since
 *  worktree notes are grove-local and never submitted anywhere external.
 *  Reuses AiDraftField directly so "Draft with AI" behaves identically to
 *  every other composer in the app. */
export function WorktreeCommentComposer({
  node,
  repo,
  worktreePath,
  path,
  side,
  line,
  onAdded,
  onCancel,
}: WorktreeCommentComposerProps) {
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function requestAiDraft(): Promise<string> {
    const res = await apiClient.aiDraft({
      dir: worktreePath,
      pr: 0,
      kind: "comment",
      path,
      line,
      instruction: body.trim() || undefined,
    });
    return res.text;
  }

  async function submit() {
    const trimmed = body.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    setError(null);
    try {
      const comment = await apiClient.addWorktreeComment({ node, repo, path, line, side, body: trimmed });
      useWorktreeReviewStore.getState().addCommentLocal(comment);
      onAdded();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="rounded-md border border-border-strong bg-surface-2 p-2">
      <AiDraftField
        value={body}
        onChange={setBody}
        ariaLabel="New worktree comment"
        placeholder="Leave a note for this worktree…"
        autoFocus
        disabled={busy}
        onRequestDraft={requestAiDraft}
      />
      <div className="mt-2 flex items-center justify-end gap-2">
        <button
          type="button"
          onClick={onCancel}
          className={clsx("rounded-md px-2.5 py-1.5 text-xs text-ink-muted hover:bg-hover hover:text-ink", FOCUS_RING)}
        >
          Cancel
        </button>
        <button
          type="button"
          onClick={() => void submit()}
          disabled={!body.trim() || busy}
          className={clsx(
            "flex items-center gap-1.5 rounded-md bg-accent px-2.5 py-1.5 text-xs font-medium text-accent-ink hover:bg-accent-strong disabled:opacity-40",
            FOCUS_RING,
          )}
        >
          <Plus size={12} />
          Add comment
        </button>
      </div>
      {error && <p className="mt-1.5 text-2xs break-words text-status-failed">{error}</p>}
    </div>
  );
}
