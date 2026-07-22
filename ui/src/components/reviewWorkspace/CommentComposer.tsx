import { useState } from "react";
import clsx from "clsx";
import { CornerDownRight, Plus } from "lucide-react";
import { apiClient } from "../../state/api";
import { useReviewWorkspaceStore } from "../../state/reviewWorkspace";
import { AiDraftField } from "./AiDraftField";
import { FOCUS_RING } from "../../lib/constants";
import type { ReviewCommentSide } from "../../gen/types";

interface NewCommentComposerProps {
  mode: "new";
  dir: string;
  pr: number;
  path: string;
  side: ReviewCommentSide;
  line: number;
  onAdded: () => void;
  onCancel: () => void;
}

interface ReplyComposerProps {
  mode: "reply";
  dir: string;
  pr: number;
  threadId: string;
  autoFocus?: boolean;
  onReplied: () => void;
  onCancel: () => void;
}

type CommentComposerProps = NewCommentComposerProps | ReplyComposerProps;

/** Shared line-comment / thread-reply composer: a textarea with an
 *  AI-drafting assist, plus mode-specific submit semantics. New-line
 *  comments become pending DraftComments (batched into the review,
 *  submitted later); thread replies post immediately per the frozen API
 *  contract, with an optional "resolve thread" checkbox. */
export function CommentComposer(props: CommentComposerProps) {
  const { dir, pr } = props;
  const [body, setBody] = useState("");
  const [resolve, setResolve] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function requestAiDraft(): Promise<string> {
    const instruction = body.trim() || undefined;
    if (props.mode === "new") {
      const res = await apiClient.aiDraft({ dir, pr, kind: "comment", path: props.path, line: props.line, instruction });
      return res.text;
    }
    const res = await apiClient.aiDraft({ dir, pr, kind: "reply", thread_id: props.threadId, instruction });
    return res.text;
  }

  async function submit() {
    const trimmed = body.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    setError(null);
    try {
      if (props.mode === "new") {
        const draft = await apiClient.addReviewDraft({
          dir,
          pr,
          path: props.path,
          line: props.line,
          side: props.side,
          body: trimmed,
        });
        useReviewWorkspaceStore.getState().addDraftLocal(draft);
        props.onAdded();
      } else {
        await apiClient.replyToThread({ dir, pr, thread_id: props.threadId, body: trimmed, resolve });
        // Refetch rather than fabricate the new comment locally -- we don't
        // reliably know the viewer's own GitHub login to stamp as author.
        const refreshed = await apiClient.getPRReview(dir, pr);
        useReviewWorkspaceStore.getState().setReview(refreshed);
        props.onReplied();
      }
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
        ariaLabel={props.mode === "new" ? "New comment" : "Reply"}
        placeholder={props.mode === "new" ? "Leave a comment…" : "Write a reply…"}
        autoFocus={props.mode === "new" ? true : props.autoFocus}
        disabled={busy}
        onRequestDraft={requestAiDraft}
      />
      <div className="mt-2 flex items-center justify-end gap-2">
        {props.mode === "reply" && (
          <label className="mr-auto flex items-center gap-1.5 text-2xs text-ink-faint">
            <input
              type="checkbox"
              checked={resolve}
              onChange={(e) => setResolve(e.target.checked)}
              className="accent-accent"
            />
            Resolve thread
          </label>
        )}
        <button
          type="button"
          onClick={props.onCancel}
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
          {props.mode === "new" ? <Plus size={12} /> : <CornerDownRight size={12} />}
          {props.mode === "new" ? "Add draft" : "Reply"}
        </button>
      </div>
      {error && <p className="mt-1.5 text-2xs break-words text-status-failed">{error}</p>}
    </div>
  );
}
