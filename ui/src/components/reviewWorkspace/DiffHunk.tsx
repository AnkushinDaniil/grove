import { Fragment } from "react";
import clsx from "clsx";
import { Plus } from "lucide-react";
import { draftsForAnchor, resolveLineAnchor, threadsForAnchor } from "../../lib/reviewDiff";
import { ThreadCard } from "./ThreadCard";
import { DraftPendingCard } from "./DraftPendingCard";
import { CommentComposer } from "./CommentComposer";
import { FOCUS_RING } from "../../lib/constants";
import type { ActiveComposerTarget } from "./ReviewWorkspace";
import type { DiffHunk as DiffHunkT, DiffLine, DraftComment, ReviewCommentSide, ReviewThread } from "../../gen/types";

const OP_ROW_CLASS: Record<DiffLine["op"], string> = {
  " ": "",
  "+": "bg-diff-add/10",
  "-": "bg-status-failed/10",
};

const OP_MARKER_CLASS: Record<DiffLine["op"], string> = {
  " ": "text-ink-disabled",
  "+": "text-diff-add",
  "-": "text-status-failed",
};

interface DiffHunkProps {
  hunk: DiffHunkT;
  path: string;
  dir: string;
  pr: number;
  threads: ReviewThread[];
  drafts: DraftComment[];
  activeComposer: ActiveComposerTarget | null;
  onOpenComposer: (side: ReviewCommentSide, line: number) => void;
  onCloseComposer: () => void;
}

/** One hunk of one file's diff, rendered as a gutter+code table. Each line
 *  has a hover "+" affordance in the gutter to start a comment there;
 *  existing threads and pending drafts anchored at a line render inline
 *  directly beneath it. */
export function DiffHunk({ hunk, path, dir, pr, threads, drafts, activeComposer, onOpenComposer, onCloseComposer }: DiffHunkProps) {
  return (
    <table className="w-full border-collapse text-2xs leading-relaxed">
      <tbody>
        <tr>
          <td colSpan={4} className="whitespace-pre bg-surface-2 px-3 py-1 font-mono text-ink-faint">
            {hunk.header}
          </td>
        </tr>
        {hunk.lines.map((line, i) => {
          const anchor = resolveLineAnchor(line);
          const lineThreads = anchor ? threadsForAnchor(threads, path, anchor) : [];
          const lineDrafts = anchor ? draftsForAnchor(drafts, path, anchor) : [];
          const composerOpen =
            anchor !== null && activeComposer?.side === anchor.side && activeComposer.line === anchor.line;
          const hasExtra = lineThreads.length > 0 || lineDrafts.length > 0 || composerOpen;

          return (
            <Fragment key={i}>
              <tr className={clsx("group", OP_ROW_CLASS[line.op])}>
                <td className="w-12 select-none px-1.5 text-right align-top font-mono text-ink-faint">
                  {line.op !== "+" ? line.old_line : ""}
                </td>
                <td className="w-12 select-none px-1.5 text-right align-top font-mono text-ink-faint">
                  {line.op !== "-" ? line.new_line : ""}
                </td>
                <td className="w-6 select-none align-top">
                  <div className="relative flex h-[1.7em] items-center justify-center">
                    <span
                      className={clsx(
                        "pointer-events-none font-mono",
                        OP_MARKER_CLASS[line.op],
                        anchor && "group-hover:opacity-0",
                      )}
                    >
                      {line.op === " " ? " " : line.op}
                    </span>
                    {anchor && (
                      <button
                        type="button"
                        onClick={() => onOpenComposer(anchor.side, anchor.line)}
                        aria-label="Add a comment on this line"
                        title="Add a comment on this line"
                        className={clsx(
                          "absolute inset-0 m-auto flex h-4 w-4 items-center justify-center rounded-sm bg-accent text-accent-ink opacity-0 group-hover:opacity-100 focus-visible:opacity-100",
                          FOCUS_RING,
                        )}
                      >
                        <Plus size={10} strokeWidth={3} />
                      </button>
                    )}
                  </div>
                </td>
                <td className="whitespace-pre px-2 font-mono text-ink">{line.text || " "}</td>
              </tr>
              {hasExtra && (
                <tr>
                  <td colSpan={4} className="bg-canvas/60 px-3 py-2">
                    <div className="space-y-2">
                      {lineThreads.map((t) => (
                        <ThreadCard key={t.id} thread={t} dir={dir} pr={pr} />
                      ))}
                      {lineDrafts.map((d) => (
                        <DraftPendingCard key={d.id} draft={d} />
                      ))}
                      {composerOpen && anchor && (
                        <CommentComposer
                          mode="new"
                          dir={dir}
                          pr={pr}
                          path={path}
                          side={anchor.side}
                          line={anchor.line}
                          onAdded={onCloseComposer}
                          onCancel={onCloseComposer}
                        />
                      )}
                    </div>
                  </td>
                </tr>
              )}
            </Fragment>
          );
        })}
      </tbody>
    </table>
  );
}
