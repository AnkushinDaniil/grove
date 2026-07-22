import clsx from "clsx";
import { FOCUS_RING } from "../../../lib/constants";

interface RepoSwitcherProps {
  repos: string[];
  active: string;
  onChange: (repo: string) => void;
}

/** Only rendered when a task node spans more than one repo (see
 *  ReviewTab's reposFor -- read from an optional `meta.repos` string array,
 *  since the frozen API contract has no repo-enumeration endpoint). A
 *  single-repo node never shows this at all. */
export function RepoSwitcher({ repos, active, onChange }: RepoSwitcherProps) {
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-border bg-surface px-4 py-2">
      <span className="text-2xs text-ink-faint">Repo</span>
      {repos.map((repo) => (
        <button
          key={repo}
          type="button"
          onClick={() => onChange(repo)}
          className={clsx(
            "rounded-md border px-2 py-1 text-2xs font-medium",
            repo === active
              ? "border-accent/40 bg-accent-soft text-accent"
              : "border-border-strong text-ink-muted hover:bg-hover hover:text-ink",
            FOCUS_RING,
          )}
        >
          {repo}
        </button>
      ))}
    </div>
  );
}
