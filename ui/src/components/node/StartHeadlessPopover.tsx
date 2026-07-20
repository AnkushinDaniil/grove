import { useEffect, useRef, useState } from "react";
import clsx from "clsx";
import { FOCUS_RING } from "../../lib/constants";

interface StartHeadlessPopoverProps {
  onStart: (prompt: string) => void;
  onCancel: () => void;
}

/** Inline panel (not a floating-positioned popover -- simpler and always
 *  responsive) for the initial prompt of a headless run. */
export function StartHeadlessPopover({ onStart, onCancel }: StartHeadlessPopoverProps) {
  const [prompt, setPrompt] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  return (
    <div className="w-full rounded-lg border border-border-strong bg-surface-2 p-3 shadow-panel">
      <label className="mb-1.5 block text-2xs font-medium text-ink-faint" htmlFor="headless-prompt">
        Initial prompt
      </label>
      <textarea
        id="headless-prompt"
        ref={textareaRef}
        value={prompt}
        onChange={(e) => setPrompt(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            if (prompt.trim()) onStart(prompt.trim());
          } else if (e.key === "Escape") {
            onCancel();
          }
        }}
        rows={3}
        placeholder="What should the agent do?"
        className={clsx(
          "w-full resize-none rounded-md border border-border bg-canvas px-2 py-1.5 font-sans text-xs text-ink placeholder:text-ink-faint",
          FOCUS_RING,
        )}
      />
      <div className="mt-2 flex items-center justify-end gap-2">
        <span className="mr-auto text-2xs text-ink-disabled">⌘Enter to start</span>
        <button
          type="button"
          onClick={onCancel}
          className={clsx(
            "rounded-md px-2.5 py-1.5 text-xs text-ink-muted hover:bg-hover hover:text-ink",
            FOCUS_RING,
          )}
        >
          Cancel
        </button>
        <button
          type="button"
          disabled={!prompt.trim()}
          onClick={() => onStart(prompt.trim())}
          className={clsx(
            "rounded-md bg-accent px-2.5 py-1.5 text-xs font-medium text-accent-ink hover:bg-accent-strong disabled:opacity-40",
            FOCUS_RING,
          )}
        >
          Start headless
        </button>
      </div>
    </div>
  );
}
