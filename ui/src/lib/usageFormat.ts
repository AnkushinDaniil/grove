/** Short countdown like "42m", "3h 5m", "2d" for resets_at/cooldown_until. */
export function formatCountdown(iso: string | undefined, now = Date.now()): string {
  if (!iso) return "";
  const target = Date.parse(iso);
  if (Number.isNaN(target)) return "";
  const diffMs = target - now;
  if (diffMs <= 0) return "now";

  const mins = Math.round(diffMs / 60_000);
  if (mins < 60) return `${mins}m`;
  const hours = Math.floor(mins / 60);
  const remMins = mins % 60;
  if (hours < 24) return remMins > 0 ? `${hours}h ${remMins}m` : `${hours}h`;
  const days = Math.floor(hours / 24);
  return `${days}d`;
}

export function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

export function formatCost(usd: number): string {
  return `$${usd.toFixed(2)}`;
}
