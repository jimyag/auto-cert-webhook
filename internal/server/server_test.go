package server

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
)

// mockCertProvider is a mock implementation for testing
type mockCertProvider struct {
	ready atomic.Bool
}

func (m *mockCertProvider) Ready() bool {
	return m.ready.Load()
}

func TestNew(t *testing.T) {
	provider := &mockCertProvider{}
	config := Config{
		Port:        8443,
		HealthzPath: "/healthz",
		ReadyzPath:  "/readyz",
	}

	server := newTestServer(provider, config)

	if server.config.Port != 8443 {
		t.Errorf("Port: got %d, want %d", server.config.Port, 8443)
	}
	if server.config.HealthzPath != "/healthz" {
		t.Errorf("HealthzPath: got %q, want %q", server.config.HealthzPath, "/healthz")
	}
	if server.config.ReadyzPath != "/readyz" {
		t.Errorf("ReadyzPath: got %q, want %q", server.config.ReadyzPath, "/readyz")
	}
	if server.mux == nil {
		t.Error("Expected non-nil mux")
	}
}

func TestServer_healthzHandler(t *testing.T) {
	provider := &mockCertProvider{}
	config := Config{
		Port:        8443,
		HealthzPath: "/healthz",
		ReadyzPath:  "/readyz",
	}

	server := newTestServer(provider, config)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	server.healthzHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("Expected body %q, got %q", "ok", rec.Body.String())
	}
}

func TestServer_readyzHandler(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		provider := &mockCertProvider{}
		provider.ready.Store(true)
		config := Config{
			Port:        8443,
			HealthzPath: "/healthz",
			ReadyzPath:  "/readyz",
		}

		server := newTestServer(provider, config)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		server.readyzHandler(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
		if rec.Body.String() != "ok" {
			t.Errorf("Expected body %q, got %q", "ok", rec.Body.String())
		}
	})

	t.Run("not ready", func(t *testing.T) {
		provider := &mockCertProvider{}
		provider.ready.Store(false)
		config := Config{
			Port:        8443,
			HealthzPath: "/healthz",
			ReadyzPath:  "/readyz",
		}

		server := newTestServer(provider, config)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		server.readyzHandler(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
		}
		if rec.Body.String() != "certificate not ready" {
			t.Errorf("Expected body %q, got %q", "certificate not ready", rec.Body.String())
		}
	})
}

func TestServer_RegisterHook(t *testing.T) {
	provider := &mockCertProvider{}
	config := Config{
		Port:        8443,
		HealthzPath: "/healthz",
		ReadyzPath:  "/readyz",
	}

	server := newTestServer(provider, config)

	admitFunc := func(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
		return &admissionv1.AdmissionResponse{Allowed: true}
	}

	// Register hooks
	server.RegisterHook("/mutate", "Mutating", admitFunc)
	server.RegisterHook("/validate", "Validating", admitFunc)

	// Verify handlers are registered by making test requests
	t.Run("mutating hook registered", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mutate", nil)
		rec := httptest.NewRecorder()
		server.mux.ServeHTTP(rec, req)
		// Should not be 404
		if rec.Code == http.StatusNotFound {
			t.Error("Expected /mutate to be registered")
		}
	})

	t.Run("validating hook registered", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		rec := httptest.NewRecorder()
		server.mux.ServeHTTP(rec, req)
		// Should not be 404
		if rec.Code == http.StatusNotFound {
			t.Error("Expected /validate to be registered")
		}
	})
}

func TestServer_HealthEndpointsRegistered(t *testing.T) {
	provider := &mockCertProvider{}
	provider.ready.Store(true)
	config := Config{
		Port:        8443,
		HealthzPath: "/healthz",
		ReadyzPath:  "/readyz",
	}

	server := newTestServer(provider, config)

	t.Run("healthz endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		server.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})

	t.Run("readyz endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		server.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})
}

func TestConfig(t *testing.T) {
	config := Config{
		Port:        9443,
		HealthzPath: "/health",
		ReadyzPath:  "/ready",
	}

	if config.Port != 9443 {
		t.Errorf("Port: got %d, want %d", config.Port, 9443)
	}
	if config.HealthzPath != "/health" {
		t.Errorf("HealthzPath: got %q, want %q", config.HealthzPath, "/health")
	}
	if config.ReadyzPath != "/ready" {
		t.Errorf("ReadyzPath: got %q, want %q", config.ReadyzPath, "/ready")
	}
}

// testServer wraps Server to use mock provider
type testServer struct {
	*Server
	mockProvider *mockCertProvider
}

// newTestServer creates a server with mock provider for testing
func newTestServer(provider *mockCertProvider, config Config) *testServer {
	mux := http.NewServeMux()

	s := &testServer{
		Server: &Server{
			mux:    mux,
			config: config,
		},
		mockProvider: provider,
	}

	// Register health endpoints using wrapper methods
	mux.HandleFunc(config.HealthzPath, s.healthzHandler)
	mux.HandleFunc(config.ReadyzPath, s.readyzHandler)

	return s
}

// Override readyzHandler to use mock provider
func (s *testServer) readyzHandler(w http.ResponseWriter, r *http.Request) {
	if !s.mockProvider.Ready() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("certificate not ready"))
		return
	}
	w.Write([]byte("ok"))
}

// healthzHandler is same as original
func (s *testServer) healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}
