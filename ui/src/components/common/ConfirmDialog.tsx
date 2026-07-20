import { useEffect, useRef } from "react";
import clsx from "clsx";
import { FOCUS_RING } from "../../lib/constants";

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  description?: string;
  confirmLabel?: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

/** Generic confirmation modal -- used by node archive, and reusable for any
 *  future destructive action. */
export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = "Confirm",
  danger,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const confirmRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (open) confirmRef.current?.focus();
  }, [open]);

  useEffect(() => {
    if (!open) return;
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") onCancel();
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open, onCancel]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-canvas/70 backdrop-blur-sm"
      role="presentation"
      onClick={onCancel}
    >
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="confirm-dialog-title"
        className="w-full max-w-sm rounded-lg border border-border-strong bg-surface-2 p-4 shadow-popover"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 id="confirm-dialog-title" className="font-sans text-sm font-medium text-ink">
          {title}
        </h2>
        {description && <p className="mt-1.5 font-sans text-xs text-ink-faint">{description}</p>}
        <div className="mt-4 flex justify-end gap-2">
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
            ref={confirmRef}
            type="button"
            onClick={onConfirm}
            className={clsx(
              "rounded-md px-2.5 py-1.5 text-xs font-medium",
              danger
                ? "bg-danger text-white hover:bg-danger-strong"
                : "bg-accent text-accent-ink hover:bg-accent-strong",
              FOCUS_RING,
            )}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
