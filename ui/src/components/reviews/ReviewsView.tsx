import { useEffect, useState } from "react";
import clsx from "clsx";
import { GitPullRequest, RefreshCw, Settings } from "lucide-react";
import { useShallow } from "zustand/react/shallow";
import { selectVisibleErrors, useReviewsStore } from "../../state/reviews";
import { refreshReviews, refreshReviewSources } from "../../state/reviewsPolling";
import { RepoSection } from "./RepoSection";
import { SourcesPanel } from "./SourcesPanel";
import { EmptyState } from "../common/EmptyState";
import { FOCUS_RING } from "../../lib/constants";

/** Review Radar: a standing queue of open PRs across watched repositories.
 *  Polling itself is bootstrapped once app-wide from AuthGate (see
 *  state/reviewsPolling.ts) so the nav badge stays live even off this route;
 *  this view is a consumer of that store plus a "refresh on arrival" nicety
 *  and the manual refresh / manage-sources affordances. */
export function ReviewsView() {
  const repos = useReviewsStore((s) => s.repos);
  const loading = useReviewsStore((s) => s.loading);
  const loaded = useReviewsStore((s) => s.loaded);
  const lastError = useReviewsStore((s) => s.lastError);
  const sourceDirs = useReviewsStore((s) => s.sourceDirs);
  // selectVisibleErrors filters to a new array each call; see InboxView's
  // identical use of useShallow for why that needs guarding here too.
  const errors = useReviewsStore(useShallow(selectVisibleErrors));
  const dismissError = useReviewsStore((s) => s.dismissError);
  const [sourcesOpen, setSourcesOpen] = useState(false);

  useEffect(() => {
    refreshReviews();
    refreshReviewSources();
  }, []);

  const showEmptySources = sourceDirs !== null && sourceDirs.length === 0;

  return (
    <div className="flex h-full flex-col">
      <div className="shrink-0 border-b border-border px-5 py-3">
        <div className="flex items-center gap-1.5">
          <h1 className="flex-1 font-sans text-sm font-medium text-ink">Reviews</h1>
          <button
            type="button"
            onClick={() => refreshReviews()}
            disabled={loading}
            aria-label="Refresh"
            title="Refresh"
            className={clsx(
              "flex h-7 w-7 items-center justify-center rounded-md text-ink-faint hover:bg-hover hover:text-ink disabled:opacity-40",
              FOCUS_RING,
            )}
          >
            <RefreshCw size={14} className={clsx(loading && "animate-spin")} />
          </button>
          <button
            type="button"
            onClick={() => setSourcesOpen((v) => !v)}
            aria-label="Manage sources"
            aria-expanded={sourcesOpen}
            title="Manage sources"
            className={clsx(
              "flex h-7 w-7 items-center justify-center rounded-md text-ink-faint hover:bg-hover hover:text-ink",
              sourcesOpen && "bg-hover text-ink",
              FOCUS_RING,
            )}
          >
            <Settings size={14} />
          </button>
        </div>
        <p className="mt-0.5 font-sans text-2xs text-ink-faint">
          A standing queue of open pull requests across your watched repositories.
        </p>
      </div>

      {sourcesOpen && <SourcesPanel onClose={() => setSourcesOpen(false)} />}

      <div className="min-h-0 flex-1 overflow-y-auto">
        {lastError && (
          <div role="alert" className="border-b border-status-failed/30 bg-status-failed/10 px-5 py-2 text-2xs text-status-failed">
            Couldn't load reviews: {lastError}
          </div>
        )}

        {errors.map((message) => (
          <div
            key={message}
            role="alert"
            className="flex items-start gap-2 border-b border-status-failed/30 bg-status-failed/10 px-5 py-2 text-2xs text-status-failed"
          >
            <span className="min-w-0 flex-1 break-words">{message}</span>
            <button
              type="button"
              onClick={() => dismissError(message)}
              className="shrink-0 text-ink-faint hover:text-ink"
            >
              dismiss
            </button>
          </div>
        ))}

        {!loaded && <div className="px-5 py-4 text-xs text-ink-faint">Loading reviews…</div>}

        {showEmptySources && (
          <EmptyState
            icon={<GitPullRequest size={28} strokeWidth={1.5} />}
            title="No repositories watched yet"
            description="Review Radar watches your repos for open pull requests that need you. Add a directory to get started."
            action={
              <button
                type="button"
                onClick={() => setSourcesOpen(true)}
                className={clsx(
                  "rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-accent-ink hover:bg-accent-strong",
                  FOCUS_RING,
                )}
              >
                Add a directory
              </button>
            }
          />
        )}

        {repos.map((repo) => (
          <RepoSection key={repo.dir} repo={repo} />
        ))}
      </div>
    </div>
  );
}
