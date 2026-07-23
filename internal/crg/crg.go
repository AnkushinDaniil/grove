// Package crg wraps the code-review-graph CLI (https://github.com/tirth8205/
// code-review-graph): a local, incremental knowledge graph of a repository's
// symbols and their relationships (call graph, imports, tests, blast radius).
// grove shells out to it exactly like internal/gitcli and internal/github wrap
// git and gh — pure argv construction plus JSON parsing — and uses it to hand a
// constrained reviewer (or a working agent) structural codebase context without
// re-reading the code each time.
//
// Every method degrades gracefully when the binary is absent: Available reports
// false and the query methods return ErrUnavailable, so a caller can always try
// and simply fall back (an AI review without the graph is still a valid review).
package crg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

// binaryName is the code-review-graph executable grove looks for on PATH.
const binaryName = "code-review-graph"

// ErrUnavailable is returned by query methods when the CLI is not installed.
var ErrUnavailable = errors.New("code-review-graph is not installed")

// Node is one graph entity — a function, class, file, or test — as the query
// and impact commands return it.
type Node struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	QualifiedName string `json:"qualified_name"`
	FilePath      string `json:"file_path"`
	LineStart     int    `json:"line_start"`
	LineEnd       int    `json:"line_end"`
	Language      string `json:"language"`
	IsTest        bool   `json:"is_test"`
}

// Impact is the parsed output of `code-review-graph impact --files …`: the blast
// radius of a set of changed files. Summary is a ready-to-read blast-radius
// paragraph; the node slices carry the structured detail.
type Impact struct {
	Summary       string   `json:"summary"`
	ChangedFiles  []string `json:"changed_files"`
	ChangedNodes  []Node   `json:"changed_nodes"`
	ImpactedNodes []Node   `json:"impacted_nodes"`
	ImpactedFiles []string `json:"impacted_files"`
	TotalImpacted int      `json:"total_impacted"`
}

// QueryResult is the parsed output of `code-review-graph query <relation>
// <target>` (callers_of, callees_of, tests_for, file_summary, …).
type QueryResult struct {
	Pattern     string `json:"pattern"`
	ResultCount int    `json:"result_count"`
	Results     []Node `json:"results"`
}

// execFunc runs a command and returns its stdout; injectable so tests exercise
// argv construction and JSON parsing without the real CLI.
type execFunc func(ctx context.Context, dir, name string, args ...string) ([]byte, error)

// Runner invokes the code-review-graph CLI against per-repo graphs stored under
// a grove-owned directory (never inside the user's repo).
type Runner struct {
	bin      string // resolved binary path; "" when not installed
	graphDir string // parent dir for all per-repo graph databases
	exec     execFunc
	logger   *slog.Logger
}

// New locates the CLI on PATH and returns a Runner whose graphs live under
// graphDir (typically <GROVE_HOME>/graphs). An absent binary is not an error —
// the Runner is still usable and simply reports Available() == false.
func New(graphDir string, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	bin, err := exec.LookPath(binaryName)
	if err != nil {
		bin = ""
	}
	return &Runner{bin: bin, graphDir: graphDir, exec: defaultExec, logger: logger}
}

// Available reports whether the CLI was found on PATH.
func (r *Runner) Available() bool { return r.bin != "" }

// dataDir is the external graph-database directory for one repo, namespaced by a
// hash of its absolute path so unrelated repos never share a graph and the
// user's working tree stays clean.
func (r *Runner) dataDir(repo string) string {
	sum := sha256.Sum256([]byte(repo))
	return filepath.Join(r.graphDir, hex.EncodeToString(sum[:])[:16])
}

// GraphReady reports whether a built graph already exists for repo — a cheap
// on-disk check (graph.db present), so readiness survives daemon restarts
// without any in-memory bookkeeping.
func (r *Runner) GraphReady(repo string) bool {
	info, err := os.Stat(filepath.Join(r.dataDir(repo), "graph.db"))
	return err == nil && !info.IsDir()
}

// Build performs a full graph build for repo into its grove-owned data dir. It
// is the slow path (minutes on a large repo), so callers run it in the
// background and gate on GraphReady; ctx bounds it.
func (r *Runner) Build(ctx context.Context, repo string) error {
	if !r.Available() {
		return ErrUnavailable
	}
	dir := r.dataDir(repo)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create graph dir: %w", err)
	}
	if _, err := r.exec(ctx, repo, r.bin, "build", "--repo", repo, "--data-dir", dir, "-q"); err != nil {
		return fmt.Errorf("crg build: %w", err)
	}
	return nil
}

// Impact returns the blast radius of the given repo-relative changed files. The
// impact/query commands take no --data-dir: they locate the graph through the
// registry that Build populated, keyed by --repo.
func (r *Runner) Impact(ctx context.Context, repo string, files []string) (Impact, error) {
	if !r.Available() {
		return Impact{}, ErrUnavailable
	}
	args := append([]string{"impact", "--repo", repo, "--files"}, files...)
	out, err := r.exec(ctx, repo, r.bin, args...)
	if err != nil {
		return Impact{}, fmt.Errorf("crg impact: %w", err)
	}
	var imp Impact
	if err := json.Unmarshal(out, &imp); err != nil {
		return Impact{}, fmt.Errorf("parse crg impact: %w", err)
	}
	return imp, nil
}

// Query runs one relationship query (callers_of, callees_of, tests_for,
// file_summary, imports_of, importers_of, children_of, inheritors_of) for a
// symbol name, qualified name, or file path.
func (r *Runner) Query(ctx context.Context, repo, relation, target string) (QueryResult, error) {
	if !r.Available() {
		return QueryResult{}, ErrUnavailable
	}
	out, err := r.exec(ctx, repo, r.bin, "query", "--repo", repo, relation, target)
	if err != nil {
		return QueryResult{}, fmt.Errorf("crg query %s: %w", relation, err)
	}
	var res QueryResult
	if err := json.Unmarshal(out, &res); err != nil {
		return QueryResult{}, fmt.Errorf("parse crg query: %w", err)
	}
	return res, nil
}

// defaultExec runs the CLI in dir and returns stdout, folding stderr into the
// error so failures are diagnosable.
func defaultExec(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	//nolint:gosec // G204: name is the fixed code-review-graph binary; args are literals + repo paths.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%w: %s", err, ee.Stderr)
		}
		return nil, err
	}
	return out, nil
}
