// Pure logic behind the work_dir combobox's terminal-style tab-completion,
// extracted so the keyboard behavior can be unit-tested without a DOM. The
// daemon (GET /fs/dirs, internal/api/fs.go) matches directory names
// case-insensitively, so the helpers here compare the same way while preserving
// the original casing of the returned suggestions.

/** Ensures a path ends in exactly one trailing slash, so completing to a
 *  directory descends into it (terminal-style) on the next fetch. */
export function ensureTrailingSlash(path: string): string {
  return path.endsWith("/") ? path : `${path}/`;
}

/** The number of leading characters two strings share, compared
 *  case-insensitively, up to `max`. */
function commonPrefixLength(a: string, b: string, max: number): number {
  const limit = Math.min(max, a.length, b.length);
  let i = 0;
  while (i < limit && a[i].toLowerCase() === b[i].toLowerCase()) i += 1;
  return i;
}

/** Longest common prefix of the suggestions, compared case-insensitively but
 *  returned with the casing of the first suggestion (so completion never
 *  rewrites the case the filesystem reported). Empty list → "". */
export function longestCommonPrefix(items: string[]): string {
  if (items.length === 0) return "";
  const first = items[0];
  let end = first.length;
  for (let i = 1; i < items.length; i += 1) {
    end = commonPrefixLength(first, items[i], end);
    if (end === 0) break;
  }
  return first.slice(0, end);
}

/** Next selectable index with wrap-around. A negative (none-selected) current
 *  index steps to the first row; an empty list stays at -1. */
export function nextIndex(current: number, length: number): number {
  if (length === 0) return -1;
  if (current < 0) return 0;
  return (current + 1) % length;
}

/** Previous selectable index with wrap-around. A negative (none-selected)
 *  current index steps to the last row; an empty list stays at -1. */
export function prevIndex(current: number, length: number): number {
  if (length === 0) return -1;
  if (current < 0) return length - 1;
  return (current - 1 + length) % length;
}

/** What pressing Tab should do given the current input and the live
 *  suggestions:
 *  - `complete`: exactly one candidate — fill it (with a trailing slash) and
 *    descend.
 *  - `extend`: several candidates whose common prefix is longer than the input —
 *    extend the input to that prefix.
 *  - `cycle`: nothing more to fill in — move the selection to the next row. */
export type TabResult =
  | { kind: "complete"; value: string }
  | { kind: "extend"; value: string }
  | { kind: "cycle" };

export function decideTab(input: string, suggestions: string[]): TabResult {
  if (suggestions.length === 1) {
    return { kind: "complete", value: ensureTrailingSlash(suggestions[0]) };
  }
  const lcp = longestCommonPrefix(suggestions);
  if (lcp.length > input.length) {
    return { kind: "extend", value: lcp };
  }
  return { kind: "cycle" };
}
