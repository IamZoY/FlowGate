package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ali/flowgate/internal/config"
)

// Server wraps http.Server with config-driven timeouts.
type Server struct {
	srv *http.Server
}

// New creates a Server bound to cfg.Host:cfg.Port with the given handler.
func New(cfg config.ServerConfig, handler http.Handler) *Server {
	return &Server{
		srv: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout.Duration,
			WriteTimeout: cfg.WriteTimeout.Duration,
			IdleTimeout:  cfg.IdleTimeout.Duration,
		},
	}
}

// Start calls ListenAndServe. It returns http.ErrServerClosed on graceful stop.
func (s *Server) Start() error {
	return s.srv.ListenAndServe()
}

// Shutdown gracefully drains connections using the provided context.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
