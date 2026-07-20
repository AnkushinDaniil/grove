package core

import "testing"

// UUIDv7 IDs must be unique and time-sortable: later IDs compare greater as
// strings, which is what lets append-only tables order by primary key.
func TestNewIDUniqueAndSortable(t *testing.T) {
	const n = 1000
	seen := make(map[NodeID]bool, n)
	var prev NodeID
	for i := 0; i < n; i++ {
		id := NewNodeID()
		if len(id) != 36 {
			t.Fatalf("id %q has length %d, want 36", id, len(id))
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
		if prev != "" && id < prev {
			t.Fatalf("ids not monotonic: %q < %q", id, prev)
		}
		prev = id
	}
}
