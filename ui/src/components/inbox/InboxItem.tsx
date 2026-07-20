import clsx from "clsx";
import { AlertCircle, CheckCircle2, MessageSquare, ShieldQuestion } from "lucide-react";
import type { Event } from "../../gen/types";
import { RelativeTime } from "../common/RelativeTime";
import { summarizeEvent } from "../../lib/eventSummary";
import { FOCUS_RING } from "../../lib/constants";

const TYPE_ICON: Partial<Record<Event["type"], typeof AlertCircle>> = {
  awaiting_input: ShieldQuestion,
  turn_done: CheckCircle2,
  error: AlertCircle,
};

interface InboxItemProps {
  event: Event;
  onGo: () => void;
  onAck: () => void;
}

export function InboxItem({ event, onGo, onAck }: InboxItemProps) {
  const Icon = TYPE_ICON[event.type] ?? MessageSquare;
  return (
    <div className="flex items-start gap-2 rounded-md py-1 text-xs">
      <Icon size={13} className="mt-0.5 shrink-0 text-ink-faint" />
      <span className="min-w-0 flex-1 truncate text-ink-muted">{summarizeEvent(event)}</span>
      <RelativeTime iso={event.created_at} className="shrink-0 text-ink-faint" />
      <div className="flex shrink-0 items-center gap-1">
        <button
          type="button"
          onClick={onGo}
          className={clsx(
            "min-h-11 rounded px-2.5 text-ink-faint hover:bg-hover hover:text-ink md:min-h-0 md:px-1.5 md:py-0.5",
            FOCUS_RING,
          )}
        >
          Go
        </button>
        <button
          type="button"
          onClick={onAck}
          className={clsx(
            "min-h-11 rounded px-2.5 text-ink-faint hover:bg-hover hover:text-accent md:min-h-0 md:px-1.5 md:py-0.5",
            FOCUS_RING,
          )}
        >
          Ack
        </button>
      </div>
    </div>
  );
}
