package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/AnkushinDaniil/grove/ui"
)

// staticHandler serves the embedded web UI with SPA fallback, or a build hint
// when the UI was not compiled in (plain `go build`, no -tags embedui).
func (s *Server) staticHandler() http.Handler {
	if !ui.Embedded {
		return http.HandlerFunc(handleHint)
	}
	dist, err := fs.Sub(ui.Dist, "dist")
	if err != nil {
		s.logger.Error("open embedded ui", "err", err)
		return http.HandlerFunc(handleHint)
	}
	return spaHandler(dist)
}

// spaHandler serves static files from dist, falling back to index.html for any
// path that is not a real file so the single-page app handles routing
// client-side.
//
// Caching is the load-bearing detail: Vite fingerprints asset filenames
// (index-<hash>.js), so those are safe to cache forever — but index.html
// itself must NEVER be cached, or a browser keeps loading an old bundle after
// the daemon updates. Without this, every grove upgrade silently served a
// stale UI until the user hard-refreshed.
func spaHandler(dist fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if name == "." || name == "/" {
			name = "index.html"
		}
		if f, err := dist.Open(name); err == nil {
			_ = f.Close()
			setCacheHeaders(w, name)
			fileServer.ServeHTTP(w, r)
			return
		}
		index, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

// setCacheHeaders caches fingerprinted assets aggressively and everything else
// (index.html, manifest, root-level files) revalidation-required, so a daemon
// upgrade is picked up on the next page load without a manual hard-refresh.
func setCacheHeaders(w http.ResponseWriter, name string) {
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}

// handleHint serves a minimal page when no UI is embedded.
func handleHint(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(hintHTML))
}

// handleLoginPage serves the token-handoff page `grove open` targets: it reads
// the one-time token from the URL fragment, exchanges it for the session cookie,
// and redirects to the app. Dependency-free inline HTML so it never relies on
// the built UI being present.
func handleLoginPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(loginHTML))
}

const hintHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>grove</title></head>
<body style="font-family: system-ui, sans-serif; max-width: 40rem; margin: 4rem auto; padding: 0 1rem; line-height: 1.5;">
<h1>grove daemon is running</h1>
<p>The web UI is not embedded in this build. To use it:</p>
<ul>
<li>Build a release binary with the UI embedded: <code>make build-release</code></li>
<li>Or run the dev server: <code>cd ui &amp;&amp; npm run dev</code></li>
</ul>
<p>The API is available under <code>/api/v1</code>.</p>
</body>
</html>`

const loginHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>grove — signing in</title></head>
<body style="font-family: system-ui, sans-serif; max-width: 30rem; margin: 4rem auto; padding: 0 1rem;">
<p id="status">Signing in…</p>
<script>
(function () {
  var status = document.getElementById("status");
  var params = new URLSearchParams((location.hash || "").replace(/^#/, ""));
  var token = params.get("t");
  if (!token) { status.textContent = "Missing token."; return; }
  fetch("/api/v1/auth/session", {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Grove-CSRF": "1" },
    credentials: "include",
    body: JSON.stringify({ token: token })
  }).then(function (r) {
    if (r.status === 204) { location.replace("/"); }
    else { status.textContent = "Authentication failed."; }
  }).catch(function () { status.textContent = "Authentication error."; });
})();
</script>
</body>
</html>`
