/** Formats a fractional hour count as "4.2h" (or "45m" under an hour, since
 *  a lone "0.2h" reads worse than "12m" at that scale). */
export function formatHours(hours: number): string {
  if (hours < 1) return `${Math.round(hours * 60)}m`;
  return `${hours.toFixed(1)}h`;
}

/** Formats a fractional minute count as "6.5m" (or "1h 20m" once it crosses
 *  an hour -- attention-wait p95 routinely does). */
export function formatMinutes(minutes: number): string {
  if (minutes >= 60) {
    const h = Math.floor(minutes / 60);
    const m = Math.round(minutes % 60);
    return m > 0 ? `${h}h ${m}m` : `${h}h`;
  }
  return `${minutes < 10 ? minutes.toFixed(1) : Math.round(minutes)}m`;
}

/** Formats a 0..1 ratio as a percent: one decimal below 10% (so a rare 2/340
 *  error rate doesn't round away to "0%"), whole numbers at or above it. */
export function formatPercent(ratio: number): string {
  const pct = ratio * 100;
  if (pct === 0) return "0%";
  if (pct < 10) return `${pct.toFixed(1)}%`;
  return `${Math.round(pct)}%`;
}
