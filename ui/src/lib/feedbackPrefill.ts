import type { Event, FeedbackKind } from "../gen/types";

export interface FeedbackPrefill {
  kind: FeedbackKind;
  subject: string;
}

/** Derives the feedback composer's initial kind+subject from an Events-tab
 *  row. A tool_call/tool_result for the Skill tool itself reports as
 *  kind=skill (subject = the skill name, best-effort from input_summary/
 *  summary since ToolCallPayload carries no structured skill field); any
 *  other tool_call/tool_result reports as kind=tool (subject = the tool
 *  name). Every other event type has no single natural kind, so it falls
 *  back to "other" with an empty subject -- the composer's kind select still
 *  lets the reporter correct it before sending. */
export function feedbackPrefillForEvent(event: Event): FeedbackPrefill {
  if (event.type === "tool_call" || event.type === "tool_result") {
    const { name } = event.payload;
    if (name === "Skill") {
      const detail = event.type === "tool_call" ? event.payload.input_summary : event.payload.summary;
      return { kind: "skill", subject: detail?.trim() || "" };
    }
    return { kind: "tool", subject: name };
  }
  return { kind: "other", subject: "" };
}
