import { useEffect, useState } from "react";
import { absoluteTime, relativeTime } from "../../lib/time";

interface RelativeTimeProps {
  iso: string | undefined;
  className?: string;
}

/** Renders "3m ago"-style text that keeps itself fresh, with the absolute
 *  timestamp available on hover via the native title tooltip. */
export function RelativeTime({ iso, className }: RelativeTimeProps) {
  const [, forceTick] = useState(0);

  useEffect(() => {
    const id = setInterval(() => forceTick((n) => n + 1), 30_000);
    return () => clearInterval(id);
  }, []);

  if (!iso) return null;
  return (
    <time dateTime={iso} title={absoluteTime(iso)} className={className}>
      {relativeTime(iso)}
    </time>
  );
}
