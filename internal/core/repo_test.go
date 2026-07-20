package core

import (
	"errors"
	"testing"
	"time"
)

func validRepo() Repo {
	return Repo{
		ID:         NewRepoID(),
		ProjectID:  NewNodeID(),
		Name:       "nethermind",
		SourcePath: "/Users/dev/nethermind",
		CreatedAt:  time.Now(),
	}
}

func TestRepoValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Repo)
		wantErr bool
	}{
		{"valid", func(r *Repo) {}, false},
		{"empty id", func(r *Repo) { r.ID = "" }, true},
		{"empty project", func(r *Repo) { r.ProjectID = "" }, true},
		{"empty name", func(r *Repo) { r.Name = "" }, true},
		{"dot name", func(r *Repo) { r.Name = "." }, true},
		{"dotdot name", func(r *Repo) { r.Name = ".." }, true},
		{"slash in name", func(r *Repo) { r.Name = "a/b" }, true},
		{"backslash in name", func(r *Repo) { r.Name = `a\b` }, true},
		{"relative source", func(r *Repo) { r.SourcePath = "dev/nethermind" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validRepo()
			tt.mutate(&r)
			err := r.Validate()
			if tt.wantErr != (err != nil) {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestWorktreeStatusValid(t *testing.T) {
	for _, s := range []WorktreeStatus{WorktreeActive, WorktreeOrphaned, WorktreeRemoved} {
		if !s.Valid() {
			t.Errorf("WorktreeStatus(%q).Valid() = false, want true", s)
		}
	}
	if WorktreeStatus("stale").Valid() {
		t.Error(`WorktreeStatus("stale").Valid() = true, want false`)
	}
}

func TestProfileValidate(t *testing.T) {
	valid := Profile{
		ID:        NewProfileID(),
		Driver:    "claude",
		Name:      "work",
		ConfigDir: "/Users/dev/.grove/profiles/claude/work",
		CreatedAt: time.Now(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid profile: Validate() = %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Profile)
	}{
		{"empty id", func(p *Profile) { p.ID = "" }},
		{"empty driver", func(p *Profile) { p.Driver = "" }},
		{"empty name", func(p *Profile) { p.Name = "" }},
		{"relative config dir", func(p *Profile) { p.ConfigDir = ".grove/x" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := valid
			tt.mutate(&p)
			if err := p.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() = %v, want ErrInvalid", err)
			}
		})
	}
}
