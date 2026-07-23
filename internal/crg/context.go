package crg

import (
	"fmt"
	"strings"
)

const (
	// maxContextNodes caps how many dependents/tests are listed so the injected
	// block stays compact and high-signal even for a wide blast radius.
	maxContextNodes = 15
	// maxContextBytes hard-bounds the whole block.
	maxContextBytes = 6 * 1024
)

// ReviewContext renders an Impact into a compact markdown block for the review
// prompt: the blast-radius summary, the dependents that could break, and the
// covering tests. It returns "" when there is nothing structural to add (so the
// caller injects nothing rather than an empty heading).
func ReviewContext(imp Impact) string {
	dependents := make([]Node, 0, len(imp.ImpactedNodes))
	tests := make([]Node, 0)
	for _, n := range imp.ImpactedNodes {
		if n.IsTest {
			tests = append(tests, n)
		} else if n.Kind != "File" {
			dependents = append(dependents, n)
		}
	}
	// Also surface tests the changed files themselves are, or that landed in the
	// impacted set — either way they are the tests a reviewer should think about.
	for _, n := range imp.ChangedNodes {
		if n.IsTest {
			tests = append(tests, n)
		}
	}
	if strings.TrimSpace(imp.Summary) == "" && len(dependents) == 0 && len(tests) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Repository call-graph context (structural facts about the code these changes touch; ")
	b.WriteString("use it to weigh impact and correctness — do not assume anything not shown here):\n")
	if s := strings.TrimSpace(imp.Summary); s != "" {
		b.WriteString(s)
		b.WriteString("\n")
	}
	if len(dependents) > 0 {
		b.WriteString("Dependents that could be affected by these changes:\n")
		writeNodes(&b, dependents)
	}
	if len(tests) > 0 {
		b.WriteString("Covering / related tests:\n")
		writeNodes(&b, tests)
	}
	return capBlock(b.String())
}

// writeNodes lists up to maxContextNodes as "- name (file:line)", noting how
// many were omitted so a truncated list never reads as complete.
func writeNodes(b *strings.Builder, nodes []Node) {
	shown := nodes
	if len(shown) > maxContextNodes {
		shown = shown[:maxContextNodes]
	}
	for _, n := range shown {
		if n.LineStart > 0 {
			fmt.Fprintf(b, "- %s (%s:%d)\n", n.Name, n.FilePath, n.LineStart)
		} else {
			fmt.Fprintf(b, "- %s (%s)\n", n.Name, n.FilePath)
		}
	}
	if omitted := len(nodes) - len(shown); omitted > 0 {
		fmt.Fprintf(b, "- … and %d more\n", omitted)
	}
}

func capBlock(s string) string {
	if len(s) <= maxContextBytes {
		return s
	}
	return s[:maxContextBytes] + "\n… (context truncated)"
}
