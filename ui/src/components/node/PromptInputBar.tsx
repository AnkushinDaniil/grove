import { useState } from "react";
import clsx from "clsx";
import { Send } from "lucide-react";
import { apiClient } from "../../state/api";
import { FOCUS_RING } from "../../lib/constants";
import type { NodeID } from "../../gen/types";

interface PromptInputBarProps {
  nodeId: NodeID;
}

/** Bottom bar on the Terminal tab, POSTs /nodes/{id}/prompt. Most useful
 *  for headless sessions (no attached terminal to type into directly), and
 *  a quick one-shot message even when a pty is attached. */
export function PromptInputBar({ nodeId }: PromptInputBarProps) {
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);

  async function submit() {
    const trimmed = text.trim();
    if (!trimmed || sending) return;
    setSending(true);
    try {
      await apiClient.sendPrompt(nodeId, trimmed);
      setText("");
    } catch {
      // Leave the text in place so the user can retry.
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="flex shrink-0 items-center gap-2 border-t border-border bg-surface px-3 py-2">
      <input
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            void submit();
          }
        }}
        placeholder="Send a message to this session…"
        className={clsx(
          "min-h-11 min-w-0 flex-1 rounded-md border border-border bg-canvas px-2.5 py-1.5 font-sans text-xs text-ink placeholder:text-ink-faint md:min-h-0",
          FOCUS_RING,
        )}
      />
      <button
        type="button"
        onClick={() => void submit()}
        disabled={!text.trim() || sending}
        className={clsx(
          "flex min-h-11 items-center gap-1.5 rounded-md bg-accent px-3 text-xs font-medium text-accent-ink hover:bg-accent-strong disabled:opacity-40 md:min-h-0 md:px-2.5 md:py-1.5",
          FOCUS_RING,
        )}
      >
        <Send size={12} />
        Send
      </button>
    </div>
  );
}
