import clsx from "clsx";
import { FOCUS_RING } from "../../lib/constants";

interface SegmentedControlProps<T extends string> {
  options: { value: T; label: string }[];
  value: T;
  onChange: (value: T) => void;
  className?: string;
}

/** Small pill-group switcher -- same visual language as RepoSwitcher's repo
 *  tabs, generalized so the stats range switcher and the feedback status
 *  filter don't each reinvent it. */
export function SegmentedControl<T extends string>({ options, value, onChange, className }: SegmentedControlProps<T>) {
  return (
    <div className={clsx("flex items-center gap-1", className)} role="group">
      {options.map((opt) => (
        <button
          key={opt.value}
          type="button"
          onClick={() => onChange(opt.value)}
          aria-pressed={opt.value === value}
          className={clsx(
            "rounded-md border px-2 py-1 text-2xs font-medium",
            opt.value === value
              ? "border-accent/40 bg-accent-soft text-accent"
              : "border-border-strong text-ink-muted hover:bg-hover hover:text-ink",
            FOCUS_RING,
          )}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}
