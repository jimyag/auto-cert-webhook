package cabundle

import (
	"context"
	"encoding/json"
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// WebhookType represents the type of webhook.
type WebhookType string

const (
	// ValidatingWebhook represents a validating admission webhook.
	ValidatingWebhook WebhookType = "validating"
	// MutatingWebhook represents a mutating admission webhook.
	MutatingWebhook WebhookType = "mutating"
)

// WebhookRef references a webhook configuration to update.
type WebhookRef struct {
	// Name is the name of the webhook configuration.
	Name string
	// Type is the type of webhook (validating or mutating).
	Type WebhookType
}

// Syncer synchronizes CA bundle to webhook configurations.
type Syncer struct {
	client                kubernetes.Interface
	namespace             string
	caBundleConfigMapName string
	webhookRefs           []WebhookRef
}

// NewSyncer creates a new CA bundle syncer.
func NewSyncer(client kubernetes.Interface, namespace, caBundleConfigMapName string, webhookRefs []WebhookRef) *Syncer {
	return &Syncer{
		client:                client,
		namespace:             namespace,
		caBundleConfigMapName: caBundleConfigMapName,
		webhookRefs:           webhookRefs,
	}
}

// Start starts watching the CA bundle configmap and syncing to webhook configurations.
func (s *Syncer) Start(ctx context.Context) error {
	// Try to sync initially
	if err := s.syncCABundle(ctx); err != nil {
		klog.Warningf("Initial CA bundle sync failed (will retry via informer): %v", err)
	}

	// Set up informer to watch for changes
	factory := informers.NewSharedInformerFactoryWithOptions(
		s.client,
		0,
		informers.WithNamespace(s.namespace),
	)

	cmInformer := factory.Core().V1().ConfigMaps().Informer()

	_, err := cmInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			cm := obj.(*corev1.ConfigMap)
			if cm.Name == s.caBundleConfigMapName {
				s.onConfigMapUpdate(ctx, cm)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			cm := newObj.(*corev1.ConfigMap)
			if cm.Name == s.caBundleConfigMapName {
				s.onConfigMapUpdate(ctx, cm)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	factory.Start(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), cmInformer.HasSynced) {
		return fmt.Errorf("failed to sync informer cache")
	}

	klog.Infof("CA bundle syncer started watching configmap %s/%s", s.namespace, s.caBundleConfigMapName)

	<-ctx.Done()
	return nil
}

// syncCABundle syncs the CA bundle to all webhook configurations.
func (s *Syncer) syncCABundle(ctx context.Context) error {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, s.caBundleConfigMapName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("CA bundle configmap %s/%s not found yet", s.namespace, s.caBundleConfigMapName)
			return nil
		}
		return err
	}

	s.onConfigMapUpdate(ctx, cm)
	return nil
}

// onConfigMapUpdate handles configmap updates.
func (s *Syncer) onConfigMapUpdate(ctx context.Context, cm *corev1.ConfigMap) {
	caBundle, ok := cm.Data["ca-bundle.crt"]
	if !ok || len(caBundle) == 0 {
		klog.V(4).Infof("ConfigMap %s/%s has no ca-bundle.crt data yet", s.namespace, s.caBundleConfigMapName)
		return
	}

	for _, ref := range s.webhookRefs {
		if err := s.patchWebhook(ctx, ref, []byte(caBundle)); err != nil {
			klog.Errorf("Failed to patch webhook %s (%s): %v", ref.Name, ref.Type, err)
		} else {
			klog.Infof("Updated CA bundle for webhook %s (%s)", ref.Name, ref.Type)
		}
	}
}

// patchWebhook patches the caBundle field of a webhook configuration.
func (s *Syncer) patchWebhook(ctx context.Context, ref WebhookRef, caBundle []byte) error {
	switch ref.Type {
	case ValidatingWebhook:
		return s.patchValidatingWebhook(ctx, ref.Name, caBundle)
	case MutatingWebhook:
		return s.patchMutatingWebhook(ctx, ref.Name, caBundle)
	default:
		return fmt.Errorf("unknown webhook type: %s", ref.Type)
	}
}

// patchValidatingWebhook patches a ValidatingWebhookConfiguration.
func (s *Syncer) patchValidatingWebhook(ctx context.Context, name string, caBundle []byte) error {
	// Get current configuration to determine how many webhooks need patching
	current, err := s.client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("ValidatingWebhookConfiguration %s not found", name)
			return nil
		}
		return err
	}

	// Build patch for all webhooks
	var patches []map[string]interface{}
	for i := range current.Webhooks {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  fmt.Sprintf("/webhooks/%d/clientConfig/caBundle", i),
			"value": caBundle,
		})
	}

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = s.client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Patch(
		ctx, name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	return err
}

// patchMutatingWebhook patches a MutatingWebhookConfiguration.
func (s *Syncer) patchMutatingWebhook(ctx context.Context, name string, caBundle []byte) error {
	// Get current configuration to determine how many webhooks need patching
	current, err := s.client.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("MutatingWebhookConfiguration %s not found", name)
			return nil
		}
		return err
	}

	// Build patch for all webhooks
	var patches []map[string]interface{}
	for i := range current.Webhooks {
		patches = append(patches, map[string]interface{}{
			"op":    "replace",
			"path":  fmt.Sprintf("/webhooks/%d/clientConfig/caBundle", i),
			"value": caBundle,
		})
	}

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = s.client.AdmissionregistrationV1().MutatingWebhookConfigurations().Patch(
		ctx, name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	return err
}

// CreateValidatingWebhookConfiguration creates a ValidatingWebhookConfiguration.
func CreateValidatingWebhookConfiguration(name, namespace, serviceName, path string, port int32, caBundle []byte, rules []admissionregistrationv1.RuleWithOperations, failurePolicy *admissionregistrationv1.FailurePolicyType, sideEffects *admissionregistrationv1.SideEffectClass, matchPolicy *admissionregistrationv1.MatchPolicyType, namespaceSelector, objectSelector *metav1.LabelSelector, timeoutSeconds *int32) *admissionregistrationv1.ValidatingWebhookConfiguration {
	return &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name:                    fmt.Sprintf("%s.%s.svc", serviceName, namespace),
				Rules:                   rules,
				FailurePolicy:           failurePolicy,
				SideEffects:             sideEffects,
				MatchPolicy:             matchPolicy,
				NamespaceSelector:       namespaceSelector,
				ObjectSelector:          objectSelector,
				TimeoutSeconds:          timeoutSeconds,
				AdmissionReviewVersions: []string{"v1", "v1beta1"},
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Name:      serviceName,
						Namespace: namespace,
						Path:      &path,
						Port:      &port,
					},
					CABundle: caBundle,
				},
			},
		},
	}
}

// CreateMutatingWebhookConfiguration creates a MutatingWebhookConfiguration.
func CreateMutatingWebhookConfiguration(name, namespace, serviceName, path string, port int32, caBundle []byte, rules []admissionregistrationv1.RuleWithOperations, failurePolicy *admissionregistrationv1.FailurePolicyType, sideEffects *admissionregistrationv1.SideEffectClass, matchPolicy *admissionregistrationv1.MatchPolicyType, namespaceSelector, objectSelector *metav1.LabelSelector, timeoutSeconds *int32, reinvocationPolicy *admissionregistrationv1.ReinvocationPolicyType) *admissionregistrationv1.MutatingWebhookConfiguration {
	return &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name:                    fmt.Sprintf("%s.%s.svc", serviceName, namespace),
				Rules:                   rules,
				FailurePolicy:           failurePolicy,
				SideEffects:             sideEffects,
				MatchPolicy:             matchPolicy,
				NamespaceSelector:       namespaceSelector,
				ObjectSelector:          objectSelector,
				TimeoutSeconds:          timeoutSeconds,
				ReinvocationPolicy:      reinvocationPolicy,
				AdmissionReviewVersions: []string{"v1", "v1beta1"},
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Name:      serviceName,
						Namespace: namespace,
						Path:      &path,
						Port:      &port,
					},
					CABundle: caBundle,
				},
			},
		},
	}
}
