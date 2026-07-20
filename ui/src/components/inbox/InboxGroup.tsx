import { KindIcon } from "../tree/KindIcon";
import { InboxItem } from "./InboxItem";
import type { Event, Node } from "../../gen/types";

interface InboxGroupProps {
  id: string;
  index: number;
  node: Node | undefined;
  events: Event[];
  onGo: () => void;
  onAck: () => void;
}

export function InboxGroup({ id, index, node, events, onGo, onAck }: InboxGroupProps) {
  return (
    <div id={id} className="px-5 py-3">
      <div className="mb-1.5 flex items-center gap-2">
        <span className="flex h-4 w-4 shrink-0 items-center justify-center rounded border border-border-strong text-[10px] text-ink-faint">
          {index + 1}
        </span>
        {node && <KindIcon kind={node.kind} size={13} className="text-ink-faint" />}
        <button
          type="button"
          onClick={onGo}
          className="truncate text-xs text-ink underline-offset-2 hover:text-accent hover:underline"
        >
          {node?.title ?? "Unknown node"}
        </button>
      </div>
      <div className="space-y-1 pl-6">
        {events.map((event) => (
          <InboxItem key={event.id} event={event} onGo={onGo} onAck={onAck} />
        ))}
      </div>
    </div>
  );
}
