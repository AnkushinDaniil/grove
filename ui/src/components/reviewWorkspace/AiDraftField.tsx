import { useState } from "react";
import clsx from "clsx";
import { Loader2, Sparkles } from "lucide-react";
import { FOCUS_RING } from "../../lib/constants";

interface AiDraftFieldProps {
  value: string;
  onChange: (value: string) => void;
  /** Accessible name -- placeholder text alone isn't reliably announced as
   *  a persistent label, and this field never has a visible <label> (its
   *  surrounding context, e.g. a card anchored on a diff line, carries that
   *  visually instead). */
  ariaLabel: string;
  placeholder?: string;
  rows?: number;
  autoFocus?: boolean;
  disabled?: boolean;
  /** Caller builds and fires the actual aiDraft request -- it knows the
   *  kind/path/line/thread_id context; this component only owns the
   *  textarea, the busy spinner, and the inline error around calling it. */
  onRequestDraft: () => Promise<string>;
}

/** Textarea + "Draft with AI" button shared by line comments, thread
 *  replies, and the overall review summary. Replaces the textarea's
 *  content with the drafted suggestion; the human always reviews/edits it
 *  from there before it becomes a draft, a reply, or the submitted body. */
export function AiDraftField({
  value,
  onChange,
  ariaLabel,
  placeholder,
  rows = 3,
  autoFocus,
  disabled,
  onRequestDraft,
}: AiDraftFieldProps) {
  const [drafting, setDrafting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function requestDraft() {
    setDrafting(true);
    setError(null);
    try {
      const text = await onRequestDraft();
      onChange(text);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setDrafting(false);
    }
  }

  return (
    <div>
      <textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label={ariaLabel}
        placeholder={placeholder}
        rows={rows}
        autoFocus={autoFocus}
        disabled={disabled}
        className={clsx(
          "w-full resize-none rounded-md border border-border bg-canvas px-2 py-1.5 font-sans text-xs text-ink placeholder:text-ink-faint disabled:opacity-50",
          FOCUS_RING,
        )}
      />
      <div className="mt-1.5 flex items-center gap-2">
        <button
          type="button"
          onClick={() => void requestDraft()}
          disabled={disabled || drafting}
          className={clsx(
            "flex items-center gap-1.5 rounded-md border border-accent/30 bg-accent-soft px-2 py-1 text-2xs font-medium text-accent hover:bg-accent-soft/70 disabled:opacity-50",
            FOCUS_RING,
          )}
        >
          {drafting ? <Loader2 size={11} className="animate-spin" /> : <Sparkles size={11} />}
          {drafting ? "Drafting…" : "Draft with AI"}
        </button>
        {error && <span className="min-w-0 flex-1 truncate text-2xs text-status-failed">{error}</span>}
      </div>
    </div>
  );
}
