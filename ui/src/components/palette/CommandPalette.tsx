import { useEffect, useMemo, useState } from "react";
import { Command } from "cmdk";
import { useLocation, useNavigate } from "react-router";
import { useShallow } from "zustand/react/shallow";
import { CheckCheck, Inbox as InboxIcon, Plus } from "lucide-react";
import { useTreeStore } from "../../state/tree";
import { selectInboxEvents, useInboxStore } from "../../state/inbox";
import { apiClient } from "../../state/api";
import { KindIcon } from "../tree/KindIcon";
import { CHILD_KIND_FOR } from "../../lib/constants";

const ITEM_CLASS =
  "flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs text-ink aria-selected:bg-hover";

export function CommandPalette() {
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState<"root" | "create">("root");
  const [search, setSearch] = useState("");
  const navigate = useNavigate();
  const location = useLocation();

  const nodesById = useTreeStore((s) => s.nodesById);
  const rootId = useTreeStore((s) => s.rootId);
  // selectInboxEvents allocates a new array each call; useShallow keeps the
  // subscription stable (a fresh-but-equal array would otherwise re-render
  // in a loop -- zustand/useSyncExternalStore treats a new reference as a
  // change even when every element is identical).
  const inboxEvents = useInboxStore(useShallow(selectInboxEvents));

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((v) => !v);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  useEffect(() => {
    setOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    if (!open) {
      setMode("root");
      setSearch("");
    }
  }, [open]);

  const nodeEntries = useMemo(() => {
    const pathCache = new Map<string, string>();
    function pathFor(id: string): string {
      const cached = pathCache.get(id);
      if (cached) return cached;
      const node = nodesById[id];
      if (!node) return "";
      const path = node.parent_id ? `${pathFor(node.parent_id)} / ${node.title}` : node.title;
      pathCache.set(id, path);
      return path;
    }
    return Object.values(nodesById)
      .filter((n) => !n.archived_at)
      .map((n) => ({ node: n, path: pathFor(n.id) }));
  }, [nodesById]);

  const currentNodeId = location.pathname.startsWith("/n/") ? location.pathname.slice(3) : rootId;
  const currentKind = currentNodeId ? nodesById[currentNodeId]?.kind : undefined;
  const childKind = currentKind ? CHILD_KIND_FOR[currentKind] : null;

  async function handleCreateSubmit() {
    const title = search.trim();
    if (!title || !currentNodeId || !childKind) return;
    const created = await apiClient.createNode({ parent_id: currentNodeId, kind: childKind, title });
    setOpen(false);
    navigate(`/n/${created.id}`);
  }

  async function ackAll() {
    const nodeIds = new Set(inboxEvents.map((e) => e.node_id));
    const inbox = useInboxStore.getState();
    nodeIds.forEach((id) => inbox.ackNodeOptimistic(id));
    await Promise.all([...nodeIds].map((id) => apiClient.ackNode(id).catch(() => {})));
    setOpen(false);
  }

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-canvas/70 pt-[15vh] backdrop-blur-sm"
      onClick={() => setOpen(false)}
    >
      <Command
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-lg overflow-hidden rounded-lg border border-border-strong bg-surface-2 shadow-popover"
      >
        <div className="flex items-center gap-2 border-b border-border px-3">
          <Command.Input
            autoFocus
            value={search}
            onValueChange={setSearch}
            onKeyDown={(e) => {
              if (mode === "create") {
                if (e.key === "Enter") {
                  e.preventDefault();
                  void handleCreateSubmit();
                } else if (e.key === "Escape") {
                  e.preventDefault();
                  setMode("root");
                  setSearch("");
                }
              } else if (e.key === "Escape") {
                setOpen(false);
              }
            }}
            placeholder={mode === "create" ? `Title for the new ${childKind}…` : "Jump to a node, or run a command…"}
            className="w-full bg-transparent py-2.5 text-sm text-ink outline-none placeholder:text-ink-faint"
          />
        </div>

        {mode === "create" ? (
          <div className="px-3 py-6 text-center text-2xs text-ink-faint">
            Press <span className="text-ink-muted">Enter</span> to create · <span className="text-ink-muted">Esc</span> to go back
          </div>
        ) : (
          <Command.List className="max-h-80 overflow-y-auto p-1.5">
            <Command.Empty className="px-3 py-6 text-center text-xs text-ink-faint">No matches.</Command.Empty>

            <Command.Group heading="Actions">
              <Command.Item
                value="go to inbox"
                onSelect={() => {
                  setOpen(false);
                  navigate("/inbox");
                }}
                className={ITEM_CLASS}
              >
                <InboxIcon size={13} className="text-ink-faint" />
                Go to inbox
                {inboxEvents.length > 0 && <span className="ml-auto text-ink-faint">{inboxEvents.length}</span>}
              </Command.Item>
              {childKind && (
                <Command.Item
                  value={`new ${childKind} under current node`}
                  onSelect={() => setMode("create")}
                  className={ITEM_CLASS}
                >
                  <Plus size={13} className="text-ink-faint" />
                  New {childKind} under current node
                </Command.Item>
              )}
              {inboxEvents.length > 0 && (
                <Command.Item value="ack all" onSelect={() => void ackAll()} className={ITEM_CLASS}>
                  <CheckCheck size={13} className="text-ink-faint" />
                  Ack all ({inboxEvents.length})
                </Command.Item>
              )}
            </Command.Group>

            <Command.Group heading="Nodes">
              {nodeEntries.map(({ node, path }) => (
                <Command.Item
                  key={node.id}
                  value={path}
                  onSelect={() => {
                    setOpen(false);
                    navigate(`/n/${node.id}`);
                  }}
                  className={ITEM_CLASS}
                >
                  <KindIcon kind={node.kind} size={13} className="shrink-0 text-ink-faint" />
                  <span className="truncate">{path}</span>
                </Command.Item>
              ))}
            </Command.Group>
          </Command.List>
        )}
      </Command>
    </div>
  );
}
