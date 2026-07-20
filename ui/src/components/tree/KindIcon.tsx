import { Boxes, FolderGit2, GitBranch } from "lucide-react";
import type { NodeKind } from "../../gen/types";
import { KIND_LABEL } from "../../lib/constants";

interface KindIconProps {
  kind: NodeKind;
  size?: number;
  className?: string;
}

const ICONS: Record<NodeKind, typeof Boxes> = {
  workspace: Boxes,
  project: FolderGit2,
  task: GitBranch, // every task owns a git worktree + branch
};

export function KindIcon({ kind, size = 14, className }: KindIconProps) {
  const Icon = ICONS[kind];
  return (
    <span title={KIND_LABEL[kind]} className="inline-flex">
      <Icon size={size} className={className} aria-hidden="true" />
    </span>
  );
}
