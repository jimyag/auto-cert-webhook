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
type Config struct {
	// Name is the webhook name, used for generating certificate resources.
	// This will be used as prefix for Secret, ConfigMap, and Lease names.
	// Required.
	Name string

	// Namespace is the namespace where the webhook is deployed.
	// If empty, auto-detected from ServiceAccount or defaults to "default".
	Namespace string

	// ServiceName is the name of the Kubernetes service for the webhook.
	// If empty, defaults to Name.
	ServiceName string

	// Port is the port the webhook server listens on.
	// If 0, defaults to 8443.
	Port int

	// MetricsEnabled enables the metrics server.
	// Defaults to true.
	MetricsEnabled *bool

	// MetricsPort is the port for the metrics server.
	// If 0, defaults to 8080.
	MetricsPort int

	// MetricsPath is the path for metrics endpoint.
	// If empty, defaults to "/metrics".
	MetricsPath string

	// HealthzPath is the path for health check endpoint.
	// If empty, defaults to "/healthz".
	HealthzPath string

	// ReadyzPath is the path for readiness check endpoint.
	// If empty, defaults to "/readyz".
	ReadyzPath string

	// CASecretName is the name of the secret containing the CA certificate.
	// If empty, defaults to "<Name>-ca".
	CASecretName string

	// CertSecretName is the name of the secret containing the server certificate.
	// If empty, defaults to "<Name>-cert".
	CertSecretName string

	// CABundleConfigMapName is the name of the configmap containing the CA bundle.
	// If empty, defaults to "<Name>-ca-bundle".
	CABundleConfigMapName string

	// CAValidity is the validity duration of the CA certificate.
	// If 0, defaults to 2 days.
	CAValidity time.Duration

	// CARefresh is the refresh interval for the CA certificate.
	// If 0, defaults to 1 day.
	CARefresh time.Duration

	// CertValidity is the validity duration of the server certificate.
	// If 0, defaults to 1 day.
	CertValidity time.Duration

	// CertRefresh is the refresh interval for the server certificate.
	// If 0, defaults to 12 hours.
	CertRefresh time.Duration

	// LeaderElection enables leader election for certificate rotation.
	// Defaults to true.
	LeaderElection *bool

	// LeaderElectionID is the name of the lease resource for leader election.
	// If empty, defaults to "<Name>-leader".
	LeaderElectionID string

	// LeaseDuration is the duration of the leader election lease.
	// If 0, defaults to 30 seconds.
	LeaseDuration time.Duration

	// RenewDeadline is the deadline for renewing the leader election lease.
	// If 0, defaults to 10 seconds.
	RenewDeadline time.Duration

	// RetryPeriod is the period between leader election retries.
	// If 0, defaults to 5 seconds.
	RetryPeriod time.Duration
}

// Admission is the main interface that users need to implement.
type Admission interface {
	// Configure returns the server-level configuration.
	Configure() Config

	// Webhooks returns all webhook definitions.
	Webhooks() []Hook
}
