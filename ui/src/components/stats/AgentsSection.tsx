import { Bot } from "lucide-react";
import { StatTile } from "./charts/StatTile";
import { SessionsByDayBar } from "./charts/SessionsByDayBar";
import { MiniBarList } from "./charts/MiniBarList";
import { formatTokens } from "../../lib/usageFormat";
import { formatMinutes } from "../../lib/statsFormat";
import type { StatsAgents } from "../../gen/types";

interface AgentsSectionProps {
  agents: StatsAgents;
}

export function AgentsSection({ agents }: AgentsSectionProps) {
  return (
    <section aria-labelledby="agents-heading" className="space-y-3">
      <h2 id="agents-heading" className="flex items-center gap-1.5 font-sans text-xs font-semibold text-ink">
        <Bot size={13} className="text-ink-faint" />
        Agents
      </h2>

      <div className="grid grid-cols-2 gap-2">
        <StatTile label="Active sessions" value={formatTokens(agents.sessions_active)} />
        <StatTile label="Avg session length" value={formatMinutes(agents.avg_session_minutes)} />
      </div>

      <div className="rounded-lg border border-border bg-surface-2 p-3">
        <h3 className="mb-2 font-sans text-2xs font-medium text-ink-faint">Sessions by day</h3>
        <SessionsByDayBar days={agents.sessions_by_day} />
      </div>

      <div>
        <h3 className="mb-1.5 font-sans text-2xs font-medium text-ink-faint">By driver</h3>
        <MiniBarList
          items={agents.by_driver.map((d) => ({ key: d.driver, label: d.driver, value: d.count }))}
          formatValue={formatTokens}
        />
      </div>
    </section>
  );
}
