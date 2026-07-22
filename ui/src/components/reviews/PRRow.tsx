import { useState } from "react";
import clsx from "clsx";
import { useNavigate } from "react-router";
import { AlertTriangle, ExternalLink, GitPullRequestArrow } from "lucide-react";
import { apiClient } from "../../state/api";
import { FOCUS_RING } from "../../lib/constants";
import { Pill } from "../common/Pill";
import { RelativeTime } from "../common/RelativeTime";
import { ChecksPill } from "./ChecksPill";
import type { PR } from "../../gen/types";

interface PRRowProps {
  pr: PR;
  dir: string;
  repoName: string;
}

/** One PR in a bucket: identity + signals (checks, draft, diffstat) on the
 *  left, actions on the right. Dense single-row layout to match the
 *  control-room rail/tab idiom used throughout the tree and inbox. */
export function PRRow({ pr, dir, repoName }: PRRowProps) {
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function reviewInGrove() {
    setBusy(true);
    setError(null);
    try {
      const node = await apiClient.startReview(dir, pr.number, `Review ${repoName}#${pr.number}: ${pr.title}`);
      navigate(`/n/${node.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1 px-5 py-2 text-xs hover:bg-hover">
      <span className="shrink-0 font-mono text-ink-faint">#{pr.number}</span>
      <span className="min-w-0 flex-1 basis-48 truncate text-ink" title={pr.title}>
        {pr.title}
      </span>
      {pr.is_draft && <Pill tone="muted">Draft</Pill>}
      <ChecksPill checks={pr.checks} />
      <span className="shrink-0 text-2xs text-ink-faint" title={`Author: ${pr.author}`}>
        {pr.author}
      </span>
      <RelativeTime iso={pr.updated_at} className="shrink-0 text-2xs text-ink-faint" />
      <span className="shrink-0 font-mono text-2xs" title={`+${pr.additions} / -${pr.deletions}`}>
        <span className="text-diff-add">+{pr.additions}</span> <span className="text-status-failed">-{pr.deletions}</span>
      </span>

      <div className="ml-auto flex shrink-0 items-center gap-1.5">
        <button
          type="button"
          onClick={() => void reviewInGrove()}
          disabled={busy}
          className={clsx(
            "flex min-h-9 items-center gap-1 rounded-md bg-accent px-2.5 py-1 text-2xs font-medium text-accent-ink hover:bg-accent-strong disabled:opacity-40",
            FOCUS_RING,
          )}
        >
          <GitPullRequestArrow size={12} />
          Review in grove
        </button>
        <a
          href={pr.url}
          target="_blank"
          rel="noreferrer"
          className={clsx(
            "flex min-h-9 items-center gap-1 rounded-md border border-border-strong px-2.5 py-1 text-2xs text-ink-muted hover:bg-hover hover:text-ink",
            FOCUS_RING,
          )}
        >
          <ExternalLink size={12} />
          Open
        </a>
      </div>

      {error && (
        <div className="flex basis-full items-center gap-1.5 text-2xs break-words text-status-failed">
          <AlertTriangle size={11} className="shrink-0" />
          {error}
          <button
            type="button"
            onClick={() => setError(null)}
            className="ml-1 shrink-0 text-ink-faint hover:text-ink"
          >
            dismiss
          </button>
        </div>
      )}
    </div>
  );
}
