import { ExternalLink } from "lucide-react";
import { BucketSection } from "./BucketSection";
import type { ReviewBuckets, ReviewRepo } from "../../gen/types";

const BUCKET_META: { key: keyof ReviewBuckets; label: string }[] = [
  { key: "needs_review", label: "Needs review" },
  { key: "re_review", label: "Changed since your review" },
  { key: "reviewed", label: "Reviewed" },
  { key: "mine", label: "Your PRs" },
];

interface RepoSectionProps {
  repo: ReviewRepo;
}

/** One watched repository: header with a GitHub link, then its four
 *  buckets. Empty buckets are hidden; if nothing is actionable (needs_review
 *  + re_review both empty) a friendly note takes their place instead of two
 *  missing sections leaving an unexplained gap. */
export function RepoSection({ repo }: RepoSectionProps) {
  const actionable = repo.buckets.needs_review.length + repo.buckets.re_review.length;

  return (
    <section className="border-b border-border">
      <div className="flex items-center gap-2 border-b border-border bg-surface-2/60 px-5 py-2">
        <h2 className="min-w-0 flex-1 truncate font-mono text-xs font-medium text-ink">{repo.name_with_owner}</h2>
        <a
          href={`https://github.com/${repo.name_with_owner}/pulls`}
          target="_blank"
          rel="noreferrer"
          className="flex shrink-0 items-center gap-1 text-2xs text-ink-faint hover:text-accent"
        >
          Open on GitHub
          <ExternalLink size={11} />
        </a>
      </div>

      {actionable === 0 && <p className="px-5 py-4 text-xs text-ink-faint">Nothing needs review here.</p>}

      {BUCKET_META.map(({ key, label }) => {
        const prs = repo.buckets[key];
        if (prs.length === 0) return null;
        return <BucketSection key={key} dir={repo.dir} repoName={repo.name_with_owner} label={label} prs={prs} />;
      })}
    </section>
  );
}
