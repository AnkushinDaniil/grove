package tree

import (
	"context"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// Store is the persistence seam. Tree persists BEFORE applying changes in
// memory: if a save fails the mutation fails atomically and no delta is
// broadcast. Implementations must make each call atomic.
type Store interface {
	SaveNodes(ctx context.Context, nodes []core.Node) error
	SaveSessions(ctx context.Context, sessions []core.Session) error
	AppendEvents(ctx context.Context, events []core.Event) error
	// AckNodeEvents marks a node's unacknowledged attention events read and
	// returns them (acked_at set) so the tree can re-broadcast the change.
	AckNodeEvents(ctx context.Context, nodeID core.NodeID, at time.Time) ([]core.Event, error)
}
