package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// hookTokenBytes is the entropy of a per-node hook token.
const hookTokenBytes = 32

// HookTokens is the per-node hook-token registry. A token is minted when a
// session starts for a node and embedded in the driver's hook wiring, so the
// agent authenticates its callbacks to POST /api/v1/internal/hook. It is safe
// for concurrent use.
type HookTokens struct {
	mu     sync.Mutex
	byNode map[core.NodeID]string
}

// NewHookTokens builds an empty registry.
func NewHookTokens() *HookTokens {
	return &HookTokens{byNode: make(map[core.NodeID]string)}
}

// Mint returns the node's hook token, generating and storing one on first use.
// It is idempotent: repeated calls for the same node return the same token so a
// node's hooks keep working across multiple sessions.
func (h *HookTokens) Mint(nodeID core.NodeID) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if tok, ok := h.byNode[nodeID]; ok {
		return tok
	}
	raw := make([]byte, hookTokenBytes)
	_, _ = rand.Read(raw) // crypto/rand.Read never returns an error on supported platforms
	tok := hex.EncodeToString(raw)
	h.byNode[nodeID] = tok
	return tok
}

// Valid reports whether token matches the node's minted hook token (constant
// time). It returns false for nodes that have never minted a token.
func (h *HookTokens) Valid(nodeID core.NodeID, token string) bool {
	h.mu.Lock()
	tok, ok := h.byNode[nodeID]
	h.mu.Unlock()
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(tok)) == 1
}
