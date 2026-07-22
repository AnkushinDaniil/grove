import { useState } from "react";
import clsx from "clsx";
import { Archive, Check, Play, Plus, Square, Terminal as TerminalIcon } from "lucide-react";
import { apiClient } from "../../state/api";
import { useInboxStore } from "../../state/inbox";
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
  primary?: boolean;
  disabled?: boolean;
  title?: string;
}

function ActionButton({ icon: Icon, label, onClick, danger, primary, disabled, title }: ActionButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={clsx(
        "flex min-h-11 items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-xs transition-colors disabled:pointer-events-none disabled:opacity-40 md:min-h-0",
        primary
          ? "border-accent bg-accent font-medium text-accent-ink hover:bg-accent-strong"
          : "border-border-strong text-ink-muted hover:bg-hover hover:text-ink",
        danger && "border-border-strong text-ink-muted hover:border-danger/50 hover:bg-danger-soft hover:text-danger",
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
    useInboxStore.getState().ackNodeOptimistic(node.id);
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
          <ActionButton
            icon={TerminalIcon}
            label="Start session"
            onClick={onStartPty}
            disabled={busy}
            primary
            title="Open an interactive terminal — attach and type, like running the CLI yourself"
          />
          <ActionButton
            icon={Play}
            label="Run headless…"
            onClick={onOpenHeadless}
            disabled={busy}
            title="Give a prompt and let the agent work autonomously in the background — no terminal to type in; watch progress in the Events tab"
          />
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
