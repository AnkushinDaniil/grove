import { useEffect } from "react";
import { useLatestRef } from "./useLatestRef";
import type { NodeID } from "../gen/types";

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable;
}

export interface UseKeyboardNavOptions {
  enabled: boolean;
  visibleIds: NodeID[];
  focusedId: NodeID | null;
  setFocusedId: (id: NodeID) => void;
  isExpanded: (id: NodeID) => boolean;
  hasChildren: (id: NodeID) => boolean;
  parentOf: (id: NodeID) => NodeID | null;
  toggleExpanded: (id: NodeID, expand?: boolean) => void;
  onOpen: (id: NodeID) => void;
  onAck: (id: NodeID) => void;
  onNewChild: (id: NodeID) => void;
}

/**
 * Global j/k (or arrow) list navigation, h/l collapse/expand, Enter to
 * open, `a` to ack, `n` for a new child -- suppressed while an editable
 * element (input/textarea/contenteditable) has focus, so it never steals
 * keystrokes from the prompt bar, search box, or rename field.
 */
export function useKeyboardNav(options: UseKeyboardNavOptions): void {
  const optionsRef = useLatestRef(options);

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      const opts = optionsRef.current;
      if (!opts.enabled) return;
      if (isEditableTarget(e.target)) return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;

      const { visibleIds, focusedId, setFocusedId } = opts;
      if (visibleIds.length === 0) return;
      const currentIndex = focusedId ? visibleIds.indexOf(focusedId) : -1;

      switch (e.key) {
        case "j":
        case "ArrowDown": {
          e.preventDefault();
          const idx = Math.min(visibleIds.length - 1, Math.max(0, currentIndex + 1));
          setFocusedId(visibleIds[idx]);
          break;
        }
        case "k":
        case "ArrowUp": {
          e.preventDefault();
          const idx = currentIndex === -1 ? 0 : Math.max(0, currentIndex - 1);
          setFocusedId(visibleIds[idx]);
          break;
        }
        case "l":
        case "ArrowRight": {
          if (!focusedId) break;
          e.preventDefault();
          if (opts.hasChildren(focusedId) && !opts.isExpanded(focusedId)) {
            opts.toggleExpanded(focusedId, true);
          } else if (opts.hasChildren(focusedId)) {
            const child = visibleIds[visibleIds.indexOf(focusedId) + 1];
            if (child) setFocusedId(child);
          }
          break;
        }
        case "h":
        case "ArrowLeft": {
          if (!focusedId) break;
          e.preventDefault();
          if (opts.hasChildren(focusedId) && opts.isExpanded(focusedId)) {
            opts.toggleExpanded(focusedId, false);
          } else {
            const parent = opts.parentOf(focusedId);
            if (parent) setFocusedId(parent);
          }
          break;
        }
        case "Enter": {
          if (!focusedId) break;
          e.preventDefault();
          opts.onOpen(focusedId);
          break;
        }
        case "a": {
          if (!focusedId) break;
          e.preventDefault();
          opts.onAck(focusedId);
          break;
        }
        case "n": {
          if (!focusedId) break;
          e.preventDefault();
          opts.onNewChild(focusedId);
          break;
        }
        default:
          break;
      }
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [optionsRef]);
}
