package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// fsHandlers builds a bare Handlers wired only for filesystem completion, with a
// fixed fake home so tests never depend on the real $HOME.
func fsHandlers(home string) *Handlers {
	return New(Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Home:   func() (string, error) { return home, nil },
	})
}

// getDirs invokes handleFsDirs for prefix and returns the status and decoded body.
func getDirs(t *testing.T, h *Handlers, prefix string) (int, dirsResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fs/dirs?prefix="+url.QueryEscape(prefix), nil)
	rec := httptest.NewRecorder()
	h.handleFsDirs(rec, req)

	var body dirsResponse
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body %q: %v", rec.Body.Bytes(), err)
		}
	}
	return rec.Code, body
}

// fsTree lays out a directory tree under t.TempDir() used across the completion
// cases and returns (root, home). home is root/home; root also holds a sibling
// "other" directory so "~" expansion has something to filter against.
//
//	root/
//	  home/
//	    Alpha/  beta/  Gamma/  .hidden/  file.txt  linkdir -> Gamma  linkfile -> file.txt
//	  other/
func fsTree(t *testing.T) (root, home string) {
	t.Helper()
	root = t.TempDir()
	home = filepath.Join(root, "home")

	for _, d := range []string{
		filepath.Join(home, "Alpha"),
		filepath.Join(home, "beta"),
		filepath.Join(home, "Gamma"),
		filepath.Join(home, ".hidden"),
		filepath.Join(root, "other"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Symlink(filepath.Join(home, "Gamma"), filepath.Join(home, "linkdir")); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}
	if err := os.Symlink(filepath.Join(home, "file.txt"), filepath.Join(home, "linkfile")); err != nil {
		t.Fatalf("symlink file: %v", err)
	}
	return root, home
}

func TestFsDirsCompletion(t *testing.T) {
	root, home := fsTree(t)
	h := fsHandlers(home)

	// Non-hidden directories inside home, case-insensitively sorted; symlink to a
	// dir is included, files and symlinks to files are not.
	homeDirs := []string{
		filepath.Join(home, "Alpha"),
		filepath.Join(home, "beta"),
		filepath.Join(home, "Gamma"),
		filepath.Join(home, "linkdir"),
	}

	cases := []struct {
		name   string
		prefix string
		want   []string
	}{
		{"empty prefix lists home", "", homeDirs},
		{"trailing slash lists all", home + "/", homeDirs},
		{"tilde-slash expands to home", "~/", homeDirs},
		{"tilde alone completes to home path", "~", []string{home}},
		{"tilde-slash mid-name filters", "~/G", []string{filepath.Join(home, "Gamma")}},
		{"mid-name prefix case-insensitive", filepath.Join(home, "a"), []string{filepath.Join(home, "Alpha")}},
		{"hidden shown when base starts with dot", home + "/.", []string{filepath.Join(home, ".hidden")}},
		{"nonexistent parent is empty", filepath.Join(home, "nope") + "/", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := getDirs(t, h, tc.prefix)
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200", status)
			}
			if body.Home != home {
				t.Errorf("home = %q, want %q", body.Home, home)
			}
			if !equalStrings(body.Dirs, tc.want) {
				t.Errorf("dirs = %v, want %v", body.Dirs, tc.want)
			}
		})
	}

	// "other" is a sibling of home, so "~" (expanded to the home path) filters
	// root's entries by base "home" and must not surface it.
	_, tildeBody := getDirs(t, h, "~")
	for _, d := range tildeBody.Dirs {
		if d == filepath.Join(root, "other") {
			t.Errorf("~ completion leaked sibling %q", d)
		}
	}
}

func TestFsDirsCapEnforced(t *testing.T) {
	home := t.TempDir()
	for i := range 60 {
		if err := os.Mkdir(filepath.Join(home, "d"+strconv.Itoa(i)), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	h := fsHandlers(home)

	status, body := getDirs(t, h, home+"/")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(body.Dirs) != maxDirSuggestions {
		t.Fatalf("len(dirs) = %d, want %d", len(body.Dirs), maxDirSuggestions)
	}
}

func TestFsDirsRejectsNulByte(t *testing.T) {
	h := fsHandlers(t.TempDir())
	status, _ := getDirs(t, h, "/tmp/a\x00b")
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestFsDirsEmptyIsNonNil(t *testing.T) {
	// A nonexistent parent must still serialize dirs as [] (never null) so the
	// client can iterate unconditionally.
	h := fsHandlers(t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fs/dirs?prefix="+url.QueryEscape("/definitely/not/here/"), nil)
	rec := httptest.NewRecorder()
	h.handleFsDirs(rec, req)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(raw["dirs"]) != "[]" {
		t.Errorf("dirs = %s, want []", raw["dirs"])
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
