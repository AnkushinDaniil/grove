import type { ReactNode } from "react";
import type { ContentOmittedReason, PRFileStatus, ReviewCommentSide } from "../../gen/types";

/** File shape DiffView renders. Structurally identical to both PRReviewFile
 *  and WorktreeFile (see docs/API.md's "Diff content for rich rendering
 *  (Pierre)"), so either can be passed straight through without mapping. */
export interface DiffViewFile {
  path: string;
  old_path?: string;
  status: PRFileStatus;
  additions: number;
  deletions: number;
  binary: boolean;
  original_content: string;
  modified_content: string;
  content_omitted: ContentOmittedReason;
}

/** One anchored comment, pre-rendered by the caller. DiffView only needs to
 *  know where it anchors (path+side+line) -- not whether it's a GitHub
 *  thread with replies, a pending draft, or a local worktree note. Callers
 *  render the actual card (ThreadCard, DraftPendingCard, a worktree comment
 *  card, ...) and hand the result in as `content`. */
export interface DiffViewComment {
  /** React list key -- not interpreted otherwise. */
  id: string;
  path: string;
  side: ReviewCommentSide;
  line: number;
  content: ReactNode;
}

export interface DiffViewComposerTarget {
  path: string;
  side: ReviewCommentSide;
  line: number;
}

export type DiffStyle = "unified" | "split";
