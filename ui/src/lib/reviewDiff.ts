import type { DiffLine, DraftComment, ReviewCommentSide, ReviewThread } from "../gen/types";

export interface LineAnchor {
  side: ReviewCommentSide;
  line: number;
}

/** Where a new inline comment on this diff line would anchor: the RIGHT
 *  (new-file) line number when the line exists on the new side (context or
 *  addition), else the LEFT (old-file) line number for a pure deletion.
 *  Returns null only if neither line number is set, which the wire
 *  contract shouldn't produce. */
export function resolveLineAnchor(line: DiffLine): LineAnchor | null {
  if (line.new_line > 0) return { side: "RIGHT", line: line.new_line };
  if (line.old_line > 0) return { side: "LEFT", line: line.old_line };
  return null;
}

interface Anchored {
  path: string;
  side: ReviewCommentSide;
  line: number;
}

function matchAnchor<T extends Anchored>(items: T[], path: string, anchor: LineAnchor): T[] {
  return items.filter((item) => item.path === path && item.side === anchor.side && item.line === anchor.line);
}

/** Existing review threads anchored at exactly this file+side+line. */
export function threadsForAnchor(threads: ReviewThread[], path: string, anchor: LineAnchor): ReviewThread[] {
  return matchAnchor(threads, path, anchor);
}

/** Pending draft comments anchored at exactly this file+side+line. */
export function draftsForAnchor(drafts: DraftComment[], path: string, anchor: LineAnchor): DraftComment[] {
  return matchAnchor(drafts, path, anchor);
}
