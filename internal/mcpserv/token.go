package mcpserv

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// tokenBytes is the entropy of a per-node MCP token.
const tokenBytes = 32

// Role is a node's orchestration capability set. A worker may report on itself;
// an orchestrator may additionally spawn, list and inspect its subtree.
type Role string

const (
	RoleWorker       Role = "worker"
	RoleOrchestrator Role = "orchestrator"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool { return r == RoleWorker || r == RoleOrchestrator }

// CanOrchestrate reports whether the role may use orchestrator-only tools.
func (r Role) CanOrchestrate() bool { return r == RoleOrchestrator }

// grant is a live token binding: the secret plus the capability it carries.
type grant struct {
	token string
	role  Role
}

// Registry mints and validates per-node MCP tokens. Each spawned session gets a
// token bound to its node id and role; the daemon-side MCP server authenticates
// every UDS connection against it, which pins identity to the node (no self
// parameter, no sibling spoofing). It is safe for concurrent use.
type Registry struct {
	mu     sync.Mutex
	byNode map[core.NodeID]grant
}

// NewRegistry builds an empty token registry.
func NewRegistry() *Registry {
	return &Registry{byNode: make(map[core.NodeID]grant)}
}

// Mint returns the node's token, generating and storing one on first use. It is
// idempotent per node: repeated calls return the same token so an orchestrator
// keeps the same identity across wake turns. The role is fixed by the first
// call and not changed by later ones.
func (r *Registry) Mint(nodeID core.NodeID, role Role) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.byNode[nodeID]; ok {
		return g.token
	}
	raw := make([]byte, tokenBytes)
	_, _ = rand.Read(raw) // crypto/rand.Read never fails on supported platforms
	tok := hex.EncodeToString(raw)
	r.byNode[nodeID] = grant{token: tok, role: role}
	return tok
}

// Resolve authenticates a (node, token) pair in constant time and returns the
// node's role. It fails for unknown nodes, mismatched tokens, and revoked
// nodes — so a token only ever speaks for the node it was minted for.
func (r *Registry) Resolve(nodeID core.NodeID, token string) (Role, bool) {
	r.mu.Lock()
	g, ok := r.byNode[nodeID]
	r.mu.Unlock()
	if !ok {
		return "", false
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(g.token)) != 1 {
		return "", false
	}
	return g.role, true
}

// Revoke drops a node's token at its terminal state; later calls with it fail.
func (r *Registry) Revoke(nodeID core.NodeID) {
	r.mu.Lock()
	delete(r.byNode, nodeID)
	r.mu.Unlock()
}

// RoleOf returns a node's minted role, if it has one. The scheduler uses it when
// waking a node to mount the right capability set.
func (r *Registry) RoleOf(nodeID core.NodeID) (Role, bool) {
	r.mu.Lock()
	g, ok := r.byNode[nodeID]
	r.mu.Unlock()
	return g.role, ok
}
