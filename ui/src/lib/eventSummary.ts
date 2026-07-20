import type { AwaitingReason, Event } from "../gen/types";

const AWAITING_REASON_LABEL: Record<AwaitingReason, string> = {
  permission: "Needs permission",
  question: "Has a question",
  idle: "Waiting for input",
};

/** One-line human summary for an event -- shared by the inbox and the
 *  Events tab's collapsed row. */
export function summarizeEvent(event: Event): string {
  switch (event.type) {
    case "session_started":
      return event.payload.model ? `Session started (${event.payload.model})` : "Session started";
    case "text":
      return event.payload.role === "user" ? `You: ${event.payload.text}` : event.payload.text;
    case "tool_call":
      return event.payload.input_summary
        ? `${event.payload.name}: ${event.payload.input_summary}`
        : event.payload.name;
    case "tool_result":
      return event.payload.summary
        ? `${event.payload.name} ${event.payload.ok ? "ok" : "failed"}: ${event.payload.summary}`
        : `${event.payload.name} ${event.payload.ok ? "ok" : "failed"}`;
    case "awaiting_input":
      return event.payload.detail || AWAITING_REASON_LABEL[event.payload.reason];
    case "turn_done":
      return event.payload.result_text || "Turn finished";
    case "session_ended":
      return event.payload.exit_code === 0 ? "Session ended" : `Session ended (exit ${event.payload.exit_code})`;
    case "error":
      return event.payload.message;
    case "usage":
      return `${event.payload.input_tokens.toLocaleString()} in / ${event.payload.output_tokens.toLocaleString()} out tokens`;
    default:
      return "";
  }
}
