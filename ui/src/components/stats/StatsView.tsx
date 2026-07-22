import { useEffect, useState } from "react";
import clsx from "clsx";
import { BarChart3, RefreshCw } from "lucide-react";
import { useStatsStore } from "../../state/stats";
import { refreshStats, setStatsRange, setStatsScope, startStatsPolling, stopStatsPolling } from "../../state/statsPolling";
import { RangeSwitcher } from "./RangeSwitcher";
import { ScopePicker } from "./ScopePicker";
import { TokensSection } from "./TokensSection";
import { AgentsSection } from "./AgentsSection";
import { FlowSection } from "./FlowSection";
import { ToolsTable } from "./ToolsTable";
import { SkillsList } from "./SkillsList";
import { FeedbackLeaderboard } from "./FeedbackLeaderboard";
import { FeedbackTab } from "../feedback/FeedbackTab";
import { EmptyState } from "../common/EmptyState";
import { FOCUS_RING } from "../../lib/constants";

type ViewTab = "overview" | "feedback";

function SkeletonBlock({ className }: { className?: string }) {
  return <div className={clsx("animate-pulse rounded-lg bg-surface-2", className)} aria-hidden="true" />;
}

function hasAnyActivity(data: ReturnType<typeof useStatsStore.getState>["data"]): boolean {
  if (!data) return false;
  return (
    data.tokens.total.input > 0 ||
    data.tokens.total.cost_usd > 0 ||
    data.agents.sessions_active > 0 ||
    data.agents.sessions_by_day.some((d) => d.started > 0) ||
    data.flow.tasks_created > 0 ||
    data.tools.some((t) => t.calls > 0)
  );
}

/** The metrics dashboard (/stats). Polling is scoped to this view's mount
 *  lifecycle (unlike usage/reviews, which run app-wide for their nav
 *  badges) -- stats has no ambient badge to keep warm, so there's no reason
 *  to fetch while nobody's looking at it. Feedback lives as a second tab
 *  here rather than a separate top-level route, per the "keep nav lean"
 *  brief. */
export function StatsView() {
  const [tab, setTab] = useState<ViewTab>("overview");
  const range = useStatsStore((s) => s.range);
  const scope = useStatsStore((s) => s.scope);
  const data = useStatsStore((s) => s.data);
  const loading = useStatsStore((s) => s.loading);
  const loaded = useStatsStore((s) => s.loaded);
  const lastError = useStatsStore((s) => s.lastError);

  useEffect(() => {
    startStatsPolling();
    return () => stopStatsPolling();
  }, []);

  const openFeedbackCount = data ? data.feedback.reduce((sum, f) => sum + f.open, 0) : 0;

  return (
    <div className="flex h-full flex-col">
      <div className="shrink-0 space-y-2.5 border-b border-border px-5 py-3">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="flex items-center gap-1.5 font-sans text-sm font-medium text-ink">
            <BarChart3 size={15} className="text-ink-faint" />
            Stats
          </h1>
          <button
            type="button"
            onClick={() => refreshStats()}
            disabled={loading}
            aria-label="Refresh"
            title="Refresh"
            className={clsx(
              "flex h-7 w-7 items-center justify-center rounded-md text-ink-faint hover:bg-hover hover:text-ink disabled:opacity-40",
              FOCUS_RING,
            )}
          >
            <RefreshCw size={13} className={clsx(loading && "animate-spin")} />
          </button>
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <ScopePicker value={scope} onChange={setStatsScope} />
            <RangeSwitcher value={range} onChange={setStatsRange} />
          </div>
        </div>

        <div className="flex items-center gap-1 text-xs">
          <button
            type="button"
            onClick={() => setTab("overview")}
            className={clsx(
              "rounded-t-md border-b-2 px-3 py-1.5 font-medium transition-colors",
              tab === "overview" ? "border-accent text-ink" : "border-transparent text-ink-faint hover:text-ink",
              FOCUS_RING,
            )}
          >
            Overview
          </button>
          <button
            type="button"
            onClick={() => setTab("feedback")}
            className={clsx(
              "flex items-center gap-1.5 rounded-t-md border-b-2 px-3 py-1.5 font-medium transition-colors",
              tab === "feedback" ? "border-accent text-ink" : "border-transparent text-ink-faint hover:text-ink",
              FOCUS_RING,
            )}
          >
            Feedback
            {openFeedbackCount > 0 && <span className="text-2xs text-accent">{openFeedbackCount}</span>}
          </button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {tab === "feedback" ? (
          <FeedbackTab />
        ) : (
          <div className={clsx("space-y-6 px-5 py-4 transition-opacity", loading && loaded && "opacity-60")}>
            {lastError && (
              <div role="alert" className="rounded-md border border-status-failed/30 bg-status-failed/10 px-3 py-2 text-2xs text-status-failed">
                Couldn't load stats: {lastError}
              </div>
            )}

            {!loaded && (
              <div className="space-y-3">
                <div className="grid grid-cols-3 gap-2">
                  <SkeletonBlock className="h-16" />
                  <SkeletonBlock className="h-16" />
                  <SkeletonBlock className="h-16" />
                </div>
                <SkeletonBlock className="h-40" />
                <SkeletonBlock className="h-24" />
              </div>
            )}

            {loaded && data && !hasAnyActivity(data) && (
              <EmptyState
                icon={<BarChart3 size={28} strokeWidth={1.5} />}
                title="No activity yet"
                description="Stats fill in once agents start running in this scope."
              />
            )}

            {loaded && data && hasAnyActivity(data) && (
              <>
                <TokensSection tokens={data.tokens} models={data.models} />
                <AgentsSection agents={data.agents} />
                <FlowSection flow={data.flow} />
                <div className="grid gap-6 sm:grid-cols-2">
                  <ToolsTable tools={data.tools} />
                  <SkillsList skills={data.skills} />
                </div>
                <FeedbackLeaderboard items={data.feedback} onOpenFeedback={() => setTab("feedback")} />
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
