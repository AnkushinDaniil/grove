import { useState } from "react";
import clsx from "clsx";
import { Link, useNavigate } from "react-router";
import { AlertTriangle, Check, Wrench } from "lucide-react";
import { apiClient } from "../../state/api";
import { useFeedbackStore } from "../../state/feedback";
import { useTreeStore } from "../../state/tree";
import { Breadcrumb } from "../node/Breadcrumb";
import { RelativeTime } from "../common/RelativeTime";
import { CHILD_KIND_FOR, FEEDBACK_KIND_LABEL, FOCUS_RING } from "../../lib/constants";
import type { Feedback } from "../../gen/types";

interface FeedbackItemProps {
  feedback: Feedback;
}

/** One feedback row: node path, kind+subject, the comment, and (while open)
 *  the two resolution actions. "Create fix task" spawns a task node as a
 *  child of the node the feedback was filed against, briefed with the
 *  feedback's own context, then links it back via resolveFeedback's
 *  fix_node_id -- closing the loop inside the tree per docs/API.md. */
export function FeedbackItem({ feedback }: FeedbackItemProps) {
  const navigate = useNavigate();
  const node = useTreeStore((s) => s.nodesById[feedback.node_id]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const resolved = Boolean(feedback.resolved_at);

  async function resolveOnly() {
    setBusy(true);
    setError(null);
    try {
      const updated = await apiClient.resolveFeedback(feedback.id);
      useFeedbackStore.getState().upsert(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function createFixTask() {
    setBusy(true);
    setError(null);
    try {
      const childKind = (node ? CHILD_KIND_FOR[node.kind] : null) ?? "task";
      const kindLabel = FEEDBACK_KIND_LABEL[feedback.kind];
      const title = `Fix: ${feedback.subject || kindLabel} feedback`;
      const brief = [
        `Fix task for ${kindLabel} feedback${feedback.subject ? ` on "${feedback.subject}"` : ""}, filed against this node.`,
        "",
        feedback.comment,
      ].join("\n");
      const created = await apiClient.createNode({ parent_id: feedback.node_id, kind: childKind, title, brief });
      const updated = await apiClient.resolveFeedback(feedback.id, created.id);
      useFeedbackStore.getState().upsert(updated);
      navigate(`/n/${created.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }

  return (
    <li className="border-b border-border px-4 py-2.5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Breadcrumb nodeId={feedback.node_id} />
        <RelativeTime iso={feedback.created_at} className="shrink-0 text-2xs text-ink-faint" />
      </div>
      <div className="mt-1.5 flex items-center gap-1.5 text-2xs">
        <span className="shrink-0 rounded-md border border-border-strong bg-surface-2 px-1.5 py-0.5 text-ink-muted">
          {FEEDBACK_KIND_LABEL[feedback.kind]}
        </span>
        {feedback.subject && <span className="truncate font-mono text-ink-muted">{feedback.subject}</span>}
      </div>
      <p className="mt-1.5 whitespace-pre-wrap font-sans text-xs text-ink">{feedback.comment}</p>

      {resolved ? (
        <p className="mt-2 flex flex-wrap items-center gap-1 text-2xs text-ink-faint">
          <Check size={11} className="shrink-0 text-status-done" />
          Resolved <RelativeTime iso={feedback.resolved_at} />
          {feedback.fix_node_id && (
            <>
              <span>--</span>
              <Link to={`/n/${feedback.fix_node_id}`} className="text-accent hover:underline">
                fix task
              </Link>
            </>
          )}
        </p>
      ) : (
        <div className="mt-2 flex items-center gap-1.5">
          <button
            type="button"
            onClick={() => void createFixTask()}
            disabled={busy}
            className={clsx(
              "flex items-center gap-1 rounded-md border border-border-strong px-2 py-1 text-2xs text-ink-muted hover:bg-hover hover:text-ink disabled:opacity-40",
              FOCUS_RING,
            )}
          >
            <Wrench size={11} />
            Create fix task
          </button>
          <button
            type="button"
            onClick={() => void resolveOnly()}
            disabled={busy}
            className={clsx(
              "flex items-center gap-1 rounded-md border border-border-strong px-2 py-1 text-2xs text-ink-muted hover:bg-hover hover:text-ink disabled:opacity-40",
              FOCUS_RING,
            )}
          >
            <Check size={11} />
            Resolve
          </button>
        </div>
      )}

      {error && (
        <div className="mt-1.5 flex items-center gap-1.5 text-2xs break-words text-status-failed">
          <AlertTriangle size={11} className="shrink-0" />
          {error}
        </div>
      )}
    </li>
  );
}
