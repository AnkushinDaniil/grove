import { GitBranch, Hourglass } from "lucide-react";
import { StatTile } from "./charts/StatTile";
import { formatTokens } from "../../lib/usageFormat";
import { formatHours, formatMinutes } from "../../lib/statsFormat";
import type { StatsFlow } from "../../gen/types";

interface FlowSectionProps {
  flow: StatsFlow;
}

export function FlowSection({ flow }: FlowSectionProps) {
  return (
    <section aria-labelledby="flow-heading" className="space-y-3">
      <h2 id="flow-heading" className="flex items-center gap-1.5 font-sans text-xs font-semibold text-ink">
        <GitBranch size={13} className="text-ink-faint" />
        Flow
      </h2>

      <div className="grid grid-cols-3 gap-2">
        <StatTile label="Tasks created" value={formatTokens(flow.tasks_created)} />
        <StatTile label="Tasks done" value={formatTokens(flow.tasks_done)} />
        <StatTile label="Tasks failed" value={formatTokens(flow.tasks_failed)} />
      </div>

      {/* The one standout metric in this section -- how long the workspace
          waited on the user, not the agent. Own card, accent-tinted like
          NodeView's attention pill, so it reads as "look here" at a glance. */}
      <div className="rounded-lg border border-accent/30 bg-accent-soft p-3">
        <div className="flex items-center gap-1.5 font-sans text-2xs text-accent">
          <Hourglass size={12} className="shrink-0" />
          Attention wait -- how long tasks sat waiting on you
        </div>
        <div className="mt-1.5 flex items-baseline gap-6">
          <div>
            <div className="text-2xl font-semibold text-ink md:text-3xl">{formatMinutes(flow.attention_wait_p50_minutes)}</div>
            <div className="text-2xs text-ink-faint">p50</div>
          </div>
          <div>
            <div className="text-2xl font-semibold text-ink md:text-3xl">{formatMinutes(flow.attention_wait_p95_minutes)}</div>
            <div className="text-2xs text-ink-faint">p95</div>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-3 gap-2">
        <StatTile label="Median task time" value={formatHours(flow.median_task_hours)} />
        <StatTile label="PRs opened" value={formatTokens(flow.prs_opened)} />
        <StatTile label="PRs merged" value={formatTokens(flow.prs_merged)} />
      </div>
    </section>
  );
}
