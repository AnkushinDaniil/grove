package core

import (
	"errors"
	"testing"
	"time"
)

func TestCanTransition(t *testing.T) {
	all := []SessionStatus{
		SessionStarting, SessionRunning, SessionAwaitingInput,
		SessionExited, SessionFailed, SessionInterrupted,
	}
	allowed := map[SessionStatus]map[SessionStatus]bool{
		SessionStarting: {
			SessionRunning: true, SessionFailed: true, SessionInterrupted: true,
		},
		SessionRunning: {
			SessionAwaitingInput: true, SessionExited: true,
			SessionFailed: true, SessionInterrupted: true,
		},
		SessionAwaitingInput: {
			SessionRunning: true, SessionExited: true,
			SessionFailed: true, SessionInterrupted: true,
		},
		// Terminal states allow nothing.
		SessionExited:      {},
		SessionFailed:      {},
		SessionInterrupted: {},
	}
	for _, from := range all {
		for _, to := range all {
			want := allowed[from][to]
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestNodeStatusFor(t *testing.T) {
	tests := []struct {
		status SessionStatus
		exitOK bool
		want   NodeStatus
	}{
		{SessionStarting, false, StatusStarting},
		{SessionRunning, false, StatusRunning},
		{SessionAwaitingInput, false, StatusAwaitingInput},
		{SessionExited, true, StatusDone},
		{SessionExited, false, StatusFailed},
		{SessionFailed, true, StatusFailed},
		{SessionInterrupted, false, StatusInterrupted},
	}
	for _, tt := range tests {
		if got := NodeStatusFor(tt.status, tt.exitOK); got != tt.want {
			t.Errorf("NodeStatusFor(%s, %v) = %s, want %s", tt.status, tt.exitOK, got, tt.want)
		}
	}
}

func validSession() Session {
	return Session{
		ID:        NewSessionID(),
		NodeID:    NewNodeID(),
		Driver:    "claude",
		Mode:      ModePTY,
		Status:    SessionStarting,
		CWD:       "/tmp/ws",
		StartedAt: time.Now(),
	}
}

func TestSessionValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Session)
		wantErr bool
	}{
		{"valid", func(s *Session) {}, false},
		{"headless valid", func(s *Session) { s.Mode = ModeHeadless }, false},
		{"empty id", func(s *Session) { s.ID = "" }, true},
		{"empty node", func(s *Session) { s.NodeID = "" }, true},
		{"empty driver", func(s *Session) { s.Driver = "" }, true},
		{"bad mode", func(s *Session) { s.Mode = "tmux" }, true},
		{"bad status", func(s *Session) { s.Status = "zombie" }, true},
		{"empty cwd", func(s *Session) { s.CWD = "" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSession()
			tt.mutate(&s)
			err := s.Validate()
			if tt.wantErr != (err != nil) {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() = %v, want ErrInvalid", err)
			}
		})
	}
}
