import { PRRow } from "./PRRow";
import type { PR } from "../../gen/types";

interface BucketSectionProps {
  dir: string;
  repoName: string;
  label: string;
  prs: PR[];
}

/** One labeled bucket ("Needs review", "Your PRs", ...) within a repo
 *  section. Callers only render this for non-empty buckets. */
export function BucketSection({ dir, repoName, label, prs }: BucketSectionProps) {
  return (
    <div>
      <h3 className="flex items-baseline gap-1.5 px-5 pt-2.5 pb-1 text-2xs font-medium tracking-wide text-ink-faint uppercase">
        {label}
        <span className="text-ink-disabled normal-case">{prs.length}</span>
      </h3>
      <div className="divide-y divide-border/60">
        {prs.map((pr) => (
          <PRRow key={pr.number} pr={pr} dir={dir} repoName={repoName} />
        ))}
      </div>
    </div>
  );
}
