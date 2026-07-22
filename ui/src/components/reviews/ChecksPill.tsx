import clsx from "clsx";
import { CheckCircle2, CircleDashed, Clock, XCircle } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { ReviewChecksState } from "../../gen/types";

const TEXT_CLASS: Record<ReviewChecksState, string> = {
  passing: "text-status-running",
  failing: "text-status-failed",
  pending: "text-status-awaiting",
  none: "text-ink-faint",
};

const ICON: Record<ReviewChecksState, LucideIcon> = {
  passing: CheckCircle2,
  failing: XCircle,
  pending: Clock,
  none: CircleDashed,
};

const LABEL: Record<ReviewChecksState, string> = {
  passing: "Passing",
  failing: "Failing",
  pending: "Pending",
  none: "None",
};

const TITLE: Record<ReviewChecksState, string> = {
  passing: "Checks passing",
  failing: "Checks failing",
  pending: "Checks pending",
  none: "No checks reported",
};

interface ChecksPillProps {
  checks: ReviewChecksState;
  className?: string;
}

/** Small CI-status pill for a PR row. Reuses grove's fixed status-hue
 *  vocabulary (teal=running/passing, amber=awaiting/pending, red=failed) so
 *  it reads consistently with StatusChip elsewhere instead of inventing new
 *  color tokens for what is semantically the same "state of an async
 *  process" signal. */
export function ChecksPill({ checks, className }: ChecksPillProps) {
  const Icon = ICON[checks];
  return (
    <span
      title={TITLE[checks]}
      className={clsx(
        "inline-flex items-center gap-1 rounded-md border border-border-strong bg-surface-2 px-1.5 py-0.5 text-2xs font-medium leading-none whitespace-nowrap",
        TEXT_CLASS[checks],
        className,
      )}
    >
      <Icon size={11} />
      {LABEL[checks]}
    </span>
  );
}
