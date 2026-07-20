package codex

import "github.com/AnkushinDaniil/grove/internal/core"

// parseThreadStarted handles {"type":"thread.started","thread_id":...},
// codex's session/thread-start event (verified by probe, both fresh runs
// and `exec resume`: resuming re-emits thread.started with the SAME
// thread_id rather than a distinct "resumed" event, so no special-casing is
// needed here for the resume case).
func parseThreadStarted(raw streamEvent) ([]core.EventInput, error) {
	payload, err := core.MarshalPayload(core.SessionStartedPayload{DriverSessionID: raw.ThreadID})
	if err != nil {
		return nil, err
	}
	return []core.EventInput{{Type: core.EventSessionStarted, Payload: payload}}, nil
}

// parseTurnCompleted handles {"type":"turn.completed","usage":{...}}: a
// turn_done carrying the last agent_message text seen this turn, plus usage
// when token counts are present.
func (p *parser) parseTurnCompleted(raw streamEvent) ([]core.EventInput, error) {
	events, err := p.turnDoneEvent()
	if err != nil {
		return nil, err
	}
	usage, err := usageEvent(raw.Usage)
	if err != nil {
		return nil, err
	}
	return append(events, usage...), nil
}

// parseTurnFailed handles {"type":"turn.failed","error":{"message":...}}.
// The turn still completed, just unsuccessfully, so — mirroring Claude's
// non-success "result" line — this emits turn_done plus an error event
// rather than error alone.
func (p *parser) parseTurnFailed(raw streamEvent) ([]core.EventInput, error) {
	events, err := p.turnDoneEvent()
	if err != nil {
		return nil, err
	}
	msg := ""
	if raw.Error != nil {
		msg = raw.Error.Message
	}
	errPayload, err := core.MarshalPayload(core.ErrorPayload{Message: msg})
	if err != nil {
		return nil, err
	}
	return append(events, core.EventInput{Type: core.EventError, Payload: errPayload}), nil
}

// turnDoneEvent builds the turn_done event shared by turn.completed and
// turn.failed.
func (p *parser) turnDoneEvent() ([]core.EventInput, error) {
	payload, err := core.MarshalPayload(core.TurnDonePayload{ResultText: p.lastAgentText})
	if err != nil {
		return nil, err
	}
	return []core.EventInput{{Type: core.EventTurnDone, Payload: payload}}, nil
}

// usageEvent builds the optional usage event: usage is only present on some
// turn-ending events (turn.completed always carries it; turn.failed is not
// confirmed to), mirroring Claude's "only if usage is present" rule.
func usageEvent(usage *streamUsage) ([]core.EventInput, error) {
	if usage == nil {
		return nil, nil
	}
	payload, err := core.MarshalPayload(core.UsagePayload{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
	})
	if err != nil {
		return nil, err
	}
	return []core.EventInput{{Type: core.EventUsage, Payload: payload}}, nil
}

// parseItemStarted handles {"type":"item.started","item":{...}}. Only the
// two "tool execution" item types — command_execution and mcp_tool_call —
// produce a tool_call; other item types either have no started-phase
// content (agent_message, reasoning are "completed only") or are not
// mapped by this driver (see parseItemCompleted).
func parseItemStarted(raw streamEvent) ([]core.EventInput, error) {
	if raw.Item == nil {
		return nil, nil
	}
	switch raw.Item.Type {
	case "command_execution":
		return toolCallEvent("shell", truncate(raw.Item.Command, summaryLen))
	case "mcp_tool_call":
		return toolCallEvent(raw.Item.Server+"."+raw.Item.Tool, compactTruncate(raw.Item.Arguments, summaryLen))
	default:
		return nil, nil
	}
}

// parseItemCompleted handles {"type":"item.completed","item":{...}}.
// agent_message updates the turn's result-text tracker and emits text;
// command_execution/mcp_tool_call emit a tool_result (ok = status ==
// "completed"; codex also has "failed" and "declined" statuses, both
// treated as not-ok); an "error" item emits an error event.
//
// reasoning, file_change, web_search, todo_list and collab_tool_call are
// deliberately not mapped: none were in scope for this driver (grove has no
// checklist/file-diff/search-result event types to carry them), so they —
// like any unrecognized item type — fall through to the same no-op as
// garbage.
func (p *parser) parseItemCompleted(raw streamEvent) ([]core.EventInput, error) {
	if raw.Item == nil {
		return nil, nil
	}
	switch raw.Item.Type {
	case "agent_message":
		p.lastAgentText = raw.Item.Text
		payload, err := core.MarshalPayload(core.TextPayload{Text: raw.Item.Text})
		if err != nil {
			return nil, err
		}
		return []core.EventInput{{Type: core.EventText, Payload: payload}}, nil
	case "command_execution":
		return toolResultEvent(raw.Item.Status == "completed", truncate(raw.Item.AggregatedOutput, summaryLen))
	case "mcp_tool_call":
		return toolResultEvent(raw.Item.Status == "completed", compactTruncate(raw.Item.Result, summaryLen))
	case "error":
		payload, err := core.MarshalPayload(core.ErrorPayload{Message: raw.Item.Message})
		if err != nil {
			return nil, err
		}
		return []core.EventInput{{Type: core.EventError, Payload: payload}}, nil
	default:
		return nil, nil
	}
}

// parseTopLevelError handles {"type":"error","message":...}, codex's
// stream-level error event (distinct from an item-typed "error").
func parseTopLevelError(raw streamEvent) ([]core.EventInput, error) {
	payload, err := core.MarshalPayload(core.ErrorPayload{Message: raw.Message})
	if err != nil {
		return nil, err
	}
	return []core.EventInput{{Type: core.EventError, Payload: payload}}, nil
}

func toolCallEvent(name, inputSummary string) ([]core.EventInput, error) {
	payload, err := core.MarshalPayload(core.ToolCallPayload{Name: name, InputSummary: inputSummary})
	if err != nil {
		return nil, err
	}
	return []core.EventInput{{Type: core.EventToolCall, Payload: payload}}, nil
}

func toolResultEvent(ok bool, summary string) ([]core.EventInput, error) {
	payload, err := core.MarshalPayload(core.ToolResultPayload{OK: ok, Summary: summary})
	if err != nil {
		return nil, err
	}
	return []core.EventInput{{Type: core.EventToolResult, Payload: payload}}, nil
}
