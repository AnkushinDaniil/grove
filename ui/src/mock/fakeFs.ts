import type { DirSuggestions } from "../gen/types";

// A tiny static directory tree backing the mock suggestDirs endpoint so
// `npm run dev:mock` can demo terminal-style work_dir completion with no daemon.
// It mirrors the daemon's completion semantics (internal/api/fs.go): expand
// "~"/empty prefixes to home, split into parent + base, then return the
// directory children whose name case-insensitively prefix-matches, sorted and
// capped, with hidden entries surfaced only when the base itself starts with ".".

const MOCK_HOME = "/Users/daniil";
const MAX = 50;

/** Every directory in the fake tree (absolute paths). Immediate children are
 *  derived by matching each entry's parent. */
const DIRS: readonly string[] = [
  "/Users/daniil/code",
  "/Users/daniil/code/grove",
  "/Users/daniil/code/grove/cmd",
  "/Users/daniil/code/grove/internal",
  "/Users/daniil/code/grove/ui",
  "/Users/daniil/code/orchard",
  "/Users/daniil/code/website",
  "/Users/daniil/docs",
  "/Users/daniil/docs/design",
  "/Users/daniil/docs/notes",
  "/Users/daniil/work",
  "/Users/daniil/work/reports",
  "/Users/daniil/work/reports/2026",
  "/Users/daniil/Downloads",
  "/Users/daniil/.config",
];

function dirname(p: string): string {
  const i = p.lastIndexOf("/");
  return i <= 0 ? "/" : p.slice(0, i);
}

function basename(p: string): string {
  return p.slice(p.lastIndexOf("/") + 1);
}

/** Mirrors internal/api/fs.go completion over the static fake tree. */
export function suggestDirsMock(prefix: string): DirSuggestions {
  let expanded = prefix;
  if (prefix === "") expanded = `${MOCK_HOME}/`;
  else if (prefix === "~") expanded = MOCK_HOME;
  else if (prefix.startsWith("~/")) expanded = MOCK_HOME + prefix.slice(1);

  let parent: string;
  let base: string;
  if (expanded.endsWith("/")) {
    parent = expanded.replace(/\/+$/, "") || "/";
    base = "";
  } else {
    parent = dirname(expanded);
    base = basename(expanded);
  }

  const includeHidden = base.startsWith(".");
  const lowerBase = base.toLowerCase();
  const dirs = DIRS.filter((d) => dirname(d) === parent)
    .filter((d) => includeHidden || !basename(d).startsWith("."))
    .filter((d) => basename(d).toLowerCase().startsWith(lowerBase))
    .sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()))
    .slice(0, MAX);

  return { dirs, home: MOCK_HOME };
}
