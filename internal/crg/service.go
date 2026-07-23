package crg

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// buildTimeout bounds one background graph build. Generous: a first full build
// of a large repo (Tree-sitter parse of every file) runs for minutes.
const buildTimeout = 30 * time.Minute

// Status is a repo's graph state, surfaced to the UI so a reviewer knows whether
// a review was codebase-aware or ran diff-only.
type Status string

const (
	StatusOff      Status = "off"      // the CLI is not installed
	StatusBuilding Status = "building" // a first build is in progress
	StatusReady    Status = "ready"    // a graph is available to query
)

// Service adds background-build orchestration over a Runner. Readiness itself is
// an on-disk fact (Runner.GraphReady), but the slow first build is kicked off
// once per repo and deduplicated so concurrent reviews don't stampede it.
type Service struct {
	runner *Runner
	logger *slog.Logger

	mu       sync.Mutex
	building map[string]bool // repos with an in-flight build
}

// NewService wraps a Runner. A nil Runner is tolerated (Status is always Off),
// so wiring never has to special-case a missing graph subsystem.
func NewService(runner *Runner, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{runner: runner, logger: logger, building: map[string]bool{}}
}

// Status reports repo's graph state, starting a background build when the graph
// is absent and none is already running. So the first review of a repo returns
// Building (and no context); later ones return Ready.
func (s *Service) Status(repo string) Status {
	if s.runner == nil || !s.runner.Available() {
		return StatusOff
	}
	if s.runner.GraphReady(repo) {
		return StatusReady
	}
	s.ensureBuilding(repo)
	return StatusBuilding
}

// ReviewContext returns the call-graph context block for a review, or "" with
// the current status when the graph is not ready. It never blocks on a build:
// the review proceeds diff-only while the graph warms up in the background.
func (s *Service) ReviewContext(ctx context.Context, repo string, files []string) (string, Status) {
	// Status may kick off a background build that intentionally runs on a
	// detached context.Background() (it must outlive this request), so it does
	// not — and must not — take the request ctx.
	st := s.Status(repo) //nolint:contextcheck // background build is deliberately detached; see ensureBuilding.
	if st != StatusReady {
		return "", st
	}
	imp, err := s.runner.Impact(ctx, repo, files)
	if err != nil {
		s.logger.Debug("crg impact query failed", "repo", repo, "err", err)
		return "", st
	}
	return ReviewContext(imp), st
}

// ensureBuilding starts a single detached background build for repo, deduped so
// a second caller during the build is a no-op.
func (s *Service) ensureBuilding(repo string) {
	s.mu.Lock()
	if s.building[repo] {
		s.mu.Unlock()
		return
	}
	s.building[repo] = true
	s.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
		defer cancel()
		if err := s.runner.Build(ctx, repo); err != nil {
			s.logger.Warn("crg background graph build failed", "repo", repo, "err", err)
		} else {
			s.logger.Info("crg graph built", "repo", repo)
		}
		s.mu.Lock()
		delete(s.building, repo)
		s.mu.Unlock()
	}()
}
