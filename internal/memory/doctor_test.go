package memory

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDoctorHealthy(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true, mcpmode: "ok", version: "3.6.0"})
	writePalace(t, fb)
	writeClaudeSettings(t, fb, `{"hooks":{}}`)

	var out bytes.Buffer
	ok := fb.env().Doctor(context.Background(), &out)
	if !ok {
		t.Fatalf("Doctor ok = false, want true\n%s", out.String())
	}
	s := out.String()
	for _, want := range []string{
		"✓ mempalace installed",
		"✓ palace present",
		"✓ MCP server healthy",
		"no mempalace references",
		"is healthy",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("doctor output missing %q\n%s", want, s)
		}
	}
}

func TestDoctorNotInstalled(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{}) // nothing installed, no palace
	var out bytes.Buffer
	ok := fb.env().Doctor(context.Background(), &out)
	if ok {
		t.Fatalf("Doctor ok = true, want false\n%s", out.String())
	}
	s := out.String()
	for _, want := range []string{
		"✗ mempalace not installed",
		"✗ palace not initialized",
		"skipping MCP probe",
		"needs attention",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("doctor output missing %q\n%s", want, s)
		}
	}
}

func TestDoctorHooksReferenceMempalaceHealthy(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true, mcpmode: "ok"})
	writePalace(t, fb)
	writeClaudeSettings(t, fb, `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"mempalace hook capture"}]}]}}`)

	var out bytes.Buffer
	if ok := fb.env().Doctor(context.Background(), &out); !ok {
		t.Fatalf("Doctor ok = false, want true\n%s", out.String())
	}
	s := out.String()
	if !strings.Contains(s, "your Claude hooks reference mempalace") || !strings.Contains(s, "works now") {
		t.Errorf("expected healthy hook note, got:\n%s", s)
	}
}

func TestDoctorHooksReferenceMempalaceBroken(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{}) // not installed
	writeClaudeSettings(t, fb, `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"mempalace hook capture"}]}]}}`)

	var out bytes.Buffer
	if ok := fb.env().Doctor(context.Background(), &out); ok {
		t.Fatalf("Doctor ok = true, want false\n%s", out.String())
	}
	if s := out.String(); !strings.Contains(s, "not working yet") {
		t.Errorf("expected broken-hook note, got:\n%s", s)
	}
}

func TestDoctorProbeFailure(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true, mcpmode: "errorinit"})
	writePalace(t, fb)
	var out bytes.Buffer
	if ok := fb.env().Doctor(context.Background(), &out); ok {
		t.Fatalf("Doctor ok = true, want false on probe failure\n%s", out.String())
	}
	if s := out.String(); !strings.Contains(s, "✗ MCP probe failed") {
		t.Errorf("expected probe failure line, got:\n%s", s)
	}
}

func TestJSONMentionsMempalace(t *testing.T) {
	cases := []struct {
		name string
		data string
		want bool
	}{
		{"nested command", `{"hooks":{"Stop":[{"hooks":[{"command":"mempalace hook"}]}]}}`, true},
		{"absent", `{"hooks":{"Stop":[{"hooks":[{"command":"grove hook"}]}]}}`, false},
		{"non-json fallback", "not json but mentions mempalace", true},
		{"non-json absent", "not json at all", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jsonMentionsMempalace([]byte(tc.data)); got != tc.want {
				t.Errorf("jsonMentionsMempalace(%q) = %v, want %v", tc.data, got, tc.want)
			}
		})
	}
}
