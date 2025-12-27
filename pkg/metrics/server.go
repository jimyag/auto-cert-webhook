package metrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"k8s.io/klog/v2"
)

// ServerConfig holds configuration for the metrics server.
type ServerConfig struct {
	// Port is the port to listen on.
	Port int

	// Path is the path to serve metrics on.
	Path string
}

// Server is a dedicated HTTP server for serving Prometheus metrics.
type Server struct {
	config ServerConfig
	server *http.Server
}

// NewServer creates a new metrics server.
func NewServer(config ServerConfig) *Server {
	if config.Path == "" {
		config.Path = "/metrics"
	}

	return &Server{
		config: config,
	}
}

// Start starts the metrics server and blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	Register()

	mux := http.NewServeMux()
	mux.Handle(s.config.Path, Handler())

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.config.Port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	klog.Infof("Starting metrics server on port %d", s.config.Port)

	errCh := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		klog.Info("Shutting down metrics server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
