import clsx from "clsx";
import type { LucideIcon } from "lucide-react";

interface StatTileProps {
  label: string;
  value: string;
  icon?: LucideIcon;
  /** Secondary caption under the value -- a breakdown or unit clarification. */
  hint?: string;
  /** Marks the one standout tile in a section (e.g. flow's attention-wait
   *  bottleneck metric) with the same accent treatment NodeView's attention
   *  pill uses for "look here" -- never used for more than one tile at a
   *  time, or it stops meaning anything. */
  emphasize?: boolean;
  className?: string;
}

/** Stat-tile contract per the dataviz skill: label (sentence case, no
 *  trailing colon) + value (semibold, auto-compact, proportional figures --
 *  grove's body font is already monospace/fixed-width throughout, so there's
 *  no separate "tabular" variant to avoid here; the value just inherits the
 *  app's ambient font like every other number in the UI) + an optional hint. */
export function StatTile({ label, value, icon: Icon, hint, emphasize, className }: StatTileProps) {
  return (
    <div
      className={clsx(
        "flex min-w-0 flex-col gap-1 rounded-lg border p-3",
        emphasize ? "border-accent/30 bg-accent-soft" : "border-border bg-surface-2",
        className,
      )}
    >
      <div className={clsx("flex items-center gap-1.5 text-2xs font-sans", emphasize ? "text-accent" : "text-ink-faint")}>
        {Icon && <Icon size={12} className="shrink-0" />}
        <span className="truncate">{label}</span>
      </div>
      <div className="truncate text-xl font-semibold text-ink md:text-2xl">{value}</div>
      {hint && <div className="truncate text-2xs text-ink-faint">{hint}</div>}
    </div>
  );
}
