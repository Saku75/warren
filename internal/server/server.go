// Package server wires Warren's HTTP surface: middleware, the REST API,
// the HTMX UI, health probes, and the graceful-shutdown lifecycle that
// rolling deployments depend on.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saku75/warren/internal/api"
	"github.com/saku75/warren/internal/changelog"
	"github.com/saku75/warren/internal/config"
	"github.com/saku75/warren/internal/db"
	"github.com/saku75/warren/internal/db/gen"
	"github.com/saku75/warren/internal/dcim"
	"github.com/saku75/warren/internal/tenancy"
	"github.com/saku75/warren/internal/web"
)

// Server is Warren's HTTP server.
type Server struct {
	cfg     config.Config
	log     *slog.Logger
	version string
	pool    *pgxpool.Pool
	http    *http.Server
}

// New assembles services and routes onto a Server ready to Run.
func New(cfg config.Config, log *slog.Logger, version string, pool *pgxpool.Pool) *Server {
	s := &Server{cfg: cfg, log: log, version: version, pool: pool}

	tenancySvc := tenancy.NewService(pool)
	dcimSvc := dcim.NewService(pool)
	changelogSvc := changelog.NewService(gen.New(pool))

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Probes: /healthz is liveness (process up); /readyz gates on database
	// reachability and schema currency so replicas built for a different
	// schema stay out of rotation.
	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)

	// Vendored static assets (htmx, stylesheet), served from the binary so
	// the container needs no CDN and no filesystem.
	r.Handle("/assets/*", http.StripPrefix("/assets/", web.AssetHandler()))

	// REST API v1.
	r.Mount("/api/v1", api.New(log, tenancySvc, dcimSvc, changelogSvc).Routes())

	// HTMX UI. CSRF protection lands together with sessions/auth.
	ui := &uiHandler{log: log, tenancy: tenancySvc, dcim: dcimSvc, changelog: changelogSvc}
	ui.routes(r)
	r.Get("/", s.handleHome)

	s.http = &http.Server{
		Addr:              cfg.Listen,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// Run serves until ctx is cancelled (SIGTERM/SIGINT), then drains in-flight
// requests for up to the configured grace period. Replicas behind a load
// balancer rely on this to roll without dropped requests.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("listening", "addr", s.cfg.Listen, "version", s.version)
		if err := s.http.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	s.log.Info("shutting down", "grace", s.cfg.ShutdownGrace)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownGrace)
	defer cancel()
	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return <-errCh
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := db.Ready(r.Context(), s.pool); err != nil {
		s.log.Warn("readiness check failed", "err", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready"))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if err := web.Home(s.version).Render(r.Context(), w); err != nil {
		s.log.Error("render home", "err", err)
	}
}

// requestLogger emits one structured line per request.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration", time.Since(start),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
