import clsx from "clsx";
import { useUsageStore } from "../../state/usage";
import { cycleUsageWindow } from "../../state/usagePolling";
import { profileColor } from "../../lib/profileColor";
import { formatCost, formatCountdown, formatTokens } from "../../lib/usageFormat";
import type { UsageWindow } from "../../gen/types";

function segmentTooltip(p: UsageWindow): string {
  const lines = [
    `${p.name} (${p.driver}) -- ${p.window} window`,
    `${formatTokens(p.input_tokens)} in / ${formatTokens(p.output_tokens)} out / ${formatTokens(p.cache_read_tokens)} cached`,
    `~${formatCost(p.cost_usd)}`,
  ];
  if (p.resets_at) lines.push(`resets in ${formatCountdown(p.resets_at)}`);
  return lines.join("\n");
}

function ProfileSegment({ profile }: { profile: UsageWindow }) {
  const color = profileColor(profile.profile_id);
  const cooling = Boolean(profile.cooldown_until) && Date.parse(profile.cooldown_until!) > Date.now();

  if (cooling) {
    return (
      <span
        className="flex items-center gap-1.5 whitespace-nowrap text-status-failed"
        title={segmentTooltip(profile)}
      >
        <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-status-failed" />
        {profile.name}: rate-limited, resets in {formatCountdown(profile.cooldown_until)}
      </span>
    );
  }

  const util = profile.utilization;
  const barColor =
    util === null
      ? undefined
      : util > 0.9
        ? "var(--color-status-failed)"
        : util > 0.7
          ? "var(--color-status-starting)"
          : color;

  return (
    <span className="flex items-center gap-1.5 whitespace-nowrap" title={segmentTooltip(profile)}>
      <span className="h-1.5 w-1.5 shrink-0 rounded-full" style={{ backgroundColor: color }} />
      <span className="text-ink-faint">{profile.name}</span>
      {util === null ? (
        <span className="flex items-center gap-1">
          <span
            className="h-1.5 w-6 shrink-0 rounded-full"
            style={{
              backgroundImage:
                "repeating-linear-gradient(45deg, var(--color-border-strong), var(--color-border-strong) 3px, transparent 3px, transparent 6px)",
            }}
            aria-hidden="true"
          />
          {/* utilization is unknown -- show raw counts instead of a % bar */}
          <span className="text-ink-disabled">
            {formatTokens(profile.input_tokens)}/{formatTokens(profile.output_tokens)}
          </span>
        </span>
      ) : (
        <span className="h-1.5 w-10 shrink-0 overflow-hidden rounded-full bg-surface-2">
          <span
            className="block h-full rounded-full transition-[width]"
            style={{ width: `${Math.min(100, util * 100)}%`, backgroundColor: barColor }}
          />
        </span>
      )}
    </span>
  );
}

interface UsageMeterProps {
  className?: string;
}

/** Per-profile usage/rate-limit plaque backed by GET /usage. Renders
 *  nothing until the first successful fetch resolves with at least one
 *  profile (the endpoint returns {profiles: []} until the daemon's
 *  aggregator lands), so it never shows broken/empty chrome. */
export function UsageMeter({ className }: UsageMeterProps) {
  const profiles = useUsageStore((s) => s.profiles);
  const window = useUsageStore((s) => s.window);

  if (profiles.length === 0) return null;

  return (
    <button
      type="button"
      onClick={cycleUsageWindow}
      className={clsx("flex min-w-0 items-center gap-3 rounded px-1.5 text-2xs hover:bg-hover", className)}
      title={`Click to switch to the ${window === "5h" ? "week" : "5h"} view`}
    >
      {profiles.map((p) => (
        <ProfileSegment key={p.profile_id} profile={p} />
      ))}
      <span className="shrink-0 text-ink-disabled">{window}</span>
    </button>
  );
}
