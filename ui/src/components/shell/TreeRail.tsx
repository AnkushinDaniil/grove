import { useCallback, useEffect, useMemo, useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import clsx from "clsx";
import { Link, useNavigate, useParams } from "react-router";
import { Inbox as InboxIcon, Plus, X } from "lucide-react";
import { useTreeStore } from "../../state/tree";
import { selectInboxEvents, useInboxStore } from "../../state/inbox";
import { apiClient } from "../../state/api";
import { loadCollapsedIds, loadRailWidth, saveCollapsedIds, saveRailWidth } from "../../state/persistLocal";
import { flattenVisible } from "../../lib/flattenTree";
import { CHILD_KIND_FOR, FOCUS_RING } from "../../lib/constants";
import { useKeyboardNav } from "../../hooks/useKeyboardNav";
import { TreeNodeRow } from "../tree/TreeNodeRow";
import { AttentionBadge } from "../tree/AttentionBadge";
import { InlineCreateRow } from "../tree/InlineCreateRow";
import { ResizeHandle } from "./ResizeHandle";
import type { NodeID } from "../../gen/types";

interface TreeRailProps {
  /** Below `md`, the rail renders as a slide-over drawer instead of a fixed
   *  sidebar; these control that overlay's open state. Ignored at md+,
   *  where the rail is always visible in normal document flow. */
  mobileOpen: boolean;
  onMobileClose: () => void;
}

export function TreeRail({ mobileOpen, onMobileClose }: TreeRailProps) {
  const rootId = useTreeStore((s) => s.rootId);
  const childrenByParent = useTreeStore((s) => s.childrenByParent);
  const nodesById = useTreeStore((s) => s.nodesById);
  const loaded = useTreeStore((s) => s.loaded);
  const navigate = useNavigate();
  const params = useParams<{ id?: string }>();
  const activeId = params.id;

  const [width, setWidth] = useState<number>(() => loadRailWidth());
  const [collapsed, setCollapsed] = useState<Set<NodeID>>(() => loadCollapsedIds());
  const [focusedId, setFocusedId] = useState<NodeID | null>(null);
  const [creatingUnder, setCreatingUnder] = useState<NodeID | null>(null);
  const inboxCount = useInboxStore((s) => selectInboxEvents(s).length);

  useEffect(() => saveCollapsedIds(collapsed), [collapsed]);
  useEffect(() => saveRailWidth(width), [width]);

  // Keep the keyboard cursor pointed at whatever the route just opened.
  useEffect(() => {
    if (activeId) setFocusedId(activeId);
  }, [activeId]);
  useEffect(() => {
    if (!focusedId && rootId) setFocusedId(rootId);
  }, [focusedId, rootId]);

  const isCollapsed = useCallback((id: NodeID) => collapsed.has(id), [collapsed]);
  const visibleIds = useMemo(
    () => (rootId ? flattenVisible(rootId, childrenByParent, isCollapsed) : []),
    [rootId, childrenByParent, isCollapsed],
  );

  const toggle = useCallback((id: NodeID, expand?: boolean) => {
    setCollapsed((prev) => {
      const nextExpanded = expand ?? prev.has(id); // toggle: expand iff currently collapsed
      const next = new Set(prev);
      if (nextExpanded) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const handleAck = useCallback((id: NodeID) => {
    void apiClient.ackNode(id).catch(() => {
      // Best-effort: a WS delta (or manual retry) reconciles state; not
      // worth a dedicated error surface for a single-node ack.
    });
  }, []);

  const handleNewChild = useCallback(
    (id: NodeID) => {
      const kind = useTreeStore.getState().nodesById[id]?.kind;
      if (!kind || !CHILD_KIND_FOR[kind]) return;
      if (isCollapsed(id)) toggle(id, true);
      setCreatingUnder(id);
    },
    [isCollapsed, toggle],
  );

  useKeyboardNav({
    enabled: loaded,
    visibleIds,
    focusedId,
    setFocusedId,
    isExpanded: (id) => !isCollapsed(id),
    hasChildren: (id) => (childrenByParent[id]?.length ?? 0) > 0,
    parentOf: (id) => useTreeStore.getState().nodesById[id]?.parent_id || null,
    toggleExpanded: (id, expand) => toggle(id, expand),
    onOpen: (id) => {
      navigate(`/n/${id}`);
      onMobileClose();
    },
    onAck: handleAck,
    onNewChild: handleNewChild,
  });

  async function submitCreate(parentId: NodeID, title: string) {
    const parentKind = useTreeStore.getState().nodesById[parentId]?.kind;
    const kind = parentKind ? CHILD_KIND_FOR[parentKind] : null;
    setCreatingUnder(null);
    if (!kind) return;
    try {
      const created = await apiClient.createNode({ parent_id: parentId, kind, title });
      navigate(`/n/${created.id}`);
      onMobileClose();
    } catch {
      // The inline "+ new" affordance is still there to retry.
    }
  }

  function renderBranch(id: NodeID, depth: number): ReactNode {
    const kids = childrenByParent[id] ?? [];
    const expanded = !isCollapsed(id);
    const childKind = CHILD_KIND_FOR[nodesById[id]?.kind ?? "task"] ?? "task";

    return (
      <div key={id} role="group">
        <TreeNodeRow
          nodeId={id}
          depth={depth}
          expanded={expanded}
          hasChildren={kids.length > 0}
          focused={focusedId === id}
          active={activeId === id}
          onToggle={() => toggle(id)}
          onSelect={() => {
            setFocusedId(id);
            onMobileClose();
          }}
        />
        {expanded && kids.map((childId) => renderBranch(childId, depth + 1))}
        {expanded && creatingUnder === id && (
          <InlineCreateRow
            indentPx={6 + (depth + 1) * 14 + 20}
            placeholder={`New ${childKind}…`}
            onSubmit={(title) => void submitCreate(id, title)}
            onCancel={() => setCreatingUnder(null)}
          />
        )}
      </div>
    );
  }

  return (
    <>
      {/* Backdrop: mobile slide-over only. */}
      {mobileOpen && (
        <div
          className="fixed inset-0 z-30 bg-canvas/70 backdrop-blur-sm md:hidden"
          onClick={onMobileClose}
          aria-hidden="true"
        />
      )}
      <div
        className={clsx(
          "fixed inset-y-0 left-0 z-40 flex h-full w-[86vw] max-w-xs flex-col border-r border-border bg-surface shadow-popover transition-transform duration-200 ease-out",
          "md:relative md:z-auto md:w-[var(--rail-w)] md:max-w-none md:translate-x-0 md:shadow-none md:transition-none",
          mobileOpen ? "translate-x-0" : "-translate-x-full",
        )}
        style={{ "--rail-w": `${width}px` } as CSSProperties}
      >
        <div className="flex items-center gap-1.5 px-3 py-2.5 text-2xs font-medium tracking-wide text-ink-faint">
          <span className="text-accent">◆</span>
          <span className="flex-1">grove</span>
          <Link
            to="/inbox"
            aria-label="Inbox"
            onClick={onMobileClose}
            className={clsx(
              "flex min-h-11 items-center gap-1 rounded px-1.5 hover:bg-hover hover:text-ink md:min-h-0 md:py-0.5",
              FOCUS_RING,
            )}
          >
            <InboxIcon size={13} />
            <AttentionBadge count={inboxCount} />
          </Link>
          <button
            type="button"
            onClick={onMobileClose}
            aria-label="Close"
            className={clsx(
              "flex h-11 w-11 items-center justify-center rounded text-ink-faint hover:bg-hover hover:text-ink md:hidden",
              FOCUS_RING,
            )}
          >
            <X size={16} />
          </button>
        </div>
        <div role="tree" aria-label="Node tree" className="min-h-0 flex-1 overflow-y-auto px-1.5 pb-2">
          {!loaded && <div className="px-3 py-4 text-2xs text-ink-faint">Loading…</div>}
          {loaded && rootId && renderBranch(rootId, 0)}
          {loaded && !rootId && <div className="px-3 py-4 text-2xs text-ink-faint">No workspace yet.</div>}
        </div>
        {rootId && creatingUnder !== rootId && (
          <div className="border-t border-border px-1.5 py-1.5">
            <button
              type="button"
              onClick={() => setCreatingUnder(rootId)}
              className="flex min-h-11 w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-xs text-ink-faint hover:bg-hover hover:text-ink md:min-h-0"
            >
              <Plus size={13} />
              New project
            </button>
          </div>
        )}
        <ResizeHandle width={width} onChange={setWidth} className="hidden md:block" />
      </div>
    </>
  );
}
