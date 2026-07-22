import { useEffect } from "react";
import { useShallow } from "zustand/react/shallow";
import { MessageSquare } from "lucide-react";
import { apiClient } from "../../state/api";
import { selectVisibleFeedback, useFeedbackStore } from "../../state/feedback";
import { SegmentedControl } from "../common/SegmentedControl";
import { EmptyState } from "../common/EmptyState";
import { FeedbackItem } from "./FeedbackItem";
import type { FeedbackStatusFilter } from "../../gen/types";

const FILTER_OPTIONS: { value: FeedbackStatusFilter; label: string }[] = [
  { value: "open", label: "Open" },
  { value: "resolved", label: "Resolved" },
  { value: "all", label: "All" },
];

/** Feedback loop list: filter open/resolved/all, refetching from GET
 *  /feedback on every filter change (server-side filtered); the visible
 *  slice is also re-derived client-side (selectVisibleFeedback) so an
 *  optimistic resolve/create-fix-task from FeedbackItem is reflected
 *  immediately without waiting on a refetch. */
export function FeedbackTab() {
  const filter = useFeedbackStore((s) => s.filter);
  const loaded = useFeedbackStore((s) => s.loaded);
  const lastError = useFeedbackStore((s) => s.lastError);
  // selectVisibleFeedback allocates a new array each call; useShallow keeps
  // the subscription stable, same guard InboxView/ReviewsView use for their
  // own derived-array selectors.
  const items = useFeedbackStore(useShallow(selectVisibleFeedback));

  useEffect(() => {
    let cancelled = false;
    const store = useFeedbackStore.getState();
    store.setLoading(true);
    apiClient
      .listFeedback(filter)
      .then((res) => {
        if (!cancelled) useFeedbackStore.getState().setItems(res);
      })
      .catch((err) => {
        if (!cancelled) useFeedbackStore.getState().setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!cancelled) useFeedbackStore.getState().setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [filter]);

  return (
    <div className="flex h-full flex-col">
      <div className="shrink-0 border-b border-border px-4 py-2.5">
        <SegmentedControl options={FILTER_OPTIONS} value={filter} onChange={(v) => useFeedbackStore.getState().setFilter(v)} />
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {lastError && (
          <div role="alert" className="border-b border-status-failed/30 bg-status-failed/10 px-4 py-2 text-2xs text-status-failed">
            Couldn't load feedback: {lastError}
          </div>
        )}
        {!loaded && <div className="px-4 py-4 text-xs text-ink-faint">Loading feedback…</div>}
        {loaded && items.length === 0 && (
          <EmptyState
            icon={<MessageSquare size={28} strokeWidth={1.5} />}
            title={filter === "open" ? "No open feedback" : "No feedback here"}
            description={filter === "open" ? "Nothing needs a look right now." : "Nothing recorded for this filter yet."}
          />
        )}
        {items.length > 0 && <ul>{items.map((f) => <FeedbackItem key={f.id} feedback={f} />)}</ul>}
      </div>
    </div>
  );
}
