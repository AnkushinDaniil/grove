import { ArrowDownToLine, ArrowUpFromLine, DollarSign, Zap } from "lucide-react";
import { StatTile } from "./charts/StatTile";
import { TrendChart } from "./charts/TrendChart";
import { MiniBarList } from "./charts/MiniBarList";
import { formatCost, formatTokens } from "../../lib/usageFormat";
import type { StatsModel, StatsTokens } from "../../gen/types";

interface TokensSectionProps {
  tokens: StatsTokens;
  models: StatsModel[];
}

export function TokensSection({ tokens, models }: TokensSectionProps) {
  const trendPoints = tokens.by_day.map((d) => ({ key: d.day, value: d.cost_usd }));

  return (
    <section aria-labelledby="tokens-heading" className="space-y-3">
      <h2 id="tokens-heading" className="flex items-center gap-1.5 font-sans text-xs font-semibold text-ink">
        <Zap size={13} className="text-ink-faint" />
        Tokens
      </h2>

      <div className="grid grid-cols-3 gap-2">
        <StatTile label="Total cost" value={formatCost(tokens.total.cost_usd)} icon={DollarSign} />
        <StatTile label="Input tokens" value={formatTokens(tokens.total.input)} icon={ArrowDownToLine} />
        <StatTile label="Output tokens" value={formatTokens(tokens.total.output)} icon={ArrowUpFromLine} />
      </div>

      <div className="rounded-lg border border-border bg-surface-2 p-3">
        <TrendChart points={trendPoints} formatValue={formatCost} ariaLabel="Cost by day" />
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <div>
          <h3 className="mb-1.5 font-sans text-2xs font-medium text-ink-faint">By driver</h3>
          <MiniBarList
            items={tokens.by_driver.map((d) => ({ key: d.driver, label: d.driver, value: d.cost_usd }))}
            formatValue={formatCost}
          />
        </div>
        <div>
          <h3 className="mb-1.5 font-sans text-2xs font-medium text-ink-faint">By model</h3>
          <MiniBarList
            items={models.map((m) => ({ key: m.model, label: m.model, value: m.cost_usd }))}
            formatValue={formatCost}
          />
        </div>
        <div className="sm:col-span-2 lg:col-span-1">
          <h3 className="mb-1.5 font-sans text-2xs font-medium text-ink-faint">Top nodes by cost</h3>
          <MiniBarList
            items={tokens.top_nodes.map((n) => ({ key: n.node_id, label: n.title, value: n.cost_usd, to: `/n/${n.node_id}` }))}
            formatValue={formatCost}
          />
        </div>
      </div>
    </section>
  );
}
