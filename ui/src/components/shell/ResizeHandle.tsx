import { useCallback, useRef } from "react";
import clsx from "clsx";

const MIN_WIDTH = 240;
const MAX_WIDTH = 320;

interface ResizeHandleProps {
  width: number;
  onChange: (width: number) => void;
  className?: string;
}

/** Drag handle on the tree rail's trailing edge, clamped to [240, 320]. */
export function ResizeHandle({ width, onChange, className }: ResizeHandleProps) {
  const startRef = useRef<{ x: number; width: number } | null>(null);

  const onPointerMove = useCallback(
    (e: PointerEvent) => {
      const start = startRef.current;
      if (!start) return;
      const next = Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, start.width + (e.clientX - start.x)));
      onChange(next);
    },
    [onChange],
  );

  const onPointerUp = useCallback(() => {
    startRef.current = null;
    window.removeEventListener("pointermove", onPointerMove);
    window.removeEventListener("pointerup", onPointerUp);
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  }, [onPointerMove]);

  const onPointerDown = useCallback(
    (e: React.PointerEvent) => {
      startRef.current = { x: e.clientX, width };
      window.addEventListener("pointermove", onPointerMove);
      window.addEventListener("pointerup", onPointerUp);
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
    },
    [width, onPointerMove, onPointerUp],
  );

  return (
    <div
      onPointerDown={onPointerDown}
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize tree rail"
      className={clsx(
        "group absolute inset-y-0 right-0 z-10 w-1.5 -translate-x-1/2 cursor-col-resize touch-none",
        className,
      )}
    >
      <div className="h-full w-px bg-transparent group-hover:bg-accent/50" />
    </div>
  );
}
