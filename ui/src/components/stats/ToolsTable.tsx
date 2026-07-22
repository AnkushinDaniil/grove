import clsx from "clsx";
import { AlertTriangle, Wrench } from "lucide-react";
import { formatTokens } from "../../lib/usageFormat";
import { formatPercent } from "../../lib/statsFormat";
import type { StatsTool } from "../../gen/types";

interface ToolsTableProps {
  tools: StatsTool[];
}

const WARN_THRESHOLD = 0.05;
const CRITICAL_THRESHOLD = 0.15;

function errorRateClass(rate: number): string {
  if (rate >= CRITICAL_THRESHOLD) return "text-status-failed";
  if (rate >= WARN_THRESHOLD) return "text-status-starting";
  return "text-ink-muted";
}

/** Error rate is a status-shaped value (it means good/bad), so it wears
 *  status tokens with an icon, never a plain number -- same "collision
 *  rule" and AlertTriangle+status-failed treatment NodeView's action-error
 *  banner already uses. */
export function ToolsTable({ tools }: ToolsTableProps) {
  if (tools.length === 0) return null;
  const sorted = [...tools].sort((a, b) => b.calls - a.calls);

  return (
    <div>
      <h3 className="mb-1.5 flex items-center gap-1.5 font-sans text-2xs font-medium text-ink-faint">
        <Wrench size={12} />
        Tools
      </h3>
      <table className="w-full text-xs">
        <thead>
          <tr className="text-left text-2xs text-ink-faint">
            <th className="pb-1 font-normal">Tool</th>
            <th className="pb-1 text-right font-normal">Calls</th>
            <th className="pb-1 text-right font-normal">Error rate</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {sorted.map((t) => {
            const rate = t.calls > 0 ? t.errors / t.calls : 0;
            const flagged = rate >= WARN_THRESHOLD;
            return (
              <tr key={t.name}>
                <td className="py-1 text-ink">{t.name}</td>
                <td className="py-1 text-right text-ink-muted">{formatTokens(t.calls)}</td>
                <td className={clsx("py-1 text-right", errorRateClass(rate))}>
                  <span className="inline-flex w-full items-center justify-end gap-1" title={`${t.errors} of ${t.calls} calls errored`}>
                    {flagged && <AlertTriangle size={11} className="shrink-0" />}
                    {formatPercent(rate)}
                  </span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
