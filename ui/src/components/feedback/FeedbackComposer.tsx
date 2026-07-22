import { useEffect, useRef, useState } from "react";
import clsx from "clsx";
import { ThumbsDown } from "lucide-react";
import { apiClient } from "../../state/api";
import { FEEDBACK_KIND_LABEL, FOCUS_RING } from "../../lib/constants";
import type { EventID, FeedbackKind, NodeID, SessionID } from "../../gen/types";

const KIND_OPTIONS: FeedbackKind[] = ["skill", "tool", "model", "agent", "other"];

interface FeedbackComposerProps {
  nodeId: NodeID;
  sessionId?: SessionID;
  eventId?: EventID;
  initialKind: FeedbackKind;
  initialSubject?: string;
  onSubmitted: () => void;
  onCancel: () => void;
}

/** Small composer for POST /feedback: kind select (prefilled, still
 *  editable), an optional subject, and a required comment. Plain textarea
 *  rather than CommentComposer's AiDraftField -- this is the reporter's own
 *  read of what went wrong, not something to AI-draft. Shows a brief
 *  confirmation before collapsing, mirroring NodeView's auto-ack delay. */
export function FeedbackComposer({
  nodeId,
  sessionId,
  eventId,
  initialKind,
  initialSubject = "",
  onSubmitted,
  onCancel,
}: FeedbackComposerProps) {
  const [kind, setKind] = useState<FeedbackKind>(initialKind);
  const [subject, setSubject] = useState(initialSubject);
  const [comment, setComment] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  useEffect(() => {
    if (!sent) return;
    const timer = setTimeout(onSubmitted, 1100);
    return () => clearTimeout(timer);
  }, [sent, onSubmitted]);

  async function submit() {
    const trimmed = comment.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    setError(null);
    try {
      await apiClient.createFeedback({
        node_id: nodeId,
        session_id: sessionId,
        event_id: eventId,
        kind,
        subject: subject.trim() || undefined,
        comment: trimmed,
      });
      setSent(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  if (sent) {
    return (
      <div role="status" className="rounded-md border border-accent/30 bg-accent-soft px-3 py-2 text-xs text-accent">
        Thanks -- feedback sent.
      </div>
    );
  }

  return (
    <div className="rounded-md border border-border-strong bg-surface-2 p-2">
      <div className="mb-2 flex flex-wrap items-center gap-1.5">
        <label className="text-2xs text-ink-faint" htmlFor="feedback-kind">
          What's this about?
        </label>
        <select
          id="feedback-kind"
          value={kind}
          onChange={(e) => setKind(e.target.value as FeedbackKind)}
          className={clsx("rounded-md border border-border bg-canvas px-1.5 py-1 text-2xs text-ink", FOCUS_RING)}
        >
          {KIND_OPTIONS.map((k) => (
            <option key={k} value={k}>
              {FEEDBACK_KIND_LABEL[k]}
            </option>
          ))}
        </select>
        <input
          value={subject}
          onChange={(e) => setSubject(e.target.value)}
          placeholder="Subject (optional) -- a skill, tool, or model name"
          aria-label="Subject"
          className={clsx(
            "min-w-0 flex-1 rounded-md border border-border bg-canvas px-2 py-1 font-mono text-2xs text-ink placeholder:text-ink-faint",
            FOCUS_RING,
          )}
        />
      </div>
      <textarea
        ref={textareaRef}
        value={comment}
        onChange={(e) => setComment(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            void submit();
          } else if (e.key === "Escape") {
            onCancel();
          }
        }}
        rows={3}
        placeholder="What went wrong?"
        aria-label="Feedback comment"
        className={clsx(
          "w-full resize-none rounded-md border border-border bg-canvas px-2 py-1.5 font-sans text-xs text-ink placeholder:text-ink-faint",
          FOCUS_RING,
        )}
      />
      <div className="mt-2 flex items-center justify-end gap-2">
        <span className="mr-auto text-2xs text-ink-disabled">⌘Enter to send</span>
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
          disabled={!comment.trim() || busy}
          className={clsx(
            "flex items-center gap-1.5 rounded-md bg-accent px-2.5 py-1.5 text-xs font-medium text-accent-ink hover:bg-accent-strong disabled:opacity-40",
            FOCUS_RING,
          )}
        >
          <ThumbsDown size={12} />
          Send feedback
        </button>
      </div>
      {error && <p className="mt-1.5 text-2xs break-words text-status-failed">{error}</p>}
    </div>
  );
}
