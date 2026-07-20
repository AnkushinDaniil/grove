import type { ReactNode } from "react";
import clsx from "clsx";

interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}

export function EmptyState({ icon, title, description, action, className }: EmptyStateProps) {
  return (
    <div
      className={clsx(
        "flex flex-1 flex-col items-center justify-center gap-3 px-6 py-12 text-center",
        className,
      )}
    >
      {icon && <div className="text-ink-faint">{icon}</div>}
      <div className="space-y-1">
        <p className="font-sans text-sm font-medium text-ink">{title}</p>
        {description && <p className="max-w-sm font-sans text-xs text-ink-faint">{description}</p>}
      </div>
      {action}
    </div>
  );
}
