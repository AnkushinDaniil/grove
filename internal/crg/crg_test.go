package crg

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner builds a Runner that "has" the CLI but routes exec to fn, so tests
// exercise argv construction and JSON parsing without the real binary.
func fakeRunner(t *testing.T, fn execFunc) *Runner {
	t.Helper()
	return &Runner{
		bin:      "code-review-graph",
		graphDir: t.TempDir(),
		exec:     fn,
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

const impactJSON = `{
  "status":"ok",
  "summary":"Blast radius for 1 changed file(s):\n  - 2 nodes directly changed\n  - 1 nodes impacted (within 2 hops)",
  "changed_files":["a.go"],
  "changed_nodes":[{"kind":"Function","name":"Helper","file_path":"a.go","line_start":2,"is_test":false}],
  "impacted_nodes":[
    {"kind":"Function","name":"Caller","file_path":"b.go","line_start":5,"is_test":false},
    {"kind":"Test","name":"TestHelper","file_path":"a_test.go","line_start":9,"is_test":true}
  ],
  "impacted_files":["b.go"],
  "total_impacted":1
}`

func TestImpactParsesAndPassesFiles(t *testing.T) {
	var gotArgs []string
	r := fakeRunner(t, func(_ context.Context, _ string, _ string, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(impactJSON), nil
	})
	imp, err := r.Impact(context.Background(), "/repo", []string{"a.go", "b.go"})
	if err != nil {
		t.Fatalf("Impact: %v", err)
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "impact --repo /repo --files a.go b.go") {
		t.Errorf("args = %v, want impact with both files", gotArgs)
	}
	if len(imp.ChangedNodes) != 1 || imp.ChangedNodes[0].Name != "Helper" {
		t.Errorf("changed nodes = %+v", imp.ChangedNodes)
	}
	if len(imp.ImpactedNodes) != 2 || !imp.ImpactedNodes[1].IsTest {
		t.Errorf("impacted nodes = %+v, want the test flagged", imp.ImpactedNodes)
	}
}

func TestQueryParses(t *testing.T) {
	const q = `{"status":"ok","pattern":"callers_of","result_count":1,"results":[{"kind":"Function","name":"Caller","file_path":"b.go"}]}`
	var gotArgs []string
	r := fakeRunner(t, func(_ context.Context, _ string, _ string, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(q), nil
	})
	res, err := r.Query(context.Background(), "/repo", "callers_of", "Helper")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if res.ResultCount != 1 || res.Results[0].Name != "Caller" {
		t.Errorf("result = %+v", res)
	}
	if strings.Join(gotArgs, " ") != "query --repo /repo callers_of Helper" {
		t.Errorf("args = %v", gotArgs)
	}
}

func TestUnavailableRunnerDegrades(t *testing.T) {
	r := &Runner{bin: "", graphDir: t.TempDir(), exec: nil, logger: nil}
	if r.Available() {
		t.Fatal("Available() true with empty bin")
	}
	if _, err := r.Impact(context.Background(), "/repo", []string{"a.go"}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Impact err = %v, want ErrUnavailable", err)
	}
	if err := r.Build(context.Background(), "/repo"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Build err = %v, want ErrUnavailable", err)
	}
}

func TestGraphReadyAndDataDirIsolation(t *testing.T) {
	r := fakeRunner(t, nil)
	if r.GraphReady("/repo") {
		t.Fatal("GraphReady true before any build")
	}
	// Two repos must map to different data dirs (namespaced by path hash).
	if r.dataDir("/repo/a") == r.dataDir("/repo/b") {
		t.Fatal("distinct repos share a data dir")
	}
	// Simulate a completed build: graph.db present under the repo's data dir.
	dir := r.dataDir("/repo/a")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "graph.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !r.GraphReady("/repo/a") {
		t.Error("GraphReady false after graph.db written")
	}
	if r.GraphReady("/repo/b") {
		t.Error("GraphReady leaked across repos")
	}
}
