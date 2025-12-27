package webhook

import (
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

// Config contains the server-level configuration.
type Config struct {
	// Name is the webhook name, used for generating certificate resources.
	// This will be used as prefix for Secret, ConfigMap, and Lease names.
	Name string
}

// Admission is the main interface that users need to implement.
type Admission interface {
	// Configure returns the server-level configuration.
	Configure() Config

	// Webhooks returns all webhook definitions.
	Webhooks() []Hook
}
