import clsx from "clsx";
import type { NodeStatus } from "../../gen/types";
import { STATUS_LABEL } from "../../lib/constants";
import { StatusDot } from "../tree/StatusDot";

const TEXT_CLASS: Record<NodeStatus, string> = {
  idle: "text-status-idle",
  starting: "text-status-starting",
  running: "text-status-running",
  awaiting_input: "text-status-awaiting",
  done: "text-status-done",
  failed: "text-status-failed",
  interrupted: "text-status-interrupted",
};

interface StatusChipProps {
  status: NodeStatus;
  className?: string;
}

export function StatusChip({ status, className }: StatusChipProps) {
  return (
    <span
      className={clsx(
        "inline-flex items-center gap-1.5 rounded-md border border-border-strong bg-surface-2 px-2 py-1 text-2xs font-medium",
        TEXT_CLASS[status],
        className,
      )}
    >
      <StatusDot status={status} />
      {STATUS_LABEL[status]}
    </span>
  );
}
