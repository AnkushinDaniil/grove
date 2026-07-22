import { create } from "zustand";

interface DiffViewedState {
  /** Viewed file paths, keyed by an opaque scope string the caller controls
   *  (e.g. `pr:${dir}:${pr}` or `worktree:${node}:${repo}`) so unrelated
   *  diffs never share viewed-state. */
  viewedByScope: Record<string, ReadonlySet<string>>;

  toggleViewed: (scope: string, path: string) => void;
  markAllViewed: (scope: string, paths: string[]) => void;
  resetScope: (scope: string) => void;
}

export const useDiffViewedStore = create<DiffViewedState>((set) => ({
  viewedByScope: {},

  toggleViewed: (scope, path) =>
    set((s) => {
      const next = new Set(s.viewedByScope[scope]);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return { viewedByScope: { ...s.viewedByScope, [scope]: next } };
    }),

  markAllViewed: (scope, paths) =>
    set((s) => {
      const next = new Set(s.viewedByScope[scope]);
      for (const p of paths) next.add(p);
      return { viewedByScope: { ...s.viewedByScope, [scope]: next } };
    }),

  resetScope: (scope) =>
    set((s) => {
      if (!(scope in s.viewedByScope)) return s;
      const rest = { ...s.viewedByScope };
      delete rest[scope];
      return { viewedByScope: rest };
    }),
}));

/** Reactive selector: is this file marked viewed within its scope? Use via
 *  `useDiffViewedStore((s) => selectIsViewed(s, scope, path))`. */
export function selectIsViewed(state: Pick<DiffViewedState, "viewedByScope">, scope: string, path: string): boolean {
  return state.viewedByScope[scope]?.has(path) ?? false;
}

/** Count of viewed files within a scope, restricted to a known path list so
 *  stale entries from a previous file set (or a different diff) never
 *  inflate the count. */
export function selectViewedCount(
  state: Pick<DiffViewedState, "viewedByScope">,
  scope: string,
  paths: string[],
): number {
  const set = state.viewedByScope[scope];
  if (!set) return 0;
  let count = 0;
  for (const p of paths) if (set.has(p)) count++;
  return count;
}
