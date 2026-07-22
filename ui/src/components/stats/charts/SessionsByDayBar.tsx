import clsx from "clsx";
import type { AgentsByDay } from "../../../gen/types";

interface SessionsByDayBarProps {
  days: AgentsByDay[];
  className?: string;
}

const CHART_H = 72;

function formatDayLabel(day: string): string {
  const d = new Date(`${day}T00:00:00`);
  if (Number.isNaN(d.getTime())) return day;
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function LegendDot({ dotClassName, label }: { dotClassName: string; label: string }) {
  return (
    <span className="flex items-center gap-1">
      <span className={clsx("h-1.5 w-1.5 shrink-0 rounded-full", dotClassName)} />
      {label}
    </span>
  );
}

/**
 * Per-day stacked columns of session outcomes. done/failed/"open" mean
 * exactly what they mean on a node's StatusChip, so this reuses grove's
 * fixed status hues rather than a generic categorical palette -- the
 * dataviz skill's collision rule: a series that IS a status wears status
 * tokens, never an arbitrary "series N" color. Three series in one chart
 * means a legend is mandatory (never color-only identity); no gaps between
 * a day's own segments, matching RollupMiniBar's precedent for stacked bars
 * this compact -- a 2px gap on a 3px-tall segment would read as broken, not
 * separated. Each column carries its own tooltip since there's no room for
 * per-day axis labels at this density.
 */
export function SessionsByDayBar({ days, className }: SessionsByDayBarProps) {
  if (days.length === 0) return null;
  const maxStarted = Math.max(...days.map((d) => d.started), 1);

  return (
    <div className={className}>
      <div className="flex items-end gap-[3px]" style={{ height: CHART_H }}>
        {days.map((d) => {
          const total = Math.max(d.started, d.done + d.failed, 1);
          const colH = Math.max(3, (total / maxStarted) * CHART_H);
          const done = Math.min(d.done, total);
          const failed = Math.min(d.failed, total - done);
          const doneH = (done / total) * colH;
          const failedH = (failed / total) * colH;
          const openH = Math.max(0, colH - doneH - failedH);
          return (
            <div key={d.day} className="flex min-w-0 flex-1 flex-col justify-end" style={{ height: CHART_H }}>
              <div
                className="flex flex-col-reverse overflow-hidden rounded-t"
                style={{ height: colH }}
                title={`${formatDayLabel(d.day)}: ${d.started} started, ${d.done} done, ${d.failed} failed`}
              >
                <span className="w-full shrink-0 bg-status-done" style={{ height: doneH }} />
                <span className="w-full shrink-0 bg-status-failed" style={{ height: failedH }} />
                <span className="w-full shrink-0 bg-status-starting" style={{ height: openH }} />
              </div>
            </div>
          );
        })}
      </div>
      <div className="mt-1.5 flex items-center gap-3 text-2xs text-ink-faint">
        <LegendDot dotClassName="bg-status-done" label="Done" />
        <LegendDot dotClassName="bg-status-failed" label="Failed" />
        <LegendDot dotClassName="bg-status-starting" label="Open" />
      </div>
    </div>
  );
}
