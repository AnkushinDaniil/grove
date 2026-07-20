import { useState } from "react";
import { useNavigate } from "react-router";
import { ListTree, Plus } from "lucide-react";
import { useTreeStore } from "../../../state/tree";
import { apiClient } from "../../../state/api";
import { CHILD_KIND_FOR } from "../../../lib/constants";
import { ChildRow } from "./ChildRow";
import { InlineCreateRow } from "../../tree/InlineCreateRow";
import { EmptyState } from "../../common/EmptyState";
import type { Node } from "../../../gen/types";

interface ChildrenTabProps {
  node: Node;
}

export function ChildrenTab({ node }: ChildrenTabProps) {
  const childIds = useTreeStore((s) => s.childrenByParent[node.id]) ?? [];
  const navigate = useNavigate();
  const [creating, setCreating] = useState(false);
  const childKind = CHILD_KIND_FOR[node.kind];

  async function submit(title: string) {
    setCreating(false);
    if (!childKind) return;
    const created = await apiClient.createNode({ parent_id: node.id, kind: childKind, title }).catch(() => null);
    if (created) navigate(`/n/${created.id}`);
  }

  return (
    <div className="h-full overflow-y-auto p-2">
      {childIds.length === 0 && !creating && (
        <EmptyState
          icon={<ListTree size={28} strokeWidth={1.5} />}
          title="No children yet"
          description={childKind ? `Add a ${childKind} to get started.` : undefined}
        />
      )}
      {childIds.length > 0 && (
        <ul className="space-y-0.5">
          {childIds.map((id) => (
            <ChildRow key={id} nodeId={id} onOpen={() => navigate(`/n/${id}`)} />
          ))}
        </ul>
      )}
      {creating ? (
        <InlineCreateRow
          indentPx={4}
          placeholder={`New ${childKind}…`}
          onSubmit={(title) => void submit(title)}
          onCancel={() => setCreating(false)}
        />
      ) : (
        childKind && (
          <button
            type="button"
            onClick={() => setCreating(true)}
            className="mt-1 flex w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-xs text-ink-faint hover:bg-hover hover:text-ink"
          >
            <Plus size={13} />
            New {childKind}
          </button>
        )
      )}
    </div>
  );
}
