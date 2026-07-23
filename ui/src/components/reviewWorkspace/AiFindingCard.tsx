import { useState } from "react";
import clsx from "clsx";
import { Check, Loader2, Pencil, X } from "lucide-react";
import { apiClient } from "../../state/api";
import { useReviewWorkspaceStore } from "../../state/reviewWorkspace";
import type { LocalFinding } from "../../state/reviewWorkspace";
import { joinSuggestion } from "../../lib/suggestion";
import { FOCUS_RING } from "../../lib/constants";
import type { AiFindingSeverity } from "../../gen/types";

const SEVERITY: Record<AiFindingSeverity, { label: string; text: string }> = {
  issue: { label: "Issue", text: "text-status-failed" },
  suggestion: { label: "Suggestion", text: "text-accent" },
  nit: { label: "Nit", text: "text-ink-faint" },
};

interface AiFindingCardProps {
  finding: LocalFinding;
  dir: string;
  pr: number;
  /** When true the card is rendered inline in the diff at its anchor line, so
   *  it omits the clickable path:line location (the surrounding diff already
   *  shows where). */
  inline?: boolean;
}

/** One AI-review proposal, rendered inline in the diff at its exact anchor
 *  line: severity, the comment, and an optional code suggestion. Accept turns
 *  it into a pending draft (the suggestion becoming a committable GitHub
 *  ```suggestion block); Dismiss drops it. Edit tweaks the text first. */
export function AiFindingCard({ finding, dir, pr, inline }: AiFindingCardProps) {
  const [editing, setEditing] = useState(false);
  const [body, setBody] = useState(finding.body);
  const [suggestion, setSuggestion] = useState(finding.suggestion);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const sev = SEVERITY[finding.severity] ?? SEVERITY.suggestion;

  async function accept() {
    setBusy(true);
    setError(null);
    try {
      const draft = await apiClient.addReviewDraft({
        dir,
        pr,
        path: finding.path,
        line: finding.line,
        side: finding.side,
        body: joinSuggestion(body, suggestion),
      });
      const store = useReviewWorkspaceStore.getState();
      store.addDraftLocal(draft);
      store.removeFinding(finding.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }

  function dismiss() {
    useReviewWorkspaceStore.getState().removeFinding(finding.id);
  }

  return (
    <div className="rounded-md border border-border bg-surface px-2.5 py-2">
      <div className="flex items-center gap-1.5 text-2xs">
        <span className={clsx("inline-flex items-center gap-1 font-medium", sev.text)}>
          <span className="h-1.5 w-1.5 rounded-full bg-current" />
          {sev.label}
        </span>
        {!inline && (
          <span className="min-w-0 truncate font-mono text-ink-faint" title={finding.path}>
            {finding.path}:{finding.line}
          </span>
        )}
      </div>

      {editing ? (
        <textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          aria-label="Finding comment"
          rows={3}
          className={clsx(
            "mt-1.5 w-full resize-none rounded border border-border bg-canvas px-2 py-1 font-sans text-xs text-ink",
            FOCUS_RING,
          )}
        />
      ) : (
        <p className="mt-1 whitespace-pre-wrap font-sans text-xs text-ink-muted">{body}</p>
      )}

      {editing ? (
        <textarea
          value={suggestion}
          onChange={(e) => setSuggestion(e.target.value)}
          aria-label="Suggested replacement"
          placeholder="Suggested replacement line (optional)"
          rows={2}
          className={clsx(
            "mt-1.5 w-full resize-none rounded border border-diff-add/40 bg-canvas px-2 py-1 font-mono text-2xs text-ink placeholder:text-ink-faint",
            FOCUS_RING,
          )}
        />
      ) : (
        suggestion.trim() !== "" && (
          <div className="mt-1.5 overflow-x-auto rounded border border-diff-add/30 bg-diff-add/10">
            <div className="border-b border-diff-add/20 px-2 py-0.5 text-2xs font-medium text-diff-add">Suggested change</div>
            <pre className="px-2 py-1 font-mono text-2xs whitespace-pre text-ink">{suggestion}</pre>
          </div>
        )
      )}

      <div className="mt-2 flex items-center gap-1.5">
        <button
          type="button"
          onClick={() => void accept()}
          disabled={busy}
          className={clsx(
            "flex items-center gap-1 rounded border border-accent/30 bg-accent-soft px-2 py-1 text-2xs font-medium text-accent hover:bg-accent-soft/70 disabled:opacity-50",
            FOCUS_RING,
          )}
        >
          {busy ? <Loader2 size={11} className="animate-spin" /> : <Check size={11} />}
          Accept
        </button>
        <button
          type="button"
          onClick={() => setEditing((v) => !v)}
          disabled={busy}
          aria-pressed={editing}
          title="Edit before accepting"
          className={clsx(
            "flex items-center gap-1 rounded px-2 py-1 text-2xs text-ink-faint hover:bg-hover hover:text-ink disabled:opacity-50",
            editing && "text-ink",
            FOCUS_RING,
          )}
        >
          <Pencil size={11} />
          Edit
        </button>
        <button
          type="button"
          onClick={dismiss}
          disabled={busy}
          title="Dismiss"
          className={clsx(
            "ml-auto flex items-center gap-1 rounded px-2 py-1 text-2xs text-ink-faint hover:bg-hover hover:text-status-failed disabled:opacity-50",
            FOCUS_RING,
          )}
        >
          <X size={11} />
          Dismiss
        </button>
      </div>
      {error && <p className="mt-1 text-2xs break-words text-status-failed">{error}</p>}
    </div>
  );
}
