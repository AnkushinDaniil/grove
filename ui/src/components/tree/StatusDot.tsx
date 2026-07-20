import clsx from "clsx";
import type { NodeStatus } from "../../gen/types";
import { PULSING_STATUSES, STATUS_LABEL } from "../../lib/constants";

const STATUS_DOT_CLASS: Record<NodeStatus, string> = {
  idle: "bg-status-idle",
  starting: "bg-status-starting",
  running: "bg-status-running",
  awaiting_input: "bg-status-awaiting",
  done: "bg-status-done",
  failed: "bg-status-failed",
  interrupted: "bg-status-interrupted",
};

interface StatusDotProps {
  status: NodeStatus;
  size?: "sm" | "md";
  className?: string;
}

/** Status indicator: a solid dot, with a radar-ping ring layered behind it
 *  for the "something is actively happening" statuses. */
export function StatusDot({ status, size = "sm", className }: StatusDotProps) {
  const dim = size === "sm" ? "h-1.5 w-1.5" : "h-2 w-2";
  const pulsing = PULSING_STATUSES.has(status);

  return (
    <span
      className={clsx("relative inline-flex shrink-0", dim, className)}
      role="img"
      aria-label={STATUS_LABEL[status]}
      title={STATUS_LABEL[status]}
    >
      {pulsing && (
        <span
          className={clsx(
            "absolute inline-flex h-full w-full animate-ping rounded-full opacity-60",
            STATUS_DOT_CLASS[status],
          )}
        />
      )}
      <span className={clsx("relative inline-flex rounded-full", dim, STATUS_DOT_CLASS[status])} />
    </span>
  );
}
