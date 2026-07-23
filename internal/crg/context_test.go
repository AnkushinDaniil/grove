package crg

import (
	"strings"
	"testing"
)

func TestReviewContextFormatsBlastRadius(t *testing.T) {
	imp := Impact{
		Summary: "Blast radius for 1 changed file(s):\n  - 2 nodes directly changed",
		ImpactedNodes: []Node{
			{Kind: "Function", Name: "Caller", FilePath: "b.go", LineStart: 5},
			{Kind: "Test", Name: "TestHelper", FilePath: "a_test.go", LineStart: 9, IsTest: true},
			{Kind: "File", Name: "b.go", FilePath: "b.go"}, // File nodes are not listed as dependents
		},
	}
	out := ReviewContext(imp)
	if !strings.Contains(out, "Blast radius for 1 changed file") {
		t.Errorf("missing summary:\n%s", out)
	}
	if !strings.Contains(out, "Caller (b.go:5)") {
		t.Errorf("missing dependent:\n%s", out)
	}
	if !strings.Contains(out, "TestHelper (a_test.go:9)") {
		t.Errorf("missing test:\n%s", out)
	}
	// The File-kind impacted node must not appear under dependents.
	if strings.Count(out, "b.go:") > 1 {
		t.Errorf("File node leaked into the dependents list:\n%s", out)
	}
}

func TestReviewContextEmptyWhenNothingStructural(t *testing.T) {
	if got := ReviewContext(Impact{}); got != "" {
		t.Errorf("ReviewContext(empty) = %q, want empty", got)
	}
}

func TestReviewContextCapsAndCountsOmitted(t *testing.T) {
	nodes := make([]Node, maxContextNodes+5)
	for i := range nodes {
		nodes[i] = Node{Kind: "Function", Name: "F", FilePath: "f.go", LineStart: i + 1}
	}
	out := ReviewContext(Impact{Summary: "x", ImpactedNodes: nodes})
	if !strings.Contains(out, "and 5 more") {
		t.Errorf("expected an omitted-count note:\n%s", out)
	}
}
