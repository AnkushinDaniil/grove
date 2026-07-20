import { useState } from "react";
import clsx from "clsx";
import {
  AlertCircle,
  CheckCircle2,
  ChevronRight,
  CircleUser,
  MessageSquare,
  Play,
  ShieldQuestion,
  Square,
  Wrench,
  Zap,
} from "lucide-react";
import type { Event } from "../../../gen/types";
import { RelativeTime } from "../../common/RelativeTime";
import { summarizeEvent } from "../../../lib/eventSummary";
import type { LucideIcon } from "lucide-react";

const TYPE_ICON: Record<Event["type"], LucideIcon> = {
  session_started: Play,
  text: MessageSquare,
  tool_call: Wrench,
  tool_result: CheckCircle2,
  awaiting_input: ShieldQuestion,
  turn_done: CheckCircle2,
  session_ended: Square,
  error: AlertCircle,
  usage: Zap,
};

interface EventRowProps {
  event: Event;
}

export function EventRow({ event }: EventRowProps) {
  const [expanded, setExpanded] = useState(false);
  // Injected user prompts get a distinct icon + accent tint so a headless
  // conversation reads as "you said / it said" at a glance, without a full
  // chat-bubble treatment -- everything else keeps the neutral styling.
  const isUserText = event.type === "text" && event.payload.role === "user";
  const Icon = isUserText ? CircleUser : TYPE_ICON[event.type];

  return (
    <li className="rounded-md">
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-hover"
      >
        <ChevronRight
          size={11}
          className={clsx("mt-1 shrink-0 text-ink-faint transition-transform", expanded && "rotate-90")}
        />
        <Icon size={13} className={clsx("mt-0.5 shrink-0", isUserText ? "text-accent" : "text-ink-faint")} />
        <span className={clsx("min-w-0 flex-1 truncate", isUserText ? "text-ink" : "text-ink-muted")}>
          {summarizeEvent(event)}
        </span>
        <span className="shrink-0 rounded border border-border-strong px-1 py-px text-[10px] uppercase tracking-wide text-ink-disabled">
          {event.type}
        </span>
        <RelativeTime iso={event.created_at} className="shrink-0 text-ink-faint" />
      </button>
      {expanded && (
        <pre className="mb-1.5 ml-11 mr-2 overflow-x-auto rounded-md border border-border bg-canvas p-2 text-[10px] leading-relaxed text-ink-muted">
          {JSON.stringify(event.payload, null, 2)}
        </pre>
      )}
    </li>
  );
}
