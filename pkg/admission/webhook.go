// Package admission provides the main entry point for running admission webhooks
// with automatic certificate rotation.
package admission

import (
	"github.com/jimyag/auto-cert-webhook/pkg/webhook"
)

// Re-export types from webhook package for convenience
type (
	// Webhook is the base interface that all webhooks must implement.
	Webhook = webhook.Webhook

	// ValidatingWebhook is the interface for validating admission webhooks.
	ValidatingWebhook = webhook.ValidatingWebhook

	// MutatingWebhook is the interface for mutating admission webhooks.
	MutatingWebhook = webhook.MutatingWebhook

	// WebhookConfig contains the configuration for a webhook.
	WebhookConfig = webhook.Config
)

// Re-export functions from webhook package
var (
	// Allowed returns an admission response that allows the request.
	Allowed = webhook.Allowed

	// AllowedWithMessage returns an admission response that allows the request with a message.
	AllowedWithMessage = webhook.AllowedWithMessage

	// Denied returns an admission response that denies the request.
	Denied = webhook.Denied

	// DeniedWithReason returns an admission response that denies the request with a specific reason.
	DeniedWithReason = webhook.DeniedWithReason

	// Errored returns an admission response for an error.
	Errored = webhook.Errored

	// ErroredWithCode returns an admission response for an error with a specific code.
	ErroredWithCode = webhook.ErroredWithCode

	// PatchResponse creates a patch response from the original and modified objects.
	PatchResponse = webhook.PatchResponse

	// PatchResponseFromRaw creates a patch response from raw JSON bytes.
	PatchResponseFromRaw = webhook.PatchResponseFromRaw

	// PatchResponseFromPatches creates a patch response from pre-built patches.
	PatchResponseFromPatches = webhook.PatchResponseFromPatches

	// DefaultWebhookConfig returns a WebhookConfig with default values applied.
	DefaultWebhookConfig = webhook.DefaultConfig
)
