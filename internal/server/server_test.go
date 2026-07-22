package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/api"
	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/gitcli"
	"github.com/AnkushinDaniil/grove/internal/session"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/tree"
	"github.com/AnkushinDaniil/grove/internal/worktree"
	"github.com/AnkushinDaniil/grove/internal/ws"
)

const testToken = "server-test-token"

// buildConfig wires real components into a server.Config bound to addr.
func buildConfig(t *testing.T, addr string) (Config, core.NodeID) {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "grove.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() }) // idempotent; Run may also close it

	tr := tree.New(st)
	root, err := tr.Bootstrap(t.Context(), "Workspace")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	reg, err := driver.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	mgr := session.NewManager(reg, tr, session.Config{ScrollbackDir: t.TempDir()})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
	})
	engine := worktree.NewEngine(gitcli.NewRunner(), t.TempDir(), time.Now)
	auth := api.NewAuth(testToken)

	cfg := Config{
		Addr: addr,
		Auth: auth,
		API: api.New(api.Config{
			Tree: tr, Sessions: mgr, Store: st, Worktrees: engine,
			Auth: auth, HookTokens: api.NewHookTokens(),
			Version: "v", Commit: "c",
		}),
		WS:       ws.New(ws.Config{Tree: tr, Sessions: mgr, Store: st, ScrollbackDir: t.TempDir()}),
		Sessions: mgr,
		Store:    st,
	}
	return cfg, root.ID
}

// serverHarness serves the full middleware stack over httptest.
type serverHarness struct {
	t    *testing.T
	ts   *httptest.Server
	root core.NodeID
}

func newServerHarness(t *testing.T) *serverHarness {
	t.Helper()
	cfg, root := buildConfig(t, "127.0.0.1:0")
	srv := New(cfg)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &serverHarness{t: t, ts: ts, root: root}
}

// request builds a request against the harness server.
func (h *serverHarness) request(method, path, body string, csrf bool) *http.Request {
	h.t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(h.t.Context(), method, h.ts.URL+path, r)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf {
		req.Header.Set(api.CSRFHeader, api.CSRFValue)
	}
	return req
}

// response is a drained HTTP response: the body is read and closed inside do so
// call sites never manage an open body.
type response struct {
	status  int
	body    []byte
	cookies []*http.Cookie
}

func (h *serverHarness) do(req *http.Request) response {
	h.t.Helper()
	resp, err := h.ts.Client().Do(req)
	if err != nil {
		h.t.Fatalf("do %s %s: %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("read body: %v", err)
	}
	return response{status: resp.StatusCode, body: body, cookies: resp.Cookies()}
}

func assertStatus(t *testing.T, resp response, want int) {
	t.Helper()
	if resp.status != want {
		t.Fatalf("status = %d, want %d (body: %s)", resp.status, want, resp.body)
	}
}

func TestAuthGating(t *testing.T) {
	h := newServerHarness(t)

	// Unauthenticated API request is rejected.
	assertStatus(t, h.do(h.request(http.MethodGet, "/api/v1/tree", "", false)), http.StatusUnauthorized)

	// Login is CSRF-exempt and issues the cookie.
	login := h.do(h.request(http.MethodPost, api.PathAuthSession, `{"token":"`+testToken+`"}`, false))
	assertStatus(t, login, http.StatusNoContent)
	var cookie *http.Cookie
	for _, c := range login.cookies {
		if c.Name == "grove_auth" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("login did not set the session cookie")
	}

	// Cookie-authenticated GET succeeds.
	authed := h.request(http.MethodGet, "/api/v1/tree", "", false)
	authed.AddCookie(cookie)
	assertStatus(t, h.do(authed), http.StatusOK)

	// Authenticated mutation without the CSRF header is refused.
	noCSRF := h.request(http.MethodPost, "/api/v1/nodes",
		`{"parent_id":"`+string(h.root)+`","kind":"project","title":"P"}`, false)
	noCSRF.AddCookie(cookie)
	assertStatus(t, h.do(noCSRF), http.StatusForbidden)

	// With the CSRF header it goes through.
	withCSRF := h.request(http.MethodPost, "/api/v1/nodes",
		`{"parent_id":"`+string(h.root)+`","kind":"project","title":"P"}`, true)
	withCSRF.AddCookie(cookie)
	assertStatus(t, h.do(withCSRF), http.StatusCreated)
}

func TestBearerAuth(t *testing.T) {
	h := newServerHarness(t)
	req := h.request(http.MethodGet, "/api/v1/tree", "", false)
	req.Header.Set("Authorization", "Bearer "+testToken)
	assertStatus(t, h.do(req), http.StatusOK)
}

func TestHostAllowlist(t *testing.T) {
	h := newServerHarness(t)

	req := h.request(http.MethodGet, "/", "", false)
	req.Host = "evil.example.com"
	assertStatus(t, h.do(req), http.StatusForbidden)

	// A loopback host is served.
	ok := h.request(http.MethodGet, "/", "", false)
	ok.Host = "localhost:1234"
	assertStatus(t, h.do(ok), http.StatusOK)
}

func TestStaticHintAndLoginPages(t *testing.T) {
	h := newServerHarness(t)

	// Without an embedded UI, the root serves the build hint.
	hint := h.do(h.request(http.MethodGet, "/", "", false))
	body := readBody(t, hint, http.StatusOK)
	if !strings.Contains(body, "grove daemon") {
		t.Errorf("hint page = %q, want it to mention the daemon", body)
	}

	// The login page carries the token-exchange script.
	login := h.do(h.request(http.MethodGet, "/auth", "", false))
	body = readBody(t, login, http.StatusOK)
	if !strings.Contains(body, "/api/v1/auth/session") {
		t.Errorf("login page = %q, want the token exchange", body)
	}
}

func readBody(t *testing.T, resp response, wantStatus int) string {
	t.Helper()
	if resp.status != wantStatus {
		t.Fatalf("status = %d, want %d", resp.status, wantStatus)
	}
	return string(resp.body)
}

func TestHostAllowedHelper(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:7433":                    true,
		"localhost:7433":                    true,
		"127.0.0.1":                         true,
		"localhost":                         true,
		"evil.com:80":                       false,
		"10.0.0.1:7433":                     false,
		"mymachine.tailnet-name.ts.net":     true,
		"mymachine.tailnet-name.ts.net:443": true,
		"evilts.net":                        false, // no dot before the suffix: not *.ts.net
		"attacker.com.ts.net.evil.com":      false, // suffix must be the true tail, not a substring
	}
	for host, want := range cases {
		if got := hostAllowed(host); got != want {
			t.Errorf("hostAllowed(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestServerRunGracefulShutdown(t *testing.T) {
	addr := freeAddr(t)
	cfg, _ := buildConfig(t, addr)
	srv := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Run(ctx) }()

	waitReachable(t, addr)

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	_ = resp.Body.Close()

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run returned error on graceful shutdown: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

// freeAddr returns a probably-free loopback address by binding then releasing a
// port.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// waitReachable blocks until addr accepts a TCP connection.
func waitReachable(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server never became reachable at %s", addr)
}
