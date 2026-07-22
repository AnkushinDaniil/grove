import { useState } from "react";
import clsx from "clsx";
import { Plus, X } from "lucide-react";
import { apiClient } from "../../state/api";
import { useReviewsStore } from "../../state/reviews";
import { refreshReviews } from "../../state/reviewsPolling";
import { DirCombobox } from "../common/DirCombobox";
import { FOCUS_RING } from "../../lib/constants";

interface SourcesPanelProps {
  onClose: () => void;
}

/** Inline settings panel for the `review_dirs` watch list (GET/POST
 *  /reviews/sources). Reads/writes the shared reviews store directly so the
 *  repo list and empty-state in ReviewsView stay in sync with every
 *  mutation made here. */
export function SourcesPanel({ onClose }: SourcesPanelProps) {
  const dirs = useReviewsStore((s) => s.sourceDirs);
  const setSourceDirs = useReviewsStore((s) => s.setSourceDirs);
  const [addValue, setAddValue] = useState("");
  const [addError, setAddError] = useState<string | null>(null);
  const [removeError, setRemoveError] = useState<string | null>(null);
  // The dir currently being removed, or "+" while an add is in flight.
  const [busyDir, setBusyDir] = useState<string | null>(null);
  // Bumped after a successful add to remount DirCombobox: clears the input,
  // refocuses it, and refetches the home-directory listing for the next entry.
  const [resetKey, setResetKey] = useState(0);

  async function persist(next: string[]): Promise<string[]> {
    const res = await apiClient.setReviewSources(next);
    setSourceDirs(res.dirs);
    refreshReviews();
    return res.dirs;
  }

  async function removeDir(dir: string) {
    if (!dirs) return;
    setBusyDir(dir);
    setRemoveError(null);
    try {
      await persist(dirs.filter((d) => d !== dir));
    } catch (err) {
      setRemoveError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusyDir(null);
    }
  }

  async function addDir(raw: string) {
    const dir = raw.trim();
    if (!dirs || !dir || dirs.includes(dir)) return;
    setBusyDir("+");
    setAddError(null);
    try {
      await persist([...dirs, dir]);
      setAddValue("");
      setResetKey((k) => k + 1);
    } catch (err) {
      setAddError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusyDir(null);
    }
  }

  return (
    <div className="space-y-2 border-b border-border bg-surface-2 px-5 py-3">
      <div className="flex items-center justify-between">
        <h2 className="text-2xs font-medium tracking-wide text-ink-faint uppercase">Watched repositories</h2>
        <button
          type="button"
          onClick={onClose}
          className={clsx("rounded px-1.5 py-0.5 text-2xs text-ink-faint hover:bg-hover hover:text-ink", FOCUS_RING)}
        >
          Done
        </button>
      </div>

      {dirs === null && <p className="text-2xs text-ink-faint">Loading sources…</p>}

      {dirs !== null && (
        <ul className="space-y-1">
          {dirs.length === 0 && <li className="text-2xs text-ink-faint">No directories watched yet.</li>}
          {dirs.map((dir) => (
            <li key={dir} className="flex items-center gap-2 font-mono text-2xs text-ink-muted">
              <span className="min-w-0 flex-1 truncate">{dir}</span>
              <button
                type="button"
                onClick={() => void removeDir(dir)}
                disabled={busyDir === dir}
                aria-label={`Stop watching ${dir}`}
                title={`Stop watching ${dir}`}
                className={clsx(
                  "flex h-6 w-6 shrink-0 items-center justify-center rounded text-ink-faint hover:bg-hover hover:text-status-failed disabled:opacity-40",
                  FOCUS_RING,
                )}
              >
                <X size={12} />
              </button>
            </li>
          ))}
        </ul>
      )}
      {removeError && <p className="text-2xs break-words text-status-failed">{removeError}</p>}

      <div className="flex items-start gap-2 pt-1">
        <DirCombobox
          key={resetKey}
          idPrefix="review-source"
          value={addValue}
          onChange={(v) => {
            setAddValue(v);
            setAddError(null);
          }}
          onCommit={(v) => void addDir(v)}
          disabled={busyDir === "+" || dirs === null}
          placeholder="/absolute/path/to/repo"
          className="flex-1"
        />
        <button
          type="button"
          onClick={() => void addDir(addValue)}
          disabled={busyDir === "+" || dirs === null || !addValue.trim()}
          className={clsx(
            "flex min-h-9 shrink-0 items-center gap-1 rounded-md bg-accent px-2.5 text-xs font-medium text-accent-ink hover:bg-accent-strong disabled:opacity-40",
            FOCUS_RING,
          )}
        >
          <Plus size={13} />
          Add
        </button>
      </div>
      {addError && <p className="text-2xs break-words text-status-failed">{addError}</p>}
    </div>
  );
}
