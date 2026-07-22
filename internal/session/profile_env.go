package session

import (
	"context"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// ProfileLookup resolves a profile by id so a starting session can be pointed at
// that account's isolated CLI config dir. The manager consults it only when
// non-nil; a nil lookup preserves the unwired launch path (the CLI runs on its
// default config dir, the pre-profile behavior).
type ProfileLookup interface {
	Get(ctx context.Context, id core.ProfileID) (core.Profile, bool)
}

// ProfileLister is the store capability the lookup adapter needs — the subset of
// *store.Store used here, kept as an interface so this package does not depend
// on the store package.
type ProfileLister interface {
	ListProfiles(ctx context.Context) ([]core.Profile, error)
}

// storeProfileLookup adapts a ProfileLister to ProfileLookup with a linear scan.
// Sessions start infrequently and profiles are few, so scanning the list per
// start is cheaper than adding a keyed store query for it.
type storeProfileLookup struct {
	lister ProfileLister
}

// NewStoreProfileLookup adapts a profile store to a ProfileLookup for wiring
// into a Manager's Config.
func NewStoreProfileLookup(lister ProfileLister) ProfileLookup {
	return storeProfileLookup{lister: lister}
}

func (l storeProfileLookup) Get(ctx context.Context, id core.ProfileID) (core.Profile, bool) {
	if id == "" {
		return core.Profile{}, false
	}
	profiles, err := l.lister.ListProfiles(ctx)
	if err != nil {
		return core.Profile{}, false
	}
	for _, p := range profiles {
		if p.ID == id {
			return p, true
		}
	}
	return core.Profile{}, false
}

// profileConfigDir returns the config dir of the resolved profile, or "" when no
// profile is selected, no lookup is configured, or the profile is unknown. An
// empty result leaves the launch on the CLI's default config dir. The daemon's
// base env already scrubs any ambient CLAUDE_CONFIG_DIR/ANTHROPIC_API_KEY (see
// env.go), so the profile dir the driver renders wins unopposed.
func (m *Manager) profileConfigDir(ctx context.Context, id core.ProfileID) string {
	if id == "" || m.cfg.Profiles == nil {
		return ""
	}
	p, ok := m.cfg.Profiles.Get(ctx, id)
	if !ok {
		return ""
	}
	return p.ConfigDir
}
