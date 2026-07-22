import { useEffect, useState } from "react";
import clsx from "clsx";
import { Brain, Database } from "lucide-react";
import { apiClient } from "../../../state/api";
import { FOCUS_RING } from "../../../lib/constants";
import { RelativeTime } from "../../common/RelativeTime";
import { EmptyState } from "../../common/EmptyState";
import type { MemoryEntry, MemoryKind, MemoryResponse, MemoryScope, MemorySource, NodeID } from "../../../gen/types";

// Read-only view of a node's MemPalace-backed memory (docs/API.md, Node memory).
// Agents write memory via MCP; users browse it here. The scope switcher maps to
// the ?scope= query: the node itself, its subtree, or its ancestor chain.

const SCOPES: { id: MemoryScope; label: string; hint: string }[] = [
  { id: "self", label: "This node", hint: "Memory filed against this node" },
  { id: "subtree", label: "Subtree", hint: "This node and everything below it" },
  { id: "ancestors", label: "Ancestors", hint: "This node and the project above it" },
];

const KIND_STYLE: Record<MemoryKind, { label: string; className: string }> = {
  decision: { label: "Decision", className: "text-accent bg-accent-soft border-accent/30" },
  gotcha: { label: "Gotcha", className: "text-status-failed bg-status-failed/10 border-status-failed/40" },
  convention: { label: "Convention", className: "text-status-done bg-status-done/10 border-status-done/30" },
  fact: { label: "Fact", className: "text-ink-muted bg-surface-2 border-border" },
};

const SOURCE_LABEL: Record<MemorySource, string> = {
  auto: "auto-captured",
  agent: "by agent",
  user: "by you",
};

interface MemoryTabProps {
  nodeId: NodeID;
}

export function MemoryTab({ nodeId }: MemoryTabProps) {
  const [scope, setScope] = useState<MemoryScope>("self");
  const [data, setData] = useState<MemoryResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setData(null);
    setError(null);
    apiClient
      .getNodeMemory(nodeId, scope)
      .then((res) => {
        if (!cancelled) setData(res);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [nodeId, scope]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-1 border-b border-border px-3 py-2">
        <span className="mr-1 text-2xs font-medium text-ink-faint">Scope</span>
        {SCOPES.map((s) => (
          <button
            key={s.id}
            type="button"
            title={s.hint}
            onClick={() => setScope(s.id)}
            className={clsx(
              "rounded-md px-2 py-1 text-2xs font-medium transition-colors",
              scope === s.id ? "bg-surface-2 text-ink" : "text-ink-faint hover:text-ink",
              FOCUS_RING,
            )}
          >
            {s.label}
          </button>
        ))}
        {data?.healthy && data.backend && (
          <span className="ml-auto inline-flex items-center gap-1 text-2xs text-ink-faint">
            <Database size={11} />
            {data.backend}
          </span>
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <MemoryBody data={data} error={error} />
      </div>
    </div>
  );
}

function MemoryBody({ data, error }: { data: MemoryResponse | null; error: string | null }) {
  if (error) {
    return (
      <EmptyState
        icon={<Database size={28} strokeWidth={1.5} />}
        title="Couldn't load memory"
        description={error}
      />
    );
  }
  if (data === null) {
    return <div className="p-4 text-2xs text-ink-faint">Loading memory…</div>;
  }
  if (!data.healthy) {
    return <MemoryUnavailable />;
  }
  if (data.entries.length === 0) {
    return (
      <EmptyState
        icon={<Brain size={28} strokeWidth={1.5} />}
        title="No memory yet"
        description="Decisions, gotchas and conventions captured for this node will appear here as agents work."
      />
    );
  }
  return (
    <ul className="space-y-2 p-3">
      {data.entries.map((entry) => (
        <MemoryEntryRow key={entry.id} entry={entry} />
      ))}
    </ul>
  );
}

function MemoryUnavailable() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-12 text-center">
      <div className="text-ink-faint">
        <Database size={28} strokeWidth={1.5} />
      </div>
      <div className="space-y-1">
        <p className="font-sans text-sm font-medium text-ink">MemPalace unavailable</p>
        <p className="max-w-sm font-sans text-xs text-ink-faint">
          Memory recall and capture are off. Install the backend to enable them:
        </p>
      </div>
      <code className="rounded-md border border-border bg-surface-2 px-2.5 py-1 font-mono text-2xs text-ink-muted">
        grove memory install
      </code>
    </div>
  );
}

function MemoryEntryRow({ entry }: { entry: MemoryEntry }) {
  const kind = KIND_STYLE[entry.kind] ?? KIND_STYLE.fact;
  return (
    <li className="rounded-md border border-border bg-surface p-3">
      <div className="mb-1.5 flex items-center gap-2">
        <span
          className={clsx(
            "inline-flex items-center rounded-md border px-1.5 py-0.5 text-2xs font-medium uppercase tracking-wide",
            kind.className,
          )}
        >
          {kind.label}
        </span>
        <span className="text-2xs text-ink-faint">{SOURCE_LABEL[entry.source] ?? entry.source}</span>
        <RelativeTime iso={entry.created_at} className="ml-auto text-2xs text-ink-faint" />
      </div>
      <p className="whitespace-pre-wrap break-words font-sans text-xs text-ink-muted">{entry.content}</p>
    </li>
  );
}
