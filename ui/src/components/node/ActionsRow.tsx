import { useState } from "react";
import clsx from "clsx";
import { Archive, Check, Play, Plus, Square, Terminal as TerminalIcon } from "lucide-react";
import { apiClient } from "../../state/api";
import { CHILD_KIND_FOR, FOCUS_RING } from "../../lib/constants";
import { ConfirmDialog } from "../common/ConfirmDialog";
import { InlineCreateRow } from "../tree/InlineCreateRow";
import type { LucideIcon } from "lucide-react";
import type { Node, Session } from "../../gen/types";

interface ActionButtonProps {
  icon: LucideIcon;
  label: string;
  onClick: () => void;
  danger?: boolean;
  disabled?: boolean;
}

function ActionButton({ icon: Icon, label, onClick, danger, disabled }: ActionButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={clsx(
        "flex min-h-11 items-center gap-1.5 rounded-md border border-border-strong px-2.5 py-1.5 text-xs text-ink-muted transition-colors hover:bg-hover hover:text-ink disabled:pointer-events-none disabled:opacity-40 md:min-h-0",
        danger && "hover:border-danger/50 hover:bg-danger-soft hover:text-danger",
        FOCUS_RING,
      )}
    >
      <Icon size={13} />
      {label}
    </button>
  );
}

interface ActionsRowProps {
  node: Node;
  activeSession: Session | undefined;
  busy: boolean;
  onStartPty: () => void;
  onOpenHeadless: () => void;
  onStopSession: () => void;
}

export function ActionsRow({ node, activeSession, busy, onStartPty, onOpenHeadless, onStopSession }: ActionsRowProps) {
  const [archiveOpen, setArchiveOpen] = useState(false);
  const [creatingChild, setCreatingChild] = useState(false);
  const childKind = CHILD_KIND_FOR[node.kind];

  async function ack() {
    await apiClient.ackNode(node.id).catch(() => {
      // Best-effort; the chip stays put and the user can retry.
    });
  }

  async function submitChild(title: string) {
    setCreatingChild(false);
    if (!childKind) return;
    await apiClient.createNode({ parent_id: node.id, kind: childKind, title }).catch(() => {});
  }

  async function archive() {
    setArchiveOpen(false);
    await apiClient.archiveNode(node.id).catch(() => {});
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {activeSession ? (
        <ActionButton icon={Square} label="Stop" onClick={onStopSession} disabled={busy} danger />
      ) : (
        <>
          <ActionButton icon={TerminalIcon} label="Start PTY session" onClick={onStartPty} disabled={busy} />
          <ActionButton icon={Play} label="Start headless…" onClick={onOpenHeadless} disabled={busy} />
        </>
      )}
      {node.attention !== "none" && <ActionButton icon={Check} label="Ack" onClick={() => void ack()} />}
      {childKind && (
        <ActionButton icon={Plus} label={`New ${childKind}`} onClick={() => setCreatingChild(true)} />
      )}
      {node.kind !== "workspace" && (
        <ActionButton icon={Archive} label="Archive…" onClick={() => setArchiveOpen(true)} danger />
      )}

      {creatingChild && childKind && (
        <div className="basis-full">
          <InlineCreateRow
            placeholder={`New ${childKind}…`}
            onSubmit={(title) => void submitChild(title)}
            onCancel={() => setCreatingChild(false)}
          />
        </div>
      )}

      <ConfirmDialog
        open={archiveOpen}
        title={`Archive "${node.title}"?`}
        description="This also archives every descendant. Worktrees are kept, never silently deleted."
        confirmLabel="Archive"
        danger
        onConfirm={() => void archive()}
        onCancel={() => setArchiveOpen(false)}
      />
    </div>
  );
}
