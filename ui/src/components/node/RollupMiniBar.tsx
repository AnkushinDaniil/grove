import clsx from "clsx";
import { useRollup } from "../../hooks/useRollup";
import type { NodeID, NodeStatus } from "../../gen/types";

const BAR_COLOR: Record<NodeStatus, string> = {
  idle: "bg-status-idle",
  starting: "bg-status-starting",
  running: "bg-status-running",
  awaiting_input: "bg-status-awaiting",
  done: "bg-status-done",
  failed: "bg-status-failed",
  interrupted: "bg-status-interrupted",
};

// Worst-first reading order, like a health bar.
const ORDER: NodeStatus[] = ["failed", "awaiting_input", "starting", "running", "interrupted", "done", "idle"];

interface RollupMiniBarProps {
  nodeId: NodeID;
  className?: string;
}

/** Tiny stacked bar showing the proportion of descendant statuses. */
export function RollupMiniBar({ nodeId, className }: RollupMiniBarProps) {
  const rollup = useRollup(nodeId);
  if (rollup.total === 0) return null;

  return (
    <div
      className={clsx("flex h-1.5 w-16 shrink-0 overflow-hidden rounded-full bg-surface-2", className)}
      title={`${rollup.total} descendant${rollup.total === 1 ? "" : "s"}`}
    >
      {ORDER.filter((status) => rollup.byStatus[status] > 0).map((status) => (
        <span
          key={status}
          className={BAR_COLOR[status]}
          style={{ width: `${(rollup.byStatus[status] / rollup.total) * 100}%` }}
        />
      ))}
    </div>
  );
}
