import clsx from "clsx";
import { FOCUS_RING } from "../../lib/constants";

interface SwitchProps {
  checked: boolean;
  onChange: () => void;
  disabled?: boolean;
  /** Accessible name -- this is a bare toggle with no visible text of its
   *  own, so callers always render a label alongside it and pass the same
   *  copy here. */
  label: string;
  className?: string;
}

/** Small pill toggle, role="switch" -- the settings equivalent of
 *  SegmentedControl. Stateless/controlled so any future boolean setting can
 *  reuse it. */
export function Switch({ checked, onChange, disabled, label, className }: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={onChange}
      className={clsx(
        "relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border transition-colors",
        checked ? "border-accent/40 bg-accent-soft" : "border-border-strong bg-surface-2",
        disabled ? "opacity-40" : "cursor-pointer hover:brightness-110",
        FOCUS_RING,
        className,
      )}
    >
      <span
        aria-hidden="true"
        className={clsx(
          "inline-block h-3.5 w-3.5 transform rounded-full shadow-panel transition-transform",
          checked ? "translate-x-[18px] bg-accent" : "translate-x-1 bg-ink-faint",
        )}
      />
    </button>
  );
}
