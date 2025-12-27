package metrics

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestUpdateCertMetrics(t *testing.T) {
	// Reset metrics for testing
	certExpiryTimestamp.Reset()
	certNotBeforeTimestamp.Reset()
	certValidDurationSeconds.Reset()

	// Create a test certificate
	notBefore := time.Now()
	notAfter := notBefore.Add(24 * time.Hour)
	cert := createTestCert(t, notBefore, notAfter)

	t.Run("updates all metrics", func(t *testing.T) {
		UpdateCertMetrics("serving", cert)

		// Check expiry timestamp
		expiry := getGaugeValue(t, certExpiryTimestamp, "serving")
		if expiry != float64(notAfter.Unix()) {
			t.Errorf("certExpiryTimestamp: got %v, want %v", expiry, float64(notAfter.Unix()))
		}

		// Check not-before timestamp
		notBeforeVal := getGaugeValue(t, certNotBeforeTimestamp, "serving")
		if notBeforeVal != float64(notBefore.Unix()) {
			t.Errorf("certNotBeforeTimestamp: got %v, want %v", notBeforeVal, float64(notBefore.Unix()))
		}

		// Check valid duration
		duration := getGaugeValue(t, certValidDurationSeconds, "serving")
		expectedDuration := notAfter.Sub(notBefore).Seconds()
		if duration != expectedDuration {
			t.Errorf("certValidDurationSeconds: got %v, want %v", duration, expectedDuration)
		}
	})

	t.Run("handles different cert types", func(t *testing.T) {
		certExpiryTimestamp.Reset()

		UpdateCertMetrics("ca", cert)
		UpdateCertMetrics("serving", cert)

		// Both should have metrics
		caExpiry := getGaugeValue(t, certExpiryTimestamp, "ca")
		servingExpiry := getGaugeValue(t, certExpiryTimestamp, "serving")

		if caExpiry != float64(notAfter.Unix()) {
			t.Errorf("CA expiry: got %v, want %v", caExpiry, float64(notAfter.Unix()))
		}
		if servingExpiry != float64(notAfter.Unix()) {
			t.Errorf("Serving expiry: got %v, want %v", servingExpiry, float64(notAfter.Unix()))
		}
	})

	t.Run("nil certificate is handled", func(t *testing.T) {
		// Should not panic
		UpdateCertMetrics("test", nil)
	})
}

func TestRegister(t *testing.T) {
	// Register should be idempotent (can be called multiple times)
	Register()
	Register()
	Register()
	// If it panics, the test fails
}

func TestHandler(t *testing.T) {
	handler := Handler()
	if handler == nil {
		t.Error("Handler() returned nil")
	}
}

// Helper to get gauge value
func getGaugeValue(t *testing.T, gauge *prometheus.GaugeVec, label string) float64 {
	t.Helper()

	metric, err := gauge.GetMetricWithLabelValues(label)
	if err != nil {
		t.Fatalf("Failed to get metric: %v", err)
	}

	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	return m.GetGauge().GetValue()
}

// Helper to create a test certificate
func createTestCert(t *testing.T, notBefore, notAfter time.Time) *x509.Certificate {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test",
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	return cert
}
