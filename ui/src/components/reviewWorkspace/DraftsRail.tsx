import { Inbox } from "lucide-react";
import { DraftPendingCard } from "./DraftPendingCard";
import { EmptyState } from "../common/EmptyState";
import type { DraftComment } from "../../gen/types";

interface DraftsRailProps {
  drafts: DraftComment[];
}

/** Pending drafts, collected in one place: a right-hand rail on wide
 *  screens, a capped-height panel below the diff on narrow ones. These are
 *  exactly what SubmitBar sends when the review is submitted. */
export function DraftsRail({ drafts }: DraftsRailProps) {
  return (
    <aside className="flex max-h-56 shrink-0 flex-col border-t border-border bg-surface lg:max-h-none lg:w-72 lg:border-t-0 lg:border-l">
      <div className="shrink-0 border-b border-border px-3 py-2">
        <h2 className="text-2xs font-medium tracking-wide text-ink-faint uppercase">
          Pending drafts <span className="text-ink-disabled normal-case">{drafts.length}</span>
        </h2>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {drafts.length === 0 ? (
          <EmptyState
            icon={<Inbox size={22} strokeWidth={1.5} />}
            title="No drafts yet"
            description="Hover a diff line and click + to leave a comment."
            className="py-8"
          />
        ) : (
          <div className="space-y-2">
            {drafts.map((d) => (
              <DraftPendingCard key={d.id} draft={d} showLocation />
            ))}
          </div>
        )}
      </div>
    </aside>
  );
}
