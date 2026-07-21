package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxDirSuggestions caps a single completion response; terminal-style tab
// completion never needs the full listing of a huge directory.
const maxDirSuggestions = 50

// dirsResponse is the GET /fs/dirs body: the completion candidates plus the
// resolved home directory (so the client can render and expand "~").
type dirsResponse struct {
	Dirs []string `json:"dirs"`
	Home string   `json:"home"`
}

// handleFsDirs powers terminal-style work_dir tab-completion: given a partial
// path prefix it lists the sibling directories that could complete it, mirroring
// shell completion semantics.
//
// Trust model: grove is a single-user daemon bound to 127.0.0.1 that exposes the
// authenticated user's own filesystem back to that same user. Directory
// traversal "outside a root" is therefore a non-concern by design -- there is no
// boundary to escape and nothing the caller could not already read directly. The
// only rejected input is a NUL byte, which no valid path contains and which would
// otherwise reach the syscall layer as a malformed argument.
func (h *Handlers) handleFsDirs(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	if strings.IndexByte(prefix, 0) >= 0 {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "prefix must not contain a NUL byte")
		return
	}

	home, err := h.home()
	if err != nil {
		// Home is best-effort context for the client (and the target for "~"/
		// empty expansion); its absence never fails completion.
		home = ""
	}

	writeJSON(w, h.logger, http.StatusOK, dirsResponse{
		Dirs: completeDirs(prefix, home),
		Home: home,
	})
}

// completeDirs returns the absolute directory paths under prefix's parent whose
// final segment case-insensitively prefix-matches, capped and sorted. An
// unreadable or nonexistent parent yields an empty (non-nil) slice: completion
// simply has nothing to offer, which is not an error.
func completeDirs(prefix, home string) []string {
	parent, base := splitCompletion(prefix, home)

	entries, err := os.ReadDir(parent)
	if err != nil {
		return []string{}
	}

	// Hidden entries (leading ".") appear only when the typed segment itself
	// starts with ".", matching shell completion.
	includeHidden := strings.HasPrefix(base, ".")
	lowerBase := strings.ToLower(base)

	dirs := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !includeHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(name), lowerBase) {
			continue
		}
		if !entryIsDir(parent, e) {
			continue
		}
		dirs = append(dirs, filepath.Join(parent, name))
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j])
	})
	if len(dirs) > maxDirSuggestions {
		dirs = dirs[:maxDirSuggestions]
	}
	return dirs
}

// splitCompletion expands "~"/empty prefixes against home, then splits the
// result into the parent directory to list and the base segment to filter by. A
// trailing slash (or an empty prefix, treated as home + "/") lists the whole
// directory with an empty base; otherwise the last path element is the base.
func splitCompletion(prefix, home string) (parent, base string) {
	switch {
	case prefix == "":
		return home, ""
	case prefix == "~":
		prefix = home
	case strings.HasPrefix(prefix, "~/"):
		prefix = home + prefix[1:] // replace "~" with home, keeping the rest (incl. "/")
	case !filepath.IsAbs(prefix):
		// Bare relative input is treated as home-relative, matching the
		// work_dir normalization rules — suggestions always come back absolute.
		trailing := strings.HasSuffix(prefix, "/")
		prefix = filepath.Join(home, prefix)
		if trailing {
			prefix += "/"
		}
	}
	if strings.HasSuffix(prefix, "/") {
		return prefix, ""
	}
	return filepath.Dir(prefix), filepath.Base(prefix)
}

// entryIsDir reports whether a directory entry should complete as a directory:
// a real directory, or a symlink whose target resolves to one. It never recurses
// into the entry -- only its own type matters.
func entryIsDir(parent string, e os.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&os.ModeSymlink == 0 {
		return false
	}
	//nolint:gosec // G703: single-user localhost daemon resolving a symlink under the authenticated user's own filesystem; path traversal is a non-concern by design (see handler trust-model comment).
	info, err := os.Stat(filepath.Join(parent, e.Name()))
	return err == nil && info.IsDir()
}
