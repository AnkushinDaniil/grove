package worktree

import (
	"context"
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// Check inspects wt on disk and reports its current State. Dirty takes
// precedence over Unmerged: a worktree with both uncommitted changes and
// unmerged commits reports Dirty.
func (e *Engine) Check(ctx context.Context, wt core.Worktree) (State, error) {
	dirty, err := e.git.IsDirty(ctx, wt.Path)
	if err != nil {
		return Clean, fmt.Errorf("check %s: %w", wt.Path, err)
	}
	if dirty {
		return Dirty, nil
	}

	unmerged, err := e.git.HasUnmerged(ctx, wt.Path, wt.BaseRef)
	if err != nil {
		return Clean, fmt.Errorf("check %s against %s: %w", wt.Path, wt.BaseRef, err)
	}
	if unmerged {
		return Unmerged, nil
	}

	return Clean, nil
}
