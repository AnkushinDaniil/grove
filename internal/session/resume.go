package session

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/term"
)

// farewellRe matches the CLI's goodbye hint printed when an interactive
// session ends ("Resume this session with: claude --resume <uuid>"). That id
// is the AUTHORITATIVE conversation id: hook-captured ids can be poisoned by
// a failed resume attempt, which mints a fresh session id via its own
// SessionStart hook and then dies without ever persisting a conversation.
var farewellRe = regexp.MustCompile(`--resume ([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// ExtractResumeID returns the last farewell conversation id in raw terminal
// output, or "" when none is present.
func ExtractResumeID(raw []byte) string {
	matches := farewellRe.FindAllSubmatch(raw, -1)
	if len(matches) == 0 {
		return ""
	}
	return string(matches[len(matches)-1][1])
}

// ScrollbackFile returns the persisted scrollback path for a session id under
// dir ("" when persistence is disabled).
func ScrollbackFile(dir string, id core.SessionID) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, string(id)+".bin")
}

// resumeIDFromScrollback reads a session's persisted scrollback and extracts
// the farewell conversation id, best-effort.
func resumeIDFromScrollback(dir string, id core.SessionID) string {
	path := ScrollbackFile(dir, id)
	if path == "" {
		return ""
	}
	raw, err := term.LoadScrollback(path)
	if err != nil {
		return ""
	}
	return ExtractResumeID(raw)
}

// BackfillStore is the slice of the store the resume backfill needs.
type BackfillStore interface {
	SessionsWithoutResumeID(ctx context.Context) ([]core.Session, error)
	SaveSessions(ctx context.Context, sessions []core.Session) error
}

// BackfillResumeIDs heals PTY sessions persisted without a conversation id
// (history from before hook wiring) by scanning their scrollback farewells.
// Returns how many sessions were updated.
func BackfillResumeIDs(ctx context.Context, st BackfillStore, scrollbackDir string) (int, error) {
	if scrollbackDir == "" {
		return 0, nil
	}
	sessions, err := st.SessionsWithoutResumeID(ctx)
	if err != nil {
		return 0, fmt.Errorf("list sessions without resume id: %w", err)
	}
	var healed []core.Session
	for _, sess := range sessions {
		if id := resumeIDFromScrollback(scrollbackDir, sess.ID); id != "" {
			sess.DriverSessionID = id
			healed = append(healed, sess)
		}
	}
	if len(healed) == 0 {
		return 0, nil
	}
	if err := st.SaveSessions(ctx, healed); err != nil {
		return 0, fmt.Errorf("persist backfilled resume ids: %w", err)
	}
	return len(healed), nil
}
