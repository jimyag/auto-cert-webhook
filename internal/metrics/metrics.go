package metrics

import (
	"crypto/x509"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace = "admission_webhook"
	subsystem = "certificate"
)

var (
	// CertExpiryTimestamp is a gauge that tracks the expiry timestamp of certificates.
	CertExpiryTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "expiry_timestamp_seconds",
			Help:      "The expiry timestamp of the certificate in seconds since epoch.",
		},
		[]string{"type"}, // "ca" or "serving"
	)

	// CertNotBeforeTimestamp is a gauge that tracks the not-before timestamp of certificates.
	CertNotBeforeTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "not_before_timestamp_seconds",
			Help:      "The not-before timestamp of the certificate in seconds since epoch.",
		},
		[]string{"type"},
	)

	// CertValidDurationSeconds is a gauge that tracks the total valid duration of certificates.
	CertValidDurationSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "valid_duration_seconds",
			Help:      "The total valid duration of the certificate in seconds.",
		},
		[]string{"type"},
	)

	registerOnce sync.Once
)

// Register registers all certificate metrics with the default registry.
func Register() {
	registerOnce.Do(func() {
		prometheus.MustRegister(CertExpiryTimestamp)
		prometheus.MustRegister(CertNotBeforeTimestamp)
		prometheus.MustRegister(CertValidDurationSeconds)
	})
}

// UpdateCertMetrics updates metrics for a certificate.
func UpdateCertMetrics(certType string, cert *x509.Certificate) {
	if cert == nil {
		return
	}

	CertExpiryTimestamp.WithLabelValues(certType).Set(float64(cert.NotAfter.Unix()))
	CertNotBeforeTimestamp.WithLabelValues(certType).Set(float64(cert.NotBefore.Unix()))
	CertValidDurationSeconds.WithLabelValues(certType).Set(cert.NotAfter.Sub(cert.NotBefore).Seconds())
}

// Handler returns an HTTP handler for the metrics endpoint.
func Handler() http.Handler {
	return promhttp.Handler()
}
