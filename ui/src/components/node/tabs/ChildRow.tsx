import { KindIcon } from "../../tree/KindIcon";
import { StatusChip } from "../StatusChip";
import { RollupMiniBar } from "../RollupMiniBar";
import { AttentionBadge } from "../../tree/AttentionBadge";
import { useTreeStore } from "../../../state/tree";
import { selectInboxCountForNode, useInboxStore } from "../../../state/inbox";
import type { NodeID } from "../../../gen/types";

interface ChildRowProps {
  nodeId: NodeID;
  onOpen: () => void;
}

export function ChildRow({ nodeId, onOpen }: ChildRowProps) {
  const node = useTreeStore((s) => s.nodesById[nodeId]);
  const attentionCount = useInboxStore((s) => selectInboxCountForNode(s, nodeId));
  if (!node) return null;

  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        className="flex min-h-11 w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left text-xs hover:bg-hover md:min-h-0"
      >
        <KindIcon kind={node.kind} size={13} className="shrink-0 text-ink-faint" />
        <span className="min-w-0 flex-1 truncate text-ink">{node.title}</span>
        <RollupMiniBar nodeId={nodeId} />
        <StatusChip status={node.status} className="shrink-0" />
        <AttentionBadge count={attentionCount} />
      </button>
    </li>
  );
}
