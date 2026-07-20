import { useEffect, useRef } from "react";
import clsx from "clsx";
import { ChevronRight } from "lucide-react";
import { Link } from "react-router";
import type { NodeID } from "../../gen/types";
import { useTreeStore } from "../../state/tree";
import { selectInboxCountForNode, useInboxStore } from "../../state/inbox";
import { StatusDot } from "./StatusDot";
import { AttentionBadge } from "./AttentionBadge";
import { KindIcon } from "./KindIcon";
import { FOCUS_RING } from "../../lib/constants";

interface TreeNodeRowProps {
  nodeId: NodeID;
  depth: number;
  expanded: boolean;
  hasChildren: boolean;
  focused: boolean;
  active: boolean;
  onToggle: () => void;
  onSelect: () => void;
}

/** One row in the tree rail. Uses roving tabindex: only the keyboard-cursor
 *  row is tab-reachable, and gaining `focused` moves real DOM focus onto it
 *  so the standard focus ring and screen readers both just work. */
export function TreeNodeRow({
  nodeId,
  depth,
  expanded,
  hasChildren,
  focused,
  active,
  onToggle,
  onSelect,
}: TreeNodeRowProps) {
  const node = useTreeStore((s) => s.nodesById[nodeId]);
  const attentionCount = useInboxStore((s) => selectInboxCountForNode(s, nodeId));
  const rowRef = useRef<HTMLAnchorElement | null>(null);

  useEffect(() => {
    if (!focused) return;
    rowRef.current?.focus();
    rowRef.current?.scrollIntoView({ block: "nearest" });
  }, [focused]);

  if (!node) return null;

  return (
    <Link
      ref={rowRef}
      to={`/n/${nodeId}`}
      onClick={onSelect}
      tabIndex={focused ? 0 : -1}
      role="treeitem"
      aria-level={depth + 1}
      aria-expanded={hasChildren ? expanded : undefined}
      aria-selected={active}
      data-node-id={nodeId}
      className={clsx(
        "group flex min-h-11 items-center gap-1.5 rounded-md py-1 pr-2 text-xs no-underline transition-colors md:min-h-0",
        active ? "bg-active text-ink" : "text-ink-muted hover:bg-hover hover:text-ink",
        FOCUS_RING,
      )}
      style={{ paddingLeft: `${6 + depth * 14}px` }}
    >
      <button
        type="button"
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          onToggle();
        }}
        className={clsx(
          "flex h-11 w-11 shrink-0 items-center justify-center rounded text-ink-faint hover:text-ink md:h-4 md:w-4",
          !hasChildren && "invisible",
        )}
        tabIndex={-1}
        aria-label={expanded ? "Collapse" : "Expand"}
      >
        <ChevronRight size={12} className={clsx("transition-transform", expanded && "rotate-90")} />
      </button>
      <KindIcon kind={node.kind} size={13} className="shrink-0 text-ink-faint" />
      <StatusDot status={node.status} />
      <span className="min-w-0 flex-1 truncate">{node.title}</span>
      <AttentionBadge count={attentionCount} />
    </Link>
  );
}
