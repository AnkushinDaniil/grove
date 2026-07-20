import clsx from "clsx";
import type { ReactNode } from "react";

interface PillProps {
  children: ReactNode;
  tone?: "neutral" | "accent" | "muted";
  className?: string;
  title?: string;
}

/** Small chip used for status/driver/profile/kind labels throughout the
 *  node view and tree rail. */
export function Pill({ children, tone = "neutral", className, title }: PillProps) {
  return (
    <span
      title={title}
      className={clsx(
        "inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-2xs leading-none whitespace-nowrap",
        tone === "neutral" && "border-border-strong bg-surface-2 text-ink-muted",
        tone === "muted" && "border-dashed border-border bg-transparent text-ink-faint",
        tone === "accent" && "border-accent/30 bg-accent-soft text-accent",
        className,
      )}
    >
      {children}
    </span>
  );
}
