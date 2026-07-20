package claude

import "github.com/AnkushinDaniil/grove/internal/core"

// parseSystem handles {"type":"system",...}. Only subtype "init" is
// documented; other system subtypes are ignored.
func parseSystem(raw streamLine) ([]core.EventInput, error) {
	if raw.Subtype != "init" {
		return nil, nil
	}
	payload, err := core.MarshalPayload(core.SessionStartedPayload{
		DriverSessionID: raw.SessionID,
		Model:           raw.Model,
	})
	if err != nil {
		return nil, err
	}
	return []core.EventInput{{Type: core.EventSessionStarted, Payload: payload}}, nil
}

// parseAssistant handles {"type":"assistant",...}, mapping each text block
// to EventText and each tool_use block to EventToolCall. Other block types
// (e.g. "thinking") are skipped.
func parseAssistant(raw streamLine) ([]core.EventInput, error) {
	if raw.Message == nil {
		return nil, nil
	}
	events := make([]core.EventInput, 0, len(raw.Message.Content))
	for _, block := range raw.Message.Content {
		var (
			evType  core.EventType
			payload string
			err     error
		)
		switch block.Type {
		case "text":
			evType = core.EventText
			payload, err = core.MarshalPayload(core.TextPayload{Text: block.Text})
		case "tool_use":
			evType = core.EventToolCall
			payload, err = core.MarshalPayload(core.ToolCallPayload{
				Name:         block.Name,
				InputSummary: compactTruncate(block.Input, summaryLen),
			})
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		events = append(events, core.EventInput{Type: evType, Payload: payload})
	}
	return events, nil
}

// parseUser handles {"type":"user",...}, mapping each tool_result block to
// EventToolResult. The tool name is unavailable in this line and left empty.
func parseUser(raw streamLine) ([]core.EventInput, error) {
	if raw.Message == nil {
		return nil, nil
	}
	events := make([]core.EventInput, 0, len(raw.Message.Content))
	for _, block := range raw.Message.Content {
		if block.Type != "tool_result" {
			continue
		}
		payload, err := core.MarshalPayload(core.ToolResultPayload{
			OK:      !block.IsError,
			Summary: truncate(flattenToolResultContent(block.Content), summaryLen),
		})
		if err != nil {
			return nil, err
		}
		events = append(events, core.EventInput{Type: core.EventToolResult, Payload: payload})
	}
	return events, nil
}

// parseResult handles {"type":"result",...}: always a turn_done plus usage,
// and additionally an error event when the turn did not succeed.
func parseResult(raw streamLine) ([]core.EventInput, error) {
	turnPayload, err := core.MarshalPayload(core.TurnDonePayload{
		ResultText: raw.Result,
		DurationMS: raw.DurationMS,
	})
	if err != nil {
		return nil, err
	}
	usage := core.UsagePayload{CostUSD: raw.TotalCostUSD}
	if raw.Usage != nil {
		usage.InputTokens = raw.Usage.InputTokens
		usage.OutputTokens = raw.Usage.OutputTokens
	}
	usagePayload, err := core.MarshalPayload(usage)
	if err != nil {
		return nil, err
	}

	events := []core.EventInput{
		{Type: core.EventTurnDone, Payload: turnPayload},
		{Type: core.EventUsage, Payload: usagePayload},
	}

	if raw.Subtype != "success" {
		msg := raw.Result
		if msg == "" {
			msg = raw.Subtype
		}
		errPayload, err := core.MarshalPayload(core.ErrorPayload{Message: msg})
		if err != nil {
			return nil, err
		}
		events = append(events, core.EventInput{Type: core.EventError, Payload: errPayload})
	}

	return events, nil
}
