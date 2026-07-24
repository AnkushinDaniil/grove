import { useState } from "react";
import clsx from "clsx";
import { Loader2, Send } from "lucide-react";
import { sendReviewChat, useReviewWorkspaceStore } from "../../state/reviewWorkspace";
import { FOCUS_RING } from "../../lib/constants";

interface ReviewChatProps {
  dir: string;
  pr: number;
}

/** Chat with the review session the ai-review pass created: it still has the
 *  PR, the injected codebase context, and its own findings in view, so you can
 *  ask about a specific finding ("why is the one on line 92 a bug?"). Only
 *  available once a review has run (there is a session to resume). */
export function ReviewChat({ dir, pr }: ReviewChatProps) {
  const messages = useReviewWorkspaceStore((s) => s.chatMessages);
  const sending = useReviewWorkspaceStore((s) => s.chatSending);
  const error = useReviewWorkspaceStore((s) => s.chatError);
  const ran = useReviewWorkspaceStore((s) => s.aiReviewRan);
  const [draft, setDraft] = useState("");

  function submit() {
    const text = draft.trim();
    if (text === "" || sending) return;
    setDraft("");
    void sendReviewChat(dir, pr, text);
  }

  if (!ran) return null; // no session to resume until a review has run

  return (
    <div className="flex shrink-0 flex-col border-t border-border">
      <div className="px-3 py-1.5 text-2xs font-semibold tracking-wide text-ink-muted uppercase">Ask the reviewer</div>

      {messages.length > 0 && (
        <div className="max-h-52 space-y-1.5 overflow-y-auto px-3 pb-2">
          {messages.map((m, i) => (
            <div
              key={i}
              className={clsx(
                "whitespace-pre-wrap rounded-md px-2 py-1.5 text-xs",
                m.role === "user" ? "bg-accent-soft/50 text-ink" : "bg-surface-2/60 text-ink-muted",
              )}
            >
              {m.text}
            </div>
          ))}
          {sending && (
            <div className="flex items-center gap-1.5 px-2 text-2xs text-ink-faint">
              <Loader2 size={11} className="animate-spin" /> Thinking...
            </div>
          )}
        </div>
      )}

      {error && <p className="px-3 pb-1 text-2xs break-words text-status-failed">{error}</p>}

      <div className="flex items-end gap-1.5 px-3 pb-2.5">
        <textarea
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              submit();
            }
          }}
          rows={1}
          placeholder="Ask about a finding, the PR, or a fix..."
          aria-label="Ask the reviewer"
          className={clsx(
            "min-h-8 flex-1 resize-none rounded-md border border-border bg-canvas px-2 py-1.5 font-sans text-xs text-ink placeholder:text-ink-faint",
            FOCUS_RING,
          )}
        />
        <button
          type="button"
          onClick={submit}
          disabled={sending || draft.trim() === ""}
          aria-label="Send"
          className={clsx(
            "flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-accent/30 bg-accent-soft text-accent hover:bg-accent-soft/70 disabled:opacity-40",
            FOCUS_RING,
          )}
        >
          {sending ? <Loader2 size={13} className="animate-spin" /> : <Send size={13} />}
        </button>
      </div>
    </div>
  );
}
