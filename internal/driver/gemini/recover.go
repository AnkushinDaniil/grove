package gemini

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

// ErrSessionNotFound is returned by RecoverSessionID (and, wrapped, by its
// helpers) when no chat file created since the run started can be found
// yet; the poll loop in RecoverSessionID retries on this specific error.
var ErrSessionNotFound = errors.New("gemini: no session file found")

// ErrAmbiguousSession is returned by RecoverSessionID when more than one
// chat file was modified since the run started, so the newly created
// session cannot be told apart from another candidate. gemini-cli's
// documented headless JSON schema omits the session id (issue #14435), so
// grove locates it out-of-band by directory scanning instead; this is the
// concurrency hazard that forces on the session manager, which is expected
// to serialize gemini spawns per profile so this stays rare.
var ErrAmbiguousSession = errors.New("gemini: ambiguous session (multiple candidate files)")

// pollInterval is how often RecoverSessionID re-scans the chats directory
// while waiting for gemini-cli's chat-recording file to appear and become
// readable after spawn.
const pollInterval = 100 * time.Millisecond

// RecoverSessionID is gemini's issue #14435 workaround: the documented
// headless JSON schema (docs/cli/headless.md) does not include a session
// id, so grove locates the session-recording file gemini-cli itself wrote
// instead (docs/cli/session-management.md: "Sessions are stored in
// ~/.gemini/tmp/<project_hash>/chats/"). It polls until ctx is done, a
// single new session file is found and its metadata line parses, or a
// second concurrent candidate makes the result ambiguous.
func (geminiDriver) RecoverSessionID(ctx context.Context, info driver.SessionInfo) (string, error) {
	baseDir := info.ConfigDir
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		baseDir = filepath.Join(home, ".gemini")
	}
	fsys := os.DirFS(baseDir)

	for {
		id, err := recoverSessionIDFromFS(fsys, info.CWD, info.StartedAt)
		switch {
		case err == nil:
			return id, nil
		case errors.Is(err, ErrAmbiguousSession):
			return "", err
		case errors.Is(err, ErrSessionNotFound):
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(pollInterval):
			}
		default:
			return "", err
		}
	}
}

// recoverSessionIDFromFS is the pure, single-pass core of RecoverSessionID:
// no sleeping, no polling, operating over an fs.FS so tests can substitute a
// fake directory tree (fstest.MapFS) instead of the real filesystem.
func recoverSessionIDFromFS(fsys fs.FS, projectRoot string, startedAt time.Time) (string, error) {
	chatsDir, err := findChatsDir(fsys, projectRoot)
	if err != nil {
		return "", err
	}

	entries, err := fs.ReadDir(fsys, chatsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ErrSessionNotFound
		}
		return "", fmt.Errorf("read gemini chats dir %s: %w", chatsDir, err)
	}

	var candidate string
	found := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") && !strings.HasSuffix(name, ".json") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue // can't stat it; skip rather than fail the whole scan.
		}
		if fi.ModTime().Before(startedAt) {
			continue
		}
		found++
		candidate = name
	}

	switch found {
	case 0:
		return "", ErrSessionNotFound
	case 1:
		return readSessionID(fsys, path.Join(chatsDir, candidate))
	default:
		return "", ErrAmbiguousSession
	}
}

// findChatsDir locates the project's chats/ directory under fsys (rooted at
// the gemini config dir). gemini-cli's project-directory naming has changed
// at least once (a raw sha256(path) hex digest, migrated to a short
// human-readable slug tracked in a projects.json registry, with a
// ".project_root" ownership-marker file written into each candidate
// directory) so both schemes are tried here: the marker file is
// authoritative when present, since it is the actual mapping gemini-cli
// itself wrote — no slug-assignment algorithm needs replicating — falling
// back to the legacy raw-hash directory name for installs that have not
// migrated.
//
// ASSUMPTION(verify-on-install): this two-tier lookup is the most
// defensible reading of gemini-cli's current source
// (packages/core/src/config/{storage,projectRegistry}.ts); a future release
// could change the scheme again. Both branches degrade to
// ErrSessionNotFound rather than erroring when they find nothing.
func findChatsDir(fsys fs.FS, projectRoot string) (string, error) {
	normalized := filepath.Clean(projectRoot)

	if dir, ok := findChatsDirByMarker(fsys, normalized); ok {
		return dir, nil
	}

	legacy := path.Join("tmp", sha256Hex(normalized), "chats")
	if _, err := fs.Stat(fsys, legacy); err == nil {
		return legacy, nil
	}

	return "", ErrSessionNotFound
}

// findChatsDirByMarker scans tmp/*/.project_root for a marker file whose
// content is projectRoot, matching gemini-cli's ProjectRegistry ownership
// markers (packages/core/src/config/projectRegistry.ts,
// ensureOwnershipMarkers/verifySlugOwnership).
func findChatsDirByMarker(fsys fs.FS, projectRoot string) (string, bool) {
	entries, err := fs.ReadDir(fsys, "tmp")
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		markerPath := path.Join("tmp", e.Name(), ".project_root")
		content, err := fs.ReadFile(fsys, markerPath)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(content)) == projectRoot {
			return path.Join("tmp", e.Name(), "chats"), true
		}
	}
	return "", false
}

// sha256Hex mirrors gemini-cli's legacy per-project directory name
// (packages/core/src/config/storage.ts, getFilePathHash:
// sha256(path).hex()).
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// sessionMetaLine is the first JSONL record gemini-cli writes to a new chat
// file (packages/core/src/services/chatRecordingService.ts,
// initialMetadata) before any conversation turns. The full session id lives
// only here — the filename itself carries just an 8-character prefix
// (session-<minute-timestamp>-<id[:8]>.jsonl), not enough to resume by.
type sessionMetaLine struct {
	SessionID string `json:"sessionId"`
}

// readSessionID reads the first non-blank line of a chats/ file and
// extracts its sessionId. A file that exists but whose first line is not
// yet a parseable metadata record — the writer may still be mid-write right
// after gemini-cli creates the file — is reported as ErrSessionNotFound so
// the caller's poll loop retries rather than returning a wrong answer.
func readSessionID(fsys fs.FS, filePath string) (string, error) {
	f, err := fsys.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("%w: open %s: %v", ErrSessionNotFound, filePath, err) //nolint:errorlint // deliberately not wrapping the fs error itself; only ErrSessionNotFound needs to be errors.Is-able here.
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxBufferBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var meta sessionMetaLine
		if err := json.Unmarshal(line, &meta); err != nil || meta.SessionID == "" {
			return "", fmt.Errorf("%w: %s has no parseable sessionId on its first line", ErrSessionNotFound, filePath)
		}
		return meta.SessionID, nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan %s: %w", filePath, err)
	}
	return "", fmt.Errorf("%w: %s is empty", ErrSessionNotFound, filePath)
}
