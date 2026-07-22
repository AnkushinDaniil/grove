import { useMemo } from "react";
import clsx from "clsx";
import { useTreeStore } from "../../state/tree";
import { FOCUS_RING } from "../../lib/constants";
import type { NodeID } from "../../gen/types";

interface ScopePickerProps {
  value: NodeID | "";
  onChange: (scope: NodeID | "") => void;
  className?: string;
}

interface Row {
  id: NodeID;
  depth: number;
  title: string;
}

function flattenWithDepth(rootId: NodeID | null, childrenByParent: Record<NodeID, NodeID[]>, nodesById: Record<NodeID, { title: string }>): Row[] {
  if (!rootId) return [];
  const rows: Row[] = [];
  function walk(id: NodeID, depth: number) {
    const node = nodesById[id];
    if (!node) return;
    rows.push({ id, depth, title: node.title });
    for (const child of childrenByParent[id] ?? []) walk(child, depth + 1);
  }
  walk(rootId, 0);
  return rows;
}

/** Whole-workspace-by-default subtree scope for the stats dashboard. A
 *  native select: fully keyboard/screen-reader accessible out of the box,
 *  and the node tree here is small enough that a search combobox (like
 *  DirCombobox, built for a different job -- filesystem completion) would
 *  be more machinery than the picker needs. */
export function ScopePicker({ value, onChange, className }: ScopePickerProps) {
  const rootId = useTreeStore((s) => s.rootId);
  const childrenByParent = useTreeStore((s) => s.childrenByParent);
  const nodesById = useTreeStore((s) => s.nodesById);

  const rows = useMemo(() => flattenWithDepth(rootId, childrenByParent, nodesById), [rootId, childrenByParent, nodesById]);

  return (
    <select
      aria-label="Scope"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={clsx(
        "rounded-md border border-border-strong bg-surface-2 px-2 py-1 text-2xs text-ink-muted hover:text-ink",
        FOCUS_RING,
        className,
      )}
    >
      <option value="">Whole workspace</option>
      {rows
        .filter((r) => r.id !== rootId)
        .map((r) => (
          <option key={r.id} value={r.id}>
            {"  ".repeat(r.depth - 1)}
            {r.title}
          </option>
        ))}
    </select>
  );
}
