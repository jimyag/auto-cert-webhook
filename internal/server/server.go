package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/klog/v2"

	"github.com/jimyag/auto-cert-webhook/internal/certprovider"
)

// HookType defines the type of admission webhook.
type HookType string

const (
	// Mutating indicates a mutating admission webhook.
	Mutating HookType = "Mutating"
	// Validating indicates a validating admission webhook.
	Validating HookType = "Validating"
)

// AdmitFunc is the function signature for handling admission requests.
// This is defined here to match the public API type signature.
type AdmitFunc = func(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse

// Config holds server configuration.
type Config struct {
	Port        int
	HealthzPath string
	ReadyzPath  string
}

// Server is the webhook HTTP server.
type Server struct {
	server       *http.Server
	certProvider *certprovider.Provider
	mux          *http.ServeMux
	config       Config
}

// New creates a new webhook server.
func New(certProvider *certprovider.Provider, config Config) *Server {
	mux := http.NewServeMux()

	s := &Server{
		certProvider: certProvider,
		mux:          mux,
		config:       config,
	}

	// Register health endpoints
	mux.HandleFunc(config.HealthzPath, s.healthzHandler)
	mux.HandleFunc(config.ReadyzPath, s.readyzHandler)

	return s
}

// RegisterHook registers a webhook handler at the given path.
func (s *Server) RegisterHook(path string, hookType HookType, admit AdmitFunc) {
	s.mux.Handle(path, newAdmissionHandler(admit))
	klog.V(2).Infof("Registered %s webhook at %s", hookType, path)
}

// Start starts the HTTPS server.
func (s *Server) Start(ctx context.Context) error {
	tlsConfig := &tls.Config{
		GetCertificate: s.certProvider.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.config.Port),
		Handler:           s.mux,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errChan := make(chan error, 1)
	go func() {
		klog.Infof("Starting webhook server on port %d", s.config.Port)
		if err := s.server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		klog.Info("Shutting down webhook server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(shutdownCtx)
	case err := <-errChan:
		klog.Errorf("Webhook server error: %v", err)
		return err
	}
}

// healthzHandler handles health check requests.
func (s *Server) healthzHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := io.WriteString(w, "ok"); err != nil {
		klog.Errorf("Failed to write healthz response: %v", err)
	}
}

// readyzHandler handles readiness check requests.
func (s *Server) readyzHandler(w http.ResponseWriter, r *http.Request) {
	if !s.certProvider.Ready() {
		klog.Error("Certificate not ready")
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := io.WriteString(w, "certificate not ready"); err != nil {
			klog.Errorf("Failed to write readyz response: %v", err)
		}
		return
	}
	if _, err := io.WriteString(w, "ok"); err != nil {
		klog.Errorf("Failed to write readyz response: %v", err)
	}
}
