import { useNavigate } from "react-router";
import { Boxes } from "lucide-react";
import { useTreeStore } from "../../state/tree";
import { useRollup } from "../../hooks/useRollup";
import { EmptyState } from "../common/EmptyState";
import { FOCUS_RING } from "../../lib/constants";
import clsx from "clsx";

export function WelcomeView() {
  const rootId = useTreeStore((s) => s.rootId);
  const loaded = useTreeStore((s) => s.loaded);
  const rootNode = useTreeStore((s) => (rootId ? s.nodesById[rootId] : undefined));
  const rollup = useRollup(rootId ?? "");
  const navigate = useNavigate();

  if (!loaded) {
    return <EmptyState title="Loading…" />;
  }

  if (!rootId || !rootNode) {
    return (
      <EmptyState
        icon={<Boxes size={28} strokeWidth={1.5} />}
        title="No workspace yet"
        description="Create your first project from the tree rail on the left."
      />
    );
  }

  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 px-6 text-center">
      <Boxes size={32} strokeWidth={1.5} className="text-ink-faint" />
      <div>
        <h1 className="text-lg font-semibold text-ink">{rootNode.title}</h1>
        <p className="mt-1 text-xs text-ink-faint">
          {rollup.total} node{rollup.total === 1 ? "" : "s"} in this workspace
          {rollup.attentionCount > 0 && ` · ${rollup.attentionCount} need attention`}
        </p>
      </div>
      <button
        type="button"
        onClick={() => navigate(`/n/${rootId}`)}
        className={clsx(
          "rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-accent-ink hover:bg-accent-strong",
          FOCUS_RING,
        )}
      >
        Open workspace
      </button>
      <p className="text-2xs text-ink-disabled">
        <kbd className="rounded border border-border-strong px-1">j</kbd>/
        <kbd className="rounded border border-border-strong px-1">k</kbd> to navigate ·{" "}
        <kbd className="rounded border border-border-strong px-1">⌘K</kbd> to jump anywhere
      </p>
    </div>
  );
}
