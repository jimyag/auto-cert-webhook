package cabundle

import (
	"testing"
)

func TestWebhookRef(t *testing.T) {
	ref := WebhookRef{
		Name: "test-webhook",
		Type: ValidatingWebhook,
	}

	if ref.Name != "test-webhook" {
		t.Errorf("Name: got %q, want %q", ref.Name, "test-webhook")
	}

	if ref.Type != ValidatingWebhook {
		t.Errorf("Type: got %v, want %v", ref.Type, ValidatingWebhook)
	}
}

func TestWebhookType_Constants(t *testing.T) {
	if ValidatingWebhook != "validating" {
		t.Errorf("ValidatingWebhook: got %q, want %q", ValidatingWebhook, "validating")
	}

	if MutatingWebhook != "mutating" {
		t.Errorf("MutatingWebhook: got %q, want %q", MutatingWebhook, "mutating")
	}
}

func TestNewSyncer(t *testing.T) {
	refs := []WebhookRef{
		{Name: "webhook1", Type: ValidatingWebhook},
		{Name: "webhook2", Type: MutatingWebhook},
	}

	syncer := NewSyncer(nil, "test-ns", "ca-bundle-cm", refs)

	if syncer.namespace != "test-ns" {
		t.Errorf("namespace: got %q, want %q", syncer.namespace, "test-ns")
	}

	if syncer.caBundleConfigMapName != "ca-bundle-cm" {
		t.Errorf("caBundleConfigMapName: got %q, want %q", syncer.caBundleConfigMapName, "ca-bundle-cm")
	}

	if len(syncer.webhookRefs) != 2 {
		t.Errorf("webhookRefs: got %d, want 2", len(syncer.webhookRefs))
	}
}
