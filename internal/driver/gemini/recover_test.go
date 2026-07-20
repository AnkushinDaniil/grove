package gemini

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

const testProjectRoot = "/home/user/repo"

// chatFile builds a fstest.MapFile for a chats/ entry, mirroring
// chatRecordingService.ts's initial metadata line as the file's first (and,
// for these tests, only) JSONL record.
func chatFile(sessionID string, modTime time.Time) *fstest.MapFile {
	content := `{"sessionId":"` + sessionID + `","projectHash":"irrelevant-here","startTime":"2026-07-20T00:00:00.000Z"}` + "\n"
	return &fstest.MapFile{Data: []byte(content), ModTime: modTime}
}

func TestRecoverSessionIDFromFSMarkerBased(t *testing.T) {
	startedAt := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		"tmp/my-repo/.project_root":                       &fstest.MapFile{Data: []byte(testProjectRoot + "\n")},
		"tmp/my-repo/chats/session-old.jsonl":             chatFile("old-id", startedAt.Add(-1*time.Hour)),
		"tmp/my-repo/chats/session-new.jsonl":             chatFile("new-id", startedAt.Add(1*time.Second)),
		"tmp/unrelated-project/.project_root":             &fstest.MapFile{Data: []byte("/some/other/repo\n")},
		"tmp/unrelated-project/chats/session-other.jsonl": chatFile("other-id", startedAt.Add(1*time.Second)),
	}

	id, err := recoverSessionIDFromFS(fsys, testProjectRoot, startedAt)
	if err != nil {
		t.Fatalf("recoverSessionIDFromFS() error = %v", err)
	}
	if id != "new-id" {
		t.Errorf("recoverSessionIDFromFS() = %q, want %q (the file modified after startedAt, not the older one or the other project's)", id, "new-id")
	}
}

func TestRecoverSessionIDFromFSHashFallback(t *testing.T) {
	startedAt := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	hash := sha256Hex(testProjectRoot)
	fsys := fstest.MapFS{
		// No .project_root marker anywhere: simulates an install that has
		// not migrated off the legacy raw-hash directory naming.
		"tmp/" + hash + "/chats/session-legacy.jsonl": chatFile("legacy-id", startedAt.Add(1*time.Second)),
	}

	id, err := recoverSessionIDFromFS(fsys, testProjectRoot, startedAt)
	if err != nil {
		t.Fatalf("recoverSessionIDFromFS() error = %v", err)
	}
	if id != "legacy-id" {
		t.Errorf("recoverSessionIDFromFS() = %q, want %q", id, "legacy-id")
	}
}

func TestRecoverSessionIDFromFSAmbiguous(t *testing.T) {
	startedAt := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		"tmp/my-repo/.project_root":         &fstest.MapFile{Data: []byte(testProjectRoot)},
		"tmp/my-repo/chats/session-a.jsonl": chatFile("id-a", startedAt.Add(1*time.Second)),
		"tmp/my-repo/chats/session-b.jsonl": chatFile("id-b", startedAt.Add(2*time.Second)),
	}

	_, err := recoverSessionIDFromFS(fsys, testProjectRoot, startedAt)
	if !errors.Is(err, ErrAmbiguousSession) {
		t.Fatalf("recoverSessionIDFromFS() error = %v, want ErrAmbiguousSession", err)
	}
}

func TestRecoverSessionIDFromFSNotFoundNoProjectDir(t *testing.T) {
	startedAt := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		"tmp/some-other-project/.project_root": &fstest.MapFile{Data: []byte("/some/other/repo")},
	}

	_, err := recoverSessionIDFromFS(fsys, testProjectRoot, startedAt)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("recoverSessionIDFromFS() error = %v, want ErrSessionNotFound", err)
	}
}

func TestRecoverSessionIDFromFSNotFoundNoNewFiles(t *testing.T) {
	startedAt := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		"tmp/my-repo/.project_root":           &fstest.MapFile{Data: []byte(testProjectRoot)},
		"tmp/my-repo/chats/session-old.jsonl": chatFile("old-id", startedAt.Add(-1*time.Hour)),
	}

	_, err := recoverSessionIDFromFS(fsys, testProjectRoot, startedAt)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("recoverSessionIDFromFS() error = %v, want ErrSessionNotFound", err)
	}
}

func TestRecoverSessionIDFromFSIgnoresNonSessionFiles(t *testing.T) {
	startedAt := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		"tmp/my-repo/.project_root":             &fstest.MapFile{Data: []byte(testProjectRoot)},
		"tmp/my-repo/chats/session-new.jsonl":   chatFile("new-id", startedAt.Add(1*time.Second)),
		"tmp/my-repo/chats/.DS_Store":           &fstest.MapFile{Data: []byte("junk"), ModTime: startedAt.Add(1 * time.Second)},
		"tmp/my-repo/chats/subdir/nested.jsonl": chatFile("nested-id", startedAt.Add(1*time.Second)),
	}

	id, err := recoverSessionIDFromFS(fsys, testProjectRoot, startedAt)
	if err != nil {
		t.Fatalf("recoverSessionIDFromFS() error = %v", err)
	}
	if id != "new-id" {
		t.Errorf("recoverSessionIDFromFS() = %q, want %q (non-.json/.jsonl files and subdirectories must not count as candidates)", id, "new-id")
	}
}

func TestRecoverSessionIDFromFSMalformedFirstLine(t *testing.T) {
	startedAt := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		"tmp/my-repo/.project_root":           &fstest.MapFile{Data: []byte(testProjectRoot)},
		"tmp/my-repo/chats/session-new.jsonl": {Data: []byte(""), ModTime: startedAt.Add(1 * time.Second)},
	}

	_, err := recoverSessionIDFromFS(fsys, testProjectRoot, startedAt)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("recoverSessionIDFromFS() error = %v, want ErrSessionNotFound (empty file: writer has not flushed the metadata line yet)", err)
	}
}

// TestRecoverSessionIDPollsUntilFound exercises the real RecoverSessionID
// method (not just the pure helper): the fake filesystem starts empty, and
// a background goroutine "creates" the session file shortly after, proving
// the poll loop retries on ErrSessionNotFound instead of failing on the
// first pass.
func TestRecoverSessionIDPollsUntilFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}
	startedAt := time.Now()
	chatsDir := filepath.Join(home, ".gemini", "tmp", sha256Hex(filepath.Clean("/work")), "chats")
	if err := os.MkdirAll(chatsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	go func() {
		time.Sleep(150 * time.Millisecond)
		content := `{"sessionId":"delayed-id","projectHash":"x","startTime":"2026-07-20T00:00:00.000Z"}` + "\n"
		_ = os.WriteFile(filepath.Join(chatsDir, "session-delayed.jsonl"), []byte(content), 0o644)
		now := time.Now()
		_ = os.Chtimes(filepath.Join(chatsDir, "session-delayed.jsonl"), now, now)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	id, err := New().RecoverSessionID(ctx, driver.SessionInfo{CWD: "/work", StartedAt: startedAt})
	if err != nil {
		t.Fatalf("RecoverSessionID() error = %v", err)
	}
	if id != "delayed-id" {
		t.Errorf("RecoverSessionID() = %q, want %q", id, "delayed-id")
	}
}

func TestRecoverSessionIDContextCancelled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := New().RecoverSessionID(ctx, driver.SessionInfo{CWD: "/nonexistent", StartedAt: time.Now()})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RecoverSessionID() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestFindChatsDirByMarkerNoTmpDir(t *testing.T) {
	fsys := fstest.MapFS{}
	_, ok := findChatsDirByMarker(fsys, testProjectRoot)
	if ok {
		t.Error("findChatsDirByMarker() ok = true, want false when tmp/ does not exist")
	}
}

func TestSHA256Hex(t *testing.T) {
	// Known SHA-256 of "" (empty string) as a sanity pin against the
	// standard algorithm gemini-cli itself uses (crypto.createHash('sha256')).
	got := sha256Hex("")
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != want {
		t.Errorf("sha256Hex(\"\") = %q, want %q", got, want)
	}
}
