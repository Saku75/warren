// Package server wires Warren's HTTP surface: middleware, routes, health
// probes, and the graceful-shutdown lifecycle that rolling deployments
// depend on.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/saku75/warren/internal/config"
	"github.com/saku75/warren/internal/web"
)

// Server is Warren's HTTP server.
type Server struct {
	cfg     config.Config
	log     *slog.Logger
	version string
	http    *http.Server
}

// New assembles the router and returns a Server ready to Run.
func New(cfg config.Config, log *slog.Logger, version string) *Server {
	s := &Server{cfg: cfg, log: log, version: version}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Probes. Liveness means the process is up; readiness will also gate on
	// database connectivity and schema currency once the first schema lands.
	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)

	// Vendored static assets (htmx, stylesheet), served from the binary so
	// the container needs no CDN and no filesystem.
	r.Handle("/assets/*", http.StripPrefix("/assets/", web.AssetHandler()))

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

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	// TODO(phase-1): ping PostgreSQL and verify schema version here.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
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
