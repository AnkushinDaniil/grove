package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestSPAHandler(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":    {Data: []byte("<html>grove app</html>")},
		"assets/app.js": {Data: []byte("console.log('grove')")},
	}
	ts := httptest.NewServer(spaHandler(dist))
	t.Cleanup(ts.Close)

	get := func(path string) (int, string) {
		t.Helper()
		resp, err := ts.Client().Get(ts.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	// A real asset is served as-is.
	if status, body := get("/assets/app.js"); status != http.StatusOK || body != "console.log('grove')" {
		t.Errorf("asset = %d %q, want 200 with the JS", status, body)
	}
	// Root serves index.html.
	if status, body := get("/"); status != http.StatusOK || body != "<html>grove app</html>" {
		t.Errorf("root = %d %q, want index.html", status, body)
	}
	// An unknown client-side route falls back to index.html.
	if status, body := get("/nodes/abc/deep/link"); status != http.StatusOK || body != "<html>grove app</html>" {
		t.Errorf("spa route = %d %q, want index.html fallback", status, body)
	}
}
