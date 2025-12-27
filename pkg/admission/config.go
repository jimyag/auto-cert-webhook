package admission

import (
	"os"
	"time"
)

// Config holds the server configuration.
type Config struct {
	// Namespace is the namespace where the webhook is deployed.
	Namespace string

	// ServiceName is the name of the Kubernetes service for the webhook.
	ServiceName string

	// Port is the port the webhook server listens on.
	Port int

	// MetricsEnabled enables the metrics server.
	MetricsEnabled bool

	// MetricsPort is the port for the metrics server.
	MetricsPort int

	// HealthzPath is the path for health check endpoint.
	HealthzPath string

	// ReadyzPath is the path for readiness check endpoint.
	ReadyzPath string

	// MetricsPath is the path for metrics endpoint.
	MetricsPath string

	// CASecretName is the name of the secret containing the CA certificate.
	CASecretName string

	// CertSecretName is the name of the secret containing the server certificate.
	CertSecretName string

	// CABundleConfigMapName is the name of the configmap containing the CA bundle.
	CABundleConfigMapName string

	// CAValidity is the validity duration of the CA certificate.
	CAValidity time.Duration

	// CARefresh is the refresh interval for the CA certificate.
	CARefresh time.Duration

	// CertValidity is the validity duration of the server certificate.
	CertValidity time.Duration

	// CertRefresh is the refresh interval for the server certificate.
	CertRefresh time.Duration

	// LeaderElection enables leader election for certificate rotation.
	LeaderElection bool

	// LeaderElectionID is the name of the lease resource for leader election.
	LeaderElectionID string

	// LeaseDuration is the duration of the leader election lease.
	LeaseDuration time.Duration

	// RenewDeadline is the deadline for renewing the leader election lease.
	RenewDeadline time.Duration

	// RetryPeriod is the period between leader election retries.
	RetryPeriod time.Duration
}

const (
	DefaultServiceName           = "auto-cert-webhook"
	DefaultNamespace             = "default"
	DefaultPort                  = 8443
	DefaultMetricsEnabled        = true
	DefaultMetricsPort           = 8080
	DefaultMetricsPath           = "/metrics"
	DefaultHealthzPath           = "/healthz"
	DefaultReadyzPath            = "/readyz"
	DefaultCASecretName          = "auto-cert-webhook-ca"
	DefaultCertSecretName        = "auto-cert-webhook-cert"
	DefaultCABundleConfigMapName = "auto-cert-webhook-ca-bundle"
	DefaultCAValidity            = 2 * 24 * time.Hour
	DefaultCARefresh             = 1 * 24 * time.Hour
	DefaultCertValidity          = 1 * 24 * time.Hour
	DefaultCertRefresh           = 0.5 * 24 * time.Hour
	DefaultLeaderElection        = true
	DefaultLeaderElectionID      = "auto-cert-webhook-cert-leader"
	DefaultLeaseDuration         = 30 * time.Second
	DefaultRenewDeadline         = 10 * time.Second
	DefaultRetryPeriod           = 5 * time.Second
)

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = DefaultNamespace
	}

	return &Config{
		Namespace:             namespace,
		ServiceName:           DefaultServiceName,
		Port:                  DefaultPort,
		MetricsEnabled:        DefaultMetricsEnabled,
		MetricsPort:           DefaultMetricsPort,
		HealthzPath:           DefaultHealthzPath,
		ReadyzPath:            DefaultReadyzPath,
		MetricsPath:           DefaultMetricsPath,
		CASecretName:          DefaultCASecretName,
		CertSecretName:        DefaultCertSecretName,
		CABundleConfigMapName: DefaultCABundleConfigMapName,
		CAValidity:            DefaultCAValidity,
		CARefresh:             DefaultCARefresh,
		CertValidity:          DefaultCertValidity,
		CertRefresh:           DefaultCertRefresh,
	}
}

// ApplyWebhookConfig applies webhook-specific configuration.
func (c *Config) ApplyWebhookConfig(wc WebhookConfig) {
	if wc.Name != "" {
		c.CASecretName = wc.Name + "-ca"
		c.CertSecretName = wc.Name + "-cert"
		c.CABundleConfigMapName = wc.Name + "-ca-bundle"
		c.LeaderElectionID = wc.Name + "-cert-leader"
		if c.ServiceName == DefaultServiceName {
			c.ServiceName = wc.Name
		}
	}
}
