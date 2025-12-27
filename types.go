// Package autocertwebhook provides a lightweight framework for building Kubernetes
// admission webhooks with automatic TLS certificate management.
package autocertwebhook

import (
	"time"

	admissionv1 "k8s.io/api/admission/v1"
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
type AdmitFunc func(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse

// Hook defines a single admission webhook endpoint.
type Hook struct {
	// Path is the URL path for this webhook, e.g., "/mutate-pods".
	Path string

	// Type is the webhook type: Mutating or Validating.
	Type HookType

	// Admit handles the admission request.
	Admit AdmitFunc
}

// Config contains all configuration for the webhook server.
// Configuration priority: code > environment variables > defaults.
// All environment variables use the "ACW_" prefix.
type Config struct {
	// Name is the webhook name, used for generating certificate resources.
	// This will be used as prefix for Secret, ConfigMap, and Lease names.
	// Required. Env: ACW_NAME
	Name string `envconfig:"NAME"`

	// Namespace is the namespace where the webhook is deployed.
	// If empty, auto-detected from ServiceAccount or defaults to "default".
	// Env: ACW_NAMESPACE
	Namespace string `envconfig:"NAMESPACE"`

	// ServiceName is the name of the Kubernetes service for the webhook.
	// If empty, defaults to Name.
	// Env: ACW_SERVICE_NAME
	ServiceName string `envconfig:"SERVICE_NAME"`

	// Port is the port the webhook server listens on.
	// Env: ACW_PORT
	Port int `envconfig:"PORT" default:"8443"`

	// MetricsEnabled enables the metrics server.
	// Env: ACW_METRICS_ENABLED
	MetricsEnabled *bool `envconfig:"METRICS_ENABLED" default:"true"`

	// MetricsPort is the port for the metrics server.
	// Env: ACW_METRICS_PORT
	MetricsPort int `envconfig:"METRICS_PORT" default:"8080"`

	// MetricsPath is the path for metrics endpoint.
	// Env: ACW_METRICS_PATH
	MetricsPath string `envconfig:"METRICS_PATH" default:"/metrics"`

	// HealthzPath is the path for health check endpoint.
	// Env: ACW_HEALTHZ_PATH
	HealthzPath string `envconfig:"HEALTHZ_PATH" default:"/healthz"`

	// ReadyzPath is the path for readiness check endpoint.
	// Env: ACW_READYZ_PATH
	ReadyzPath string `envconfig:"READYZ_PATH" default:"/readyz"`

	// CASecretName is the name of the secret containing the CA certificate.
	// If empty, defaults to "<Name>-ca".
	// Env: ACW_CA_SECRET_NAME
	CASecretName string `envconfig:"CA_SECRET_NAME"`

	// CertSecretName is the name of the secret containing the server certificate.
	// If empty, defaults to "<Name>-cert".
	// Env: ACW_CERT_SECRET_NAME
	CertSecretName string `envconfig:"CERT_SECRET_NAME"`

	// CABundleConfigMapName is the name of the configmap containing the CA bundle.
	// If empty, defaults to "<Name>-ca-bundle".
	// Env: ACW_CA_BUNDLE_CONFIGMAP_NAME
	CABundleConfigMapName string `envconfig:"CA_BUNDLE_CONFIGMAP_NAME"`

	// CAValidity is the validity duration of the CA certificate.
	// Env: ACW_CA_VALIDITY (e.g., "48h")
	CAValidity time.Duration `envconfig:"CA_VALIDITY" default:"48h"`

	// CARefresh is the refresh interval for the CA certificate.
	// Env: ACW_CA_REFRESH (e.g., "24h")
	CARefresh time.Duration `envconfig:"CA_REFRESH" default:"24h"`

	// CertValidity is the validity duration of the server certificate.
	// Env: ACW_CERT_VALIDITY (e.g., "24h")
	CertValidity time.Duration `envconfig:"CERT_VALIDITY" default:"24h"`

	// CertRefresh is the refresh interval for the server certificate.
	// Env: ACW_CERT_REFRESH (e.g., "12h")
	CertRefresh time.Duration `envconfig:"CERT_REFRESH" default:"12h"`

	// LeaderElection enables leader election for certificate rotation.
	// Env: ACW_LEADER_ELECTION
	LeaderElection *bool `envconfig:"LEADER_ELECTION"`

	// LeaderElectionID is the name of the lease resource for leader election.
	// If empty, defaults to "<Name>-leader".
	// Env: ACW_LEADER_ELECTION_ID
	LeaderElectionID string `envconfig:"LEADER_ELECTION_ID"`

	// LeaseDuration is the duration of the leader election lease.
	// Env: ACW_LEASE_DURATION (e.g., "30s")
	LeaseDuration time.Duration `envconfig:"LEASE_DURATION" default:"30s"`

	// RenewDeadline is the deadline for renewing the leader election lease.
	// Env: ACW_RENEW_DEADLINE (e.g., "10s")
	RenewDeadline time.Duration `envconfig:"RENEW_DEADLINE" default:"10s"`

	// RetryPeriod is the period between leader election retries.
	// Env: ACW_RETRY_PERIOD (e.g., "5s")
	RetryPeriod time.Duration `envconfig:"RETRY_PERIOD" default:"5s"`
}

// Admission is the main interface that users need to implement.
type Admission interface {
	// Configure returns the server-level configuration.
	Configure() Config

	// Webhooks returns all webhook definitions.
	Webhooks() []Hook
}
