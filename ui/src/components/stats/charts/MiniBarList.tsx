import clsx from "clsx";
import { Link } from "react-router";
import { VIZ_FILL } from "../../../lib/vizColor";

export interface MiniBarItem {
  key: string;
  label: string;
  value: number;
  /** Node route to link to (top nodes by cost) -- plain text otherwise. */
  to?: string;
}

interface MiniBarListProps {
  items: MiniBarItem[];
  formatValue: (v: number) => string;
  className?: string;
}

/**
 * Horizontal single-hue magnitude bars with direct labels -- by_driver,
 * by_model, top_nodes, skills invocation counts. This is a plain
 * magnitude-by-category comparison (one number per row, not multiple series
 * sharing a row), so per the dataviz skill's form guidance it takes ONE
 * sequential hue rather than a categorical palette: coloring each row a
 * different hue would spend the identity channel re-encoding what the label
 * already shows. Same filled-pill treatment as UsageMeter's utilization bar
 * (track = surface-2, fill = rounded pill), just swapping the fill hue for
 * "this is a magnitude, not a status or a profile" (see lib/vizColor.ts).
 * Values are direct-labeled at the row's end, so nothing here is
 * hover-only -- no separate table view needed for a list this short.
 */
export function MiniBarList({ items, formatValue, className }: MiniBarListProps) {
  if (items.length === 0) return null;
  const max = Math.max(...items.map((i) => i.value), 1);

  return (
    <ul className={clsx("space-y-1.5", className)}>
      {items.map((item) => {
        const pct = Math.max(2, (item.value / max) * 100);
        const label = (
          <span className="min-w-0 flex-1 truncate text-ink" title={item.label}>
            {item.label}
          </span>
        );
        return (
          <li key={item.key} className="flex items-center gap-2 text-xs">
            {item.to ? (
              <Link to={item.to} className="flex min-w-0 flex-1 items-center hover:text-accent hover:underline">
                {label}
              </Link>
            ) : (
              label
            )}
            <span className="h-1.5 w-16 shrink-0 overflow-hidden rounded-full bg-surface-2 sm:w-24">
              <span className="block h-full rounded-full" style={{ width: `${pct}%`, backgroundColor: VIZ_FILL }} />
            </span>
            <span className="w-14 shrink-0 text-right text-ink-muted">{formatValue(item.value)}</span>
          </li>
        );
      })}
    </ul>
  );
}
