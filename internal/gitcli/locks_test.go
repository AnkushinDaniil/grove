package gitcli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLockSerializesSameRepo(t *testing.T) {
	path := t.TempDir()

	unlock1 := Lock(path)

	acquired := make(chan struct{})
	go func() {
		unlock2 := Lock(path)
		close(acquired)
		unlock2()
	}()

	select {
	case <-acquired:
		t.Fatal("second Lock on the same repo path was acquired while the first was still held")
	case <-time.After(50 * time.Millisecond):
		// expected: still blocked
	}

	unlock1()

	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("second Lock never acquired after the first was released")
	}
}

func TestLockDifferentReposIndependent(t *testing.T) {
	p1 := t.TempDir()
	p2 := t.TempDir()

	unlock1 := Lock(p1)
	defer unlock1()

	done := make(chan struct{})
	go func() {
		unlock2 := Lock(p2)
		defer unlock2()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Lock on a different repo path blocked; expected independent locks")
	}
}

func TestLockCanonicalizesSymlinks(t *testing.T) {
	real := t.TempDir()
	parent := t.TempDir()
	link := filepath.Join(parent, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks not supported in this environment: %v", err)
	}

	unlock1 := Lock(real)

	acquired := make(chan struct{})
	go func() {
		unlock2 := Lock(link)
		close(acquired)
		unlock2()
	}()

	select {
	case <-acquired:
		t.Fatal("Lock(symlink) did not serialize with Lock(target); canonicalization failed")
	case <-time.After(50 * time.Millisecond):
		// expected: still blocked, they share one mutex
	}

	unlock1()

	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("second Lock never acquired after the first was released")
	}
}
