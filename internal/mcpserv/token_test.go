package mcpserv

import (
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func TestRegistryMintIdempotent(t *testing.T) {
	reg := NewRegistry()
	node := core.NewNodeID()
	a := reg.Mint(node, RoleOrchestrator)
	b := reg.Mint(node, RoleWorker) // role arg ignored once minted
	if a != b {
		t.Fatalf("Mint not idempotent: %q != %q", a, b)
	}
	if role, ok := reg.RoleOf(node); !ok || role != RoleOrchestrator {
		t.Fatalf("RoleOf = %q,%v; want orchestrator,true", role, ok)
	}
}

func TestRegistryResolve(t *testing.T) {
	reg := NewRegistry()
	node := core.NewNodeID()
	other := core.NewNodeID()
	tok := reg.Mint(node, RoleWorker)

	if role, ok := reg.Resolve(node, tok); !ok || role != RoleWorker {
		t.Fatalf("Resolve valid = %q,%v; want worker,true", role, ok)
	}
	if _, ok := reg.Resolve(node, "wrong-token"); ok {
		t.Fatal("Resolve accepted a wrong token")
	}
	if _, ok := reg.Resolve(other, tok); ok {
		t.Fatal("Resolve accepted a foreign node's token (sibling spoofing)")
	}
	if _, ok := reg.Resolve(core.NewNodeID(), "anything"); ok {
		t.Fatal("Resolve accepted an unknown node")
	}
}

func TestRegistryRevoke(t *testing.T) {
	reg := NewRegistry()
	node := core.NewNodeID()
	tok := reg.Mint(node, RoleWorker)
	reg.Revoke(node)
	if _, ok := reg.Resolve(node, tok); ok {
		t.Fatal("Resolve accepted a revoked token")
	}
	// A fresh mint after revoke issues a new token.
	if tok2 := reg.Mint(node, RoleWorker); tok2 == tok {
		t.Fatal("Mint after revoke reused the old token")
	}
}

func TestRoleCapability(t *testing.T) {
	if RoleWorker.CanOrchestrate() {
		t.Fatal("worker should not orchestrate")
	}
	if !RoleOrchestrator.CanOrchestrate() {
		t.Fatal("orchestrator should orchestrate")
	}
}
