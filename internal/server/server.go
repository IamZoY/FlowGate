package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ali/flowgate/internal/config"
)

// Server wraps http.Server with config-driven timeouts and optional TLS.
type Server struct {
	srv     *http.Server
	tlsCert string
	tlsKey  string
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
		tlsCert: cfg.TLSCert,
		tlsKey:  cfg.TLSKey,
	}
}

// Start listens on the configured address. Uses TLS if cert and key are set.
func (s *Server) Start() error {
	if s.tlsCert != "" && s.tlsKey != "" {
		return s.srv.ListenAndServeTLS(s.tlsCert, s.tlsKey)
	}
	return s.srv.ListenAndServe()
}

// TLSEnabled reports whether the server is configured to use TLS.
func (s *Server) TLSEnabled() bool {
	return s.tlsCert != "" && s.tlsKey != ""
}

// Shutdown gracefully drains connections using the provided context.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
