import clsx from "clsx";

interface AttentionBadgeProps {
  count: number;
  className?: string;
}

/** Small count pill for "N things need you," pulsing while count > 0. Uses
 *  the brand accent (not a status hue) since this is a call-to-action, not
 *  a status readout. */
export function AttentionBadge({ count, className }: AttentionBadgeProps) {
  if (count <= 0) return null;
  return (
    <span
      className={clsx(
        "inline-flex h-[1.1rem] min-w-[1.1rem] shrink-0 animate-pulse items-center justify-center rounded-full bg-accent px-1 text-[10px] font-semibold leading-none text-accent-ink",
        className,
      )}
      aria-label={`${count} need${count === 1 ? "s" : ""} attention`}
    >
      {count > 99 ? "99+" : count}
    </span>
  );
}
