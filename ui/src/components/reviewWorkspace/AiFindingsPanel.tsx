import clsx from "clsx";
import { Loader2, Sparkles } from "lucide-react";
import { runAIReview, useReviewWorkspaceStore } from "../../state/reviewWorkspace";
import type { LocalFinding } from "../../state/reviewWorkspace";
import { FOCUS_RING } from "../../lib/constants";
import { AiFindingRow } from "./AiFindingRow";

interface AiFindingsPanelProps {
  dir: string;
  pr: number;
  /** Jumps the diff to a finding's inline card at its exact line. */
  onSelect: (finding: LocalFinding) => void;
}

/** The AI-review side panel: a "Review with AI" pass over the whole PR diff
 *  produces line-anchored findings (proposed comments + suggestions) listed
 *  here. Each is accepted into the drafts batch or dismissed; clicking one
 *  jumps the diff to its file. Nothing posts without the reviewer. */
export function AiFindingsPanel({ dir, pr, onSelect }: AiFindingsPanelProps) {
  const findings = useReviewWorkspaceStore((s) => s.aiFindings);
  const reviewing = useReviewWorkspaceStore((s) => s.aiReviewing);
  const error = useReviewWorkspaceStore((s) => s.aiReviewError);
  const ran = useReviewWorkspaceStore((s) => s.aiReviewRan);

  return (
    <div className="flex min-h-0 flex-1 flex-col border-b border-border">
      <div className="flex items-center gap-2 px-3 py-2">
        <Sparkles size={13} className="shrink-0 text-accent" />
        <span className="text-2xs font-semibold tracking-wide text-ink-muted uppercase">AI review</span>
        {findings.length > 0 && (
          <span className="rounded-full bg-accent-soft px-1.5 text-2xs font-medium text-accent">{findings.length}</span>
        )}
        <button
          type="button"
          onClick={() => void runAIReview(dir, pr)}
          disabled={reviewing}
          className={clsx(
            "ml-auto flex items-center gap-1.5 rounded-md border border-accent/30 bg-accent-soft px-2 py-1 text-2xs font-medium text-accent hover:bg-accent-soft/70 disabled:opacity-50",
            FOCUS_RING,
          )}
        >
          {reviewing ? <Loader2 size={11} className="animate-spin" /> : <Sparkles size={11} />}
          {reviewing ? "Reviewing…" : ran ? "Re-run" : "Review with AI"}
        </button>
      </div>

      <div className="min-h-0 flex-1 space-y-1.5 overflow-y-auto px-3 pb-3">
        {error && <p className="rounded border border-status-failed/30 bg-status-failed/10 px-2 py-1.5 text-2xs break-words text-status-failed">{error}</p>}

        {reviewing && (
          <p className="px-1 py-2 text-2xs text-ink-faint">
            Reading the whole diff and drafting findings — this usually takes a minute or two.
          </p>
        )}

        {findings.map((f) => (
          <AiFindingRow key={f.id} finding={f} onSelect={onSelect} />
        ))}

        {!reviewing && findings.length === 0 && !error && (
          <p className="px-1 py-2 text-2xs text-ink-faint">
            {ran
              ? "No findings — nothing worth flagging."
              : "Run an AI pass for line-by-line comments and code suggestions over the whole diff."}
          </p>
        )}
      </div>
    </div>
  );
}
