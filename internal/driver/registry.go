package driver

import (
	"fmt"
	"slices"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// Registry is an immutable set of drivers keyed by ID, built once at startup.
type Registry struct {
	byID map[string]Driver
	ids  []string
}

// NewRegistry builds a registry, rejecting duplicate driver IDs.
func NewRegistry(drivers ...Driver) (*Registry, error) {
	r := &Registry{byID: make(map[string]Driver, len(drivers))}
	for _, d := range drivers {
		id := d.ID()
		if id == "" {
			return nil, fmt.Errorf("%w: driver with empty id", core.ErrInvalid)
		}
		if _, dup := r.byID[id]; dup {
			return nil, fmt.Errorf("%w: duplicate driver id %q", core.ErrInvalid, id)
		}
		r.byID[id] = d
		r.ids = append(r.ids, id)
	}
	slices.Sort(r.ids)
	return r, nil
}

// Get returns the driver for id.
func (r *Registry) Get(id string) (Driver, bool) {
	d, ok := r.byID[id]
	return d, ok
}

// IDs returns the sorted driver ids.
func (r *Registry) IDs() []string { return slices.Clone(r.ids) }
