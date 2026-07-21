// Package server wires the REST API, the WebSocket endpoints, static UI serving
// and the security middleware into one http.Server, and owns the daemon's
// graceful startup and shutdown.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/AnkushinDaniil/grove/internal/api"
	"github.com/AnkushinDaniil/grove/internal/notify"
	"github.com/AnkushinDaniil/grove/internal/session"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/ws"
)

const (
	// shutdownTimeout bounds graceful shutdown of the HTTP server and sessions.
	shutdownTimeout = 10 * time.Second
	// readHeaderTimeout guards against slow-header (Slowloris) clients.
	readHeaderTimeout = 10 * time.Second
)

// Config carries the Server dependencies.
type Config struct {
	Addr     string // listen address, e.g. 127.0.0.1:7433
	Auth     *api.Auth
	API      *api.Handlers
	WS       *ws.Handlers
	Sessions *session.Manager
	Store    *store.Store
	Logger   *slog.Logger
	// Notify watches the tree for attention transitions and dispatches desktop
	// notifications. Nil disables notifications.
	Notify *notify.Runner
}

// Server binds the daemon's HTTP surface and manages its lifecycle.
type Server struct {
	httpServer *http.Server
	auth       *api.Auth
	sessions   *session.Manager
	store      *store.Store
	logger     *slog.Logger
	notify     *notify.Runner

	baseCtx    context.Context
	baseCancel context.CancelFunc
	// notifyDone closes when the notification runner has stopped; it is closed
	// immediately when no runner is configured so shutdown never blocks on it.
	notifyDone chan struct{}
}

// New assembles the routing tree and returns a ready-to-run Server.
func New(cfg Config) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	baseCtx, baseCancel := context.WithCancel(context.Background())
	s := &Server{
		auth:       cfg.Auth,
		sessions:   cfg.Sessions,
		store:      cfg.Store,
		logger:     logger,
		notify:     cfg.Notify,
		baseCtx:    baseCtx,
		baseCancel: baseCancel,
		notifyDone: make(chan struct{}),
	}

	root := http.NewServeMux()
	root.Handle("/api/v1/", s.guard(cfg.API.Routes()))
	root.Handle("/ws/", s.guard(cfg.WS.Routes()))
	root.HandleFunc("GET /auth", handleLoginPage)
	root.Handle("/", s.staticHandler())

	s.httpServer = &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.hostGuard(root),
		ReadHeaderTimeout: readHeaderTimeout,
		// Derive every request (and hijacked WebSocket) context from baseCtx so
		// shutdown can signal long-lived WebSocket handlers to close.
		BaseContext: func(_ net.Listener) context.Context { return baseCtx },
	}
	return s
}

// Handler returns the fully wrapped HTTP handler (host allowlist, auth, routes,
// static UI). Exposed so tests can drive the whole middleware stack via httptest
// without binding a socket.
func (s *Server) Handler() http.Handler { return s.httpServer.Handler }

// Run serves until ctx is canceled, then shuts down gracefully: it stops
// accepting connections, signals WebSocket handlers, drains in-flight requests,
// stops live sessions, and closes the store.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		err := s.httpServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	s.logger.Info("grove daemon listening", "addr", s.httpServer.Addr)
	s.startNotify()

	select {
	case err := <-errCh:
		// Server stopped on its own (e.g. the port was already in use). Stop the
		// notification runner too, since we return without a graceful shutdown.
		s.baseCancel()
		<-s.notifyDone
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	//nolint:contextcheck // shutdown deliberately uses a fresh context: the run context is already canceled.
	return s.shutdown(errCh)
}

// startNotify launches the attention-notification runner bound to the server
// lifetime (baseCtx). With no runner configured it closes notifyDone at once so
// shutdown never waits on it.
func (s *Server) startNotify() {
	if s.notify == nil {
		close(s.notifyDone)
		return
	}
	go func() {
		defer close(s.notifyDone)
		s.notify.Run(s.baseCtx)
	}()
}

// shutdown performs the ordered graceful teardown and joins any errors.
func (s *Server) shutdown(errCh <-chan error) error {
	s.logger.Info("grove daemon shutting down")
	// Signal WebSocket handlers (hijacked connections http.Server.Shutdown does
	// not track) and the notification runner to close.
	s.baseCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	var errs []error
	select {
	case <-s.notifyDone:
	case <-shutdownCtx.Done():
		errs = append(errs, fmt.Errorf("notify shutdown: %w", shutdownCtx.Err()))
	}
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		errs = append(errs, fmt.Errorf("http shutdown: %w", err))
	}
	if err := s.sessions.Shutdown(shutdownCtx); err != nil {
		errs = append(errs, fmt.Errorf("sessions shutdown: %w", err))
	}
	if err := s.store.Close(); err != nil {
		errs = append(errs, fmt.Errorf("store close: %w", err))
	}
	if err := <-errCh; err != nil {
		errs = append(errs, fmt.Errorf("serve: %w", err))
	}
	return errors.Join(errs...)
}
