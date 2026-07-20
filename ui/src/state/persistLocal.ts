const PREFIX = "grove.";

const RAIL_WIDTH_MIN = 240;
const RAIL_WIDTH_MAX = 320;
const RAIL_WIDTH_DEFAULT = 272;

function read<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(PREFIX + key);
    if (raw === null) return fallback;
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}

function write<T>(key: string, value: T): void {
  try {
    localStorage.setItem(PREFIX + key, JSON.stringify(value));
  } catch {
    // Private-mode/quota errors: persistence is a nicety, not correctness.
  }
}

export function loadCollapsedIds(): Set<string> {
  return new Set(read<string[]>("tree.collapsed", []));
}

export function saveCollapsedIds(ids: Set<string>): void {
  write("tree.collapsed", [...ids]);
}

function clampRailWidth(width: number): number {
  return Math.min(RAIL_WIDTH_MAX, Math.max(RAIL_WIDTH_MIN, Math.round(width)));
}

export function loadRailWidth(): number {
  return clampRailWidth(read<number>("rail.width", RAIL_WIDTH_DEFAULT));
}

export function saveRailWidth(width: number): void {
  write("rail.width", clampRailWidth(width));
}
