import { useEffect, useRef, useState } from "react";
import clsx from "clsx";
import { FOCUS_RING } from "../../lib/constants";

interface InlineCreateRowProps {
  /** Left padding in px so the input can align with tree-rail indentation;
   *  callers outside the rail just omit it. */
  indentPx?: number;
  placeholder: string;
  onSubmit: (title: string) => void;
  onCancel: () => void;
}

/** Lightweight inline text input for "new project" / "new subtask"
 *  affordances -- no modal, submits on Enter or blur-with-text, cancels on
 *  Escape or blur-while-empty. */
export function InlineCreateRow({ indentPx = 0, placeholder, onSubmit, onCancel }: InlineCreateRowProps) {
  const [value, setValue] = useState("");
  const inputRef = useRef<HTMLInputElement | null>(null);
  const settledRef = useRef(false);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  function settle() {
    if (settledRef.current) return;
    settledRef.current = true;
    const trimmed = value.trim();
    if (trimmed) onSubmit(trimmed);
    else onCancel();
  }

  return (
    <div className="flex items-center gap-1.5 py-1 pr-2" style={{ paddingLeft: indentPx }}>
      <input
        ref={inputRef}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            settle();
          } else if (e.key === "Escape") {
            e.preventDefault();
            settledRef.current = true;
            onCancel();
          }
        }}
        onBlur={settle}
        placeholder={placeholder}
        className={clsx(
          "w-full rounded-md border border-accent/40 bg-surface-2 px-1.5 py-0.5 text-xs text-ink placeholder:text-ink-faint",
          FOCUS_RING,
        )}
      />
    </div>
  );
}
