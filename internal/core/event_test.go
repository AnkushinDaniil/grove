package core

import "testing"

func TestEventTypeValid(t *testing.T) {
	valid := []EventType{
		EventSessionStarted, EventText, EventToolCall, EventToolResult,
		EventAwaitingInput, EventTurnDone, EventSessionEnded, EventError, EventUsage,
	}
	for _, et := range valid {
		if !et.Valid() {
			t.Errorf("EventType(%q).Valid() = false, want true", et)
		}
	}
	if EventType("heartbeat").Valid() {
		t.Error(`EventType("heartbeat").Valid() = true, want false`)
	}
}

func TestAttentionFor(t *testing.T) {
	tests := []struct {
		event  EventType
		reason AwaitingReason
		want   Attention
	}{
		{EventAwaitingInput, AwaitPermission, AttentionPermission},
		{EventAwaitingInput, AwaitQuestion, AttentionQuestion},
		{EventAwaitingInput, AwaitIdle, AttentionQuestion},
		{EventTurnDone, "", AttentionDone},
		{EventError, "", AttentionError},
		{EventText, "", AttentionNone},
		{EventToolCall, "", AttentionNone},
		{EventUsage, "", AttentionNone},
		{EventSessionStarted, "", AttentionNone},
		{EventSessionEnded, "", AttentionNone},
	}
	for _, tt := range tests {
		if got := AttentionFor(tt.event, tt.reason); got != tt.want {
			t.Errorf("AttentionFor(%s, %s) = %s, want %s", tt.event, tt.reason, got, tt.want)
		}
	}
}
