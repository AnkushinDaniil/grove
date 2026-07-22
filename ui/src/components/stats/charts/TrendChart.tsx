import { useMemo, useRef, useState } from "react";
import type { KeyboardEvent, PointerEvent as ReactPointerEvent } from "react";
import clsx from "clsx";
import { FOCUS_RING } from "../../../lib/constants";

export interface TrendPoint {
  /** ISO day ("2026-07-20") -- doubles as the row key and the label source. */
  key: string;
  value: number;
}

interface TrendChartProps {
  points: TrendPoint[];
  formatValue: (v: number) => string;
  formatLabel?: (key: string) => string;
  ariaLabel: string;
  className?: string;
}

const VB_W = 720;
const VB_H = 168;
const PAD_T = 20;
const PAD_B = 28;
const PAD_X = 6;
const TOOLTIP_MARGIN = 44; // approx. half the tooltip's rendered width

function defaultFormatLabel(key: string): string {
  const d = new Date(`${key}T00:00:00`);
  if (Number.isNaN(d.getTime())) return key;
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function pickLabelIndices(n: number, maxLabels = 7): number[] {
  if (n <= 1) return [0];
  if (n <= maxLabels) return Array.from({ length: n }, (_, i) => i);
  const idx = new Set<number>();
  for (let k = 0; k < maxLabels; k++) idx.add(Math.round((k * (n - 1)) / (maxLabels - 1)));
  return [...idx].sort((a, b) => a - b);
}

/**
 * Hand-rolled inline-SVG area/line trend chart -- cost or tokens by day, the
 * one series this dashboard leads with, so it's the only chart wearing the
 * accent (see lib/vizColor.ts for why every other magnitude chart here uses
 * violet instead). Straight, unsmoothed segments deliberately -- a curve fit
 * would misrepresent the real daily values for the sake of prettiness.
 *
 * The crosshair + tooltip track one `active` index driven by either pointer
 * hover or keyboard arrow focus, so both input modes see identical detail
 * (dataviz skill: "same details on keyboard focus as on hover"). An sr-only
 * table mirrors the same day/value pairs for screen readers, so no value is
 * hover-only.
 */
export function TrendChart({ points, formatValue, formatLabel = defaultFormatLabel, ariaLabel, className }: TrendChartProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [active, setActive] = useState<{ index: number; px: number; py: number } | null>(null);

  const n = points.length;
  const max = useMemo(() => Math.max(...points.map((p) => p.value), 1) * 1.15, [points]);
  const baselineY = VB_H - PAD_B;

  const coords = useMemo<[number, number][]>(() => {
    const plotW = VB_W - PAD_X * 2;
    const plotH = VB_H - PAD_T - PAD_B;
    return points.map((p, i) => {
      const x = n <= 1 ? PAD_X + plotW / 2 : PAD_X + (i / (n - 1)) * plotW;
      const y = baselineY - (p.value / max) * plotH;
      return [x, y];
    });
  }, [points, max, n, baselineY]);

  if (n === 0) return null;

  const linePath = coords.map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`).join(" ");
  const areaPath = `${linePath} L${coords[n - 1][0].toFixed(1)},${baselineY} L${coords[0][0].toFixed(1)},${baselineY} Z`;
  const [lastX, lastY] = coords[n - 1];
  const labelIndices = pickLabelIndices(n);

  function pxFor(index: number): number {
    const width = containerRef.current?.clientWidth ?? 0;
    return (coords[index][0] / VB_W) * width;
  }

  function onPointerMove(e: ReactPointerEvent<HTMLDivElement>) {
    const rect = containerRef.current?.getBoundingClientRect();
    if (!rect || rect.width === 0) return;
    const fx = ((e.clientX - rect.left) / rect.width) * VB_W;
    let index = 0;
    let bestDist = Infinity;
    for (let i = 0; i < n; i++) {
      const d = Math.abs(coords[i][0] - fx);
      if (d < bestDist) {
        bestDist = d;
        index = i;
      }
    }
    setActive({ index, px: e.clientX - rect.left, py: e.clientY - rect.top });
  }

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    let index = active?.index ?? n - 1;
    if (e.key === "ArrowLeft") index = Math.max(0, index - 1);
    else if (e.key === "ArrowRight") index = Math.min(n - 1, index + 1);
    else if (e.key === "Home") index = 0;
    else if (e.key === "End") index = n - 1;
    else return;
    e.preventDefault();
    setActive({ index, px: pxFor(index), py: (containerRef.current?.clientHeight ?? 0) / 2 });
  }

  const activePoint = active ? points[active.index] : undefined;
  const containerWidth = containerRef.current?.clientWidth ?? 0;
  const tooltipLeft = active ? Math.min(Math.max(active.px, TOOLTIP_MARGIN), Math.max(containerWidth - TOOLTIP_MARGIN, TOOLTIP_MARGIN)) : 0;

  return (
    <div className={className}>
      <div
        ref={containerRef}
        role="group"
        aria-label={ariaLabel}
        tabIndex={0}
        onPointerMove={onPointerMove}
        onPointerLeave={() => setActive(null)}
        onFocus={() => setActive({ index: n - 1, px: pxFor(n - 1), py: 0 })}
        onBlur={() => setActive(null)}
        onKeyDown={onKeyDown}
        className={clsx("relative w-full cursor-crosshair rounded-md", FOCUS_RING)}
      >
        <svg viewBox={`0 0 ${VB_W} ${VB_H}`} className="block w-full" preserveAspectRatio="none" aria-hidden="true">
          <line x1={PAD_X} y1={baselineY} x2={VB_W - PAD_X} y2={baselineY} style={{ stroke: "var(--color-border)" }} strokeWidth={1} />
          <path d={areaPath} style={{ fill: "var(--color-accent)", fillOpacity: 0.12 }} stroke="none" />
          <path d={linePath} fill="none" style={{ stroke: "var(--color-accent)" }} strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
          {active && (
            <line
              x1={coords[active.index][0]}
              y1={PAD_T}
              x2={coords[active.index][0]}
              y2={baselineY}
              style={{ stroke: "var(--color-border-strong)" }}
              strokeWidth={1}
            />
          )}
          <circle
            cx={active ? coords[active.index][0] : lastX}
            cy={active ? coords[active.index][1] : lastY}
            r={5}
            style={{ fill: "var(--color-accent)", stroke: "var(--color-surface)" }}
            strokeWidth={2}
          />
          {!active && (
            <text x={lastX} y={lastY - 10} textAnchor="end" className="text-[13px] font-semibold" style={{ fill: "var(--color-ink)" }}>
              {formatValue(points[n - 1].value)}
            </text>
          )}
          {labelIndices.map((i) => (
            <text
              key={points[i].key}
              x={coords[i][0]}
              y={VB_H - 8}
              textAnchor="middle"
              className="text-[11px]"
              style={{ fill: "var(--color-ink-faint)" }}
            >
              {formatLabel(points[i].key)}
            </text>
          ))}
        </svg>
        {active && activePoint && (
          <div
            className="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-[calc(100%+8px)] whitespace-nowrap rounded-md border border-border-strong bg-surface-2 px-2 py-1 text-2xs shadow-popover"
            style={{ left: tooltipLeft, top: active.py }}
          >
            <div className="font-sans text-ink-faint">{formatLabel(activePoint.key)}</div>
            <div className="font-semibold text-ink">{formatValue(activePoint.value)}</div>
          </div>
        )}
      </div>
      <table className="sr-only">
        <caption>{ariaLabel}</caption>
        <thead>
          <tr>
            <th>Day</th>
            <th>Value</th>
          </tr>
        </thead>
        <tbody>
          {points.map((p) => (
            <tr key={p.key}>
              <td>{formatLabel(p.key)}</td>
              <td>{formatValue(p.value)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
