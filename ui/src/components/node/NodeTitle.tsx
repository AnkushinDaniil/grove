import { useEffect, useRef, useState } from "react";
import clsx from "clsx";
import { apiClient } from "../../state/api";
import { FOCUS_RING } from "../../lib/constants";
import type { Node } from "../../gen/types";

interface NodeTitleProps {
  node: Node;
}

/** Renders the node title as plain text; double-click swaps in an editable
 *  input that PATCHes /nodes/{id} on commit. */
export function NodeTitle({ node }: NodeTitleProps) {
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(node.title);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!editing) setValue(node.title);
  }, [node.title, editing]);

  useEffect(() => {
    if (editing) {
      inputRef.current?.focus();
      inputRef.current?.select();
    }
  }, [editing]);

  async function commit() {
    const trimmed = value.trim();
    setEditing(false);
    if (!trimmed || trimmed === node.title) {
      setValue(node.title);
      return;
    }
    await apiClient.patchNode(node.id, { title: trimmed }).catch(() => setValue(node.title));
  }

  if (editing) {
    return (
      <input
        ref={inputRef}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onBlur={() => void commit()}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            void commit();
          } else if (e.key === "Escape") {
            e.preventDefault();
            setValue(node.title);
            setEditing(false);
          }
        }}
        className={clsx(
          "min-w-0 flex-1 rounded-md border border-accent/50 bg-surface-2 px-1.5 py-0.5 text-lg font-semibold text-ink",
          FOCUS_RING,
        )}
      />
    );
  }

  return (
    <h1
      onDoubleClick={() => setEditing(true)}
      title="Double-click to rename"
      className="min-w-0 flex-1 truncate text-lg font-semibold text-ink"
    >
      {node.title}
    </h1>
  );
}
