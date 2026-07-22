import { useState } from "react";
import clsx from "clsx";
import { CheckCircle2, MessageSquare } from "lucide-react";
import { RelativeTime } from "../common/RelativeTime";
import { Pill } from "../common/Pill";
import { CommentComposer } from "./CommentComposer";
import { FOCUS_RING } from "../../lib/constants";
import type { ReviewThread } from "../../gen/types";

interface ThreadCardProps {
  thread: ReviewThread;
  dir: string;
  pr: number;
}

/** An existing review thread anchored at a diff line: its comments (author,
 *  body, relative time, a resolved tag) plus a collapsed "Reply…" trigger
 *  that expands into the shared comment composer. */
export function ThreadCard({ thread, dir, pr }: ThreadCardProps) {
  const [replyOpen, setReplyOpen] = useState(false);

  return (
    <div className="rounded-md border border-border-strong bg-surface shadow-panel">
      <div className="flex items-center gap-1.5 border-b border-border px-2.5 py-1.5">
        <MessageSquare size={11} className="shrink-0 text-ink-faint" />
        <span className="text-2xs font-medium text-ink-faint">
          {thread.comments.length} comment{thread.comments.length === 1 ? "" : "s"}
        </span>
        {thread.is_resolved && (
          <Pill tone="accent" className="ml-auto">
            <CheckCircle2 size={10} />
            Resolved
          </Pill>
        )}
      </div>
      <ul className="divide-y divide-border/60">
        {thread.comments.map((c) => (
          <li key={c.id} className="px-2.5 py-2">
            <div className="flex items-center gap-1.5 text-2xs">
              <span className={clsx("font-medium", c.is_mine ? "text-accent" : "text-ink")}>{c.author}</span>
              <RelativeTime iso={c.created_at} className="text-ink-faint" />
            </div>
            <p className="mt-1 whitespace-pre-wrap font-sans text-xs text-ink-muted">{c.body}</p>
          </li>
        ))}
      </ul>
      <div className="border-t border-border p-2">
        {replyOpen ? (
          <CommentComposer
            mode="reply"
            dir={dir}
            pr={pr}
            threadId={thread.id}
            autoFocus
            onReplied={() => setReplyOpen(false)}
            onCancel={() => setReplyOpen(false)}
          />
        ) : (
          <button
            type="button"
            onClick={() => setReplyOpen(true)}
            className={clsx(
              "w-full rounded-md border border-dashed border-border px-2 py-1.5 text-left text-2xs text-ink-faint hover:border-border-strong hover:text-ink-muted",
              FOCUS_RING,
            )}
          >
            Reply…
          </button>
        )}
      </div>
    </div>
  );
}
