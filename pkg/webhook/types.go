package webhook

import (
	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Webhook is the base interface that all webhooks must implement.
type Webhook interface {
	// Configure returns the webhook configuration.
	Configure() Config
}

// ValidatingWebhook is the interface for validating admission webhooks.
type ValidatingWebhook interface {
	Webhook
	// Validate handles the admission request and returns a response.
	Validate(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse
}

// MutatingWebhook is the interface for mutating admission webhooks.
type MutatingWebhook interface {
	Webhook
	// Mutate handles the admission request and returns a response with optional patches.
	Mutate(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse
}

// Config contains the configuration for a webhook.
type Config struct {
	// Name is the name of the webhook, used for generating resource names.
	// Required.
	Name string

	// ValidatePath is the path for validating webhook endpoint.
	// Defaults to "/validate".
	ValidatePath string

	// MutatePath is the path for mutating webhook endpoint.
	// Defaults to "/mutate".
	MutatePath string

	// Rules defines the resources and operations this webhook handles.
	// Required.
	Rules []admissionregistrationv1.RuleWithOperations

	// NamespaceSelector restricts which namespaces the webhook applies to.
	// Optional.
	NamespaceSelector *metav1.LabelSelector

	// ObjectSelector restricts which objects the webhook applies to.
	// Optional.
	ObjectSelector *metav1.LabelSelector

	// FailurePolicy specifies what to do when the webhook is unavailable.
	// Defaults to Fail.
	FailurePolicy *admissionregistrationv1.FailurePolicyType

	// SideEffects specifies whether the webhook has side effects.
	// Defaults to None.
	SideEffects *admissionregistrationv1.SideEffectClass

	// MatchPolicy specifies how the rules should be matched.
	// Defaults to Equivalent.
	MatchPolicy *admissionregistrationv1.MatchPolicyType

	// TimeoutSeconds specifies the timeout for the webhook call.
	// Defaults to 10.
	TimeoutSeconds *int32

	// ReinvocationPolicy specifies when the mutating webhook should be reinvoked.
	// Only applies to MutatingWebhook. Defaults to Never.
	ReinvocationPolicy *admissionregistrationv1.ReinvocationPolicyType
}

// DefaultConfig returns a Config with default values applied.
func DefaultConfig(name string) Config {
	failurePolicy := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	matchPolicy := admissionregistrationv1.Equivalent
	timeoutSeconds := int32(10)
	reinvocationPolicy := admissionregistrationv1.NeverReinvocationPolicy

	return Config{
		Name:               name,
		ValidatePath:       "/validate",
		MutatePath:         "/mutate",
		FailurePolicy:      &failurePolicy,
		SideEffects:        &sideEffects,
		MatchPolicy:        &matchPolicy,
		TimeoutSeconds:     &timeoutSeconds,
		ReinvocationPolicy: &reinvocationPolicy,
	}
}
