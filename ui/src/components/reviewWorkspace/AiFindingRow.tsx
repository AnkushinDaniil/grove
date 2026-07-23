import clsx from "clsx";
import { SquareArrowOutUpRight, X } from "lucide-react";
import { useReviewWorkspaceStore } from "../../state/reviewWorkspace";
import type { LocalFinding } from "../../state/reviewWorkspace";
import { FOCUS_RING } from "../../lib/constants";
import type { AiFindingSeverity } from "../../gen/types";

const SEVERITY_TEXT: Record<AiFindingSeverity, string> = {
  issue: "text-status-failed",
  suggestion: "text-accent",
  nit: "text-ink-faint",
};

interface AiFindingRowProps {
  finding: LocalFinding;
  /** Jumps the diff to this finding's inline card at its exact line. */
  onSelect: (finding: LocalFinding) => void;
}

/** A compact navigator row in the AI-review panel. The full card (accept/edit/
 *  dismiss + suggestion) lives inline in the diff at the finding's line; this
 *  row is the index that jumps there. Dismiss is here too for quick triage. */
export function AiFindingRow({ finding, onSelect }: AiFindingRowProps) {
  const filename = finding.path.split("/").pop() ?? finding.path;
  return (
    <div className="group flex items-start gap-1.5 rounded-md px-1.5 py-1 hover:bg-hover">
      <button
        type="button"
        onClick={() => onSelect(finding)}
        title={`Jump to ${finding.path}:${finding.line}`}
        className={clsx("min-w-0 flex-1 text-left", FOCUS_RING)}
      >
        <div className="flex items-center gap-1.5 text-2xs">
          <span className={clsx("h-1.5 w-1.5 shrink-0 rounded-full bg-current", SEVERITY_TEXT[finding.severity])} />
          <span className="min-w-0 truncate font-mono text-ink-faint">
            {filename}:{finding.line}
          </span>
          <SquareArrowOutUpRight size={9} className="shrink-0 text-ink-disabled opacity-0 group-hover:opacity-100" />
        </div>
        <p className="mt-0.5 line-clamp-2 font-sans text-2xs text-ink-muted">{finding.body}</p>
      </button>
      <button
        type="button"
        onClick={() => useReviewWorkspaceStore.getState().removeFinding(finding.id)}
        aria-label="Dismiss finding"
        title="Dismiss"
        className={clsx(
          "flex h-5 w-5 shrink-0 items-center justify-center rounded text-ink-faint opacity-0 hover:bg-hover hover:text-status-failed group-hover:opacity-100",
          FOCUS_RING,
        )}
      >
        <X size={11} />
      </button>
    </div>
  );
}
