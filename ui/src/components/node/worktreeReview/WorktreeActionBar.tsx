import { useState } from "react";
import clsx from "clsx";
import { AlertTriangle, Bot, Check, GitMerge } from "lucide-react";
import { apiClient } from "../../../state/api";
import { loadWorktreeReview } from "../../../state/worktreeReview";
import { FOCUS_RING } from "../../../lib/constants";

interface WorktreeActionBarProps {
  node: string;
  repo: string;
  hasChanges: boolean;
  commentCount: number;
  /** Called after addressWorktree succeeds so the caller can switch the
   *  node view to its Terminal tab -- this component only starts the
   *  session, NodeView owns the tab strip. */
  onAddressed: () => void;
}

/** Sticky bottom bar for the worktree review tab: merge the worktree into
 *  its parent, or hand the local comments to the agent as a fix prompt.
 *  Mirrors SubmitBar's busy/result banner pattern. */
export function WorktreeActionBar({ node, repo, hasChanges, commentCount, onAddressed }: WorktreeActionBarProps) {
  const [busy, setBusy] = useState<"merge" | "address" | null>(null);
  const [result, setResult] = useState<{ tone: "success" | "error"; message: string } | null>(null);

  async function merge() {
    setBusy("merge");
    setResult(null);
    try {
      const res = await apiClient.mergeWorktree(node, repo);
      setResult({ tone: res.merged ? "success" : "error", message: res.message });
      if (res.merged) await loadWorktreeReview(node, repo);
    } catch (err) {
      setResult({ tone: "error", message: err instanceof Error ? err.message : String(err) });
    } finally {
      setBusy(null);
    }
  }

  async function address() {
    setBusy("address");
    setResult(null);
    try {
      await apiClient.addressWorktree(node, repo);
      onAddressed();
    } catch (err) {
      setResult({ tone: "error", message: err instanceof Error ? err.message : String(err) });
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="shrink-0 space-y-2 border-t border-border bg-surface px-4 py-3">
      {result && (
        <div
          className={clsx(
            "flex items-center gap-2 rounded-md border px-2.5 py-1.5 text-xs",
            result.tone === "success"
              ? "border-accent/30 bg-accent-soft text-accent"
              : "border-status-failed/40 bg-status-failed/10 text-status-failed",
          )}
        >
          {result.tone === "success" ? <Check size={13} className="shrink-0" /> : <AlertTriangle size={13} className="shrink-0" />}
          <span className="min-w-0 flex-1 break-words">{result.message}</span>
          <button type="button" onClick={() => setResult(null)} className="shrink-0 text-2xs opacity-70 hover:opacity-100">
            dismiss
          </button>
        </div>
      )}
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        <span className="mr-auto text-2xs text-ink-faint">
          {commentCount} local comment{commentCount === 1 ? "" : "s"}
        </span>
        <button
          type="button"
          onClick={() => void address()}
          disabled={busy !== null || commentCount === 0}
          title={commentCount === 0 ? "Leave at least one comment first" : "Compose the comments into a prompt and start a session on this node"}
          className={clsx(
            "flex min-h-9 items-center gap-1.5 rounded-md border border-border-strong px-2.5 py-1.5 text-xs text-ink-muted hover:bg-hover hover:text-ink disabled:opacity-40",
            FOCUS_RING,
          )}
        >
          <Bot size={13} />
          Address with agent
        </button>
        <button
          type="button"
          onClick={() => void merge()}
          disabled={busy !== null || !hasChanges}
          title={hasChanges ? "Squash-merge this worktree into its parent" : "No changes to merge"}
          className={clsx(
            "flex min-h-9 items-center gap-1.5 rounded-md bg-accent px-2.5 py-1.5 text-xs font-medium text-accent-ink hover:bg-accent-strong disabled:opacity-40",
            FOCUS_RING,
          )}
        >
          <GitMerge size={13} />
          Merge to parent
        </button>
      </div>
    </div>
  );
}
