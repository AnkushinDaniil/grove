package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewGuidelinesMergesGlobalAndRepo(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "review.md"), []byte("Global: ASCII only."), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".grove"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".grove", "review.md"), []byte("Repo: prefer early returns."), 0o600); err != nil {
		t.Fatal(err)
	}

	h := &Handlers{groveHome: home}
	got := h.reviewGuidelines(repo)
	if !strings.Contains(got, "Global: ASCII only.") || !strings.Contains(got, "Repo: prefer early returns.") {
		t.Errorf("guidelines = %q, want both global and repo parts", got)
	}
	// Global comes before repo so the repo file extends/overrides it.
	if strings.Index(got, "Global") > strings.Index(got, "Repo") {
		t.Errorf("repo guidelines should follow global: %q", got)
	}
}

func TestReviewGuidelinesAbsentIsEmpty(t *testing.T) {
	h := &Handlers{groveHome: t.TempDir()}
	if got := h.reviewGuidelines(t.TempDir()); got != "" {
		t.Errorf("reviewGuidelines with no files = %q, want empty", got)
	}
}

func TestGuidelinesBlockOmittedWhenEmpty(t *testing.T) {
	if got := guidelinesBlock("   "); got != "" {
		t.Errorf("guidelinesBlock(blank) = %q, want empty", got)
	}
	if got := guidelinesBlock("ASCII only"); !strings.Contains(got, "ASCII only") {
		t.Errorf("guidelinesBlock dropped the content: %q", got)
	}
}

func TestReviewPromptIncludesGuidelines(t *testing.T) {
	prompt := buildAIReviewPrompt(anchoredPR(), "", "Comments must be ASCII-only.")
	if !strings.Contains(prompt, "Comments must be ASCII-only.") {
		t.Errorf("review prompt missing injected guidelines:\n%s", prompt)
	}
}
