package main

import (
	"encoding/json"
	"flag"

	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/jimyag/auto-cert-webhook/pkg/admission"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	if err := admission.Run(&podLabelInjector{}); err != nil {
		klog.Fatalf("Failed to run webhook: %v", err)
	}
}

// podLabelInjector is a mutating webhook that injects labels into pods.
type podLabelInjector struct{}

// Configure returns the webhook configuration.
func (p *podLabelInjector) Configure() admission.WebhookConfig {
	return admission.WebhookConfig{
		Name:       "pod-label-injector",
		MutatePath: "/mutate",
		Rules: []admissionregistrationv1.RuleWithOperations{
			{
				Operations: []admissionregistrationv1.OperationType{
					admissionregistrationv1.Create,
				},
				Rule: admissionregistrationv1.Rule{
					APIGroups:   []string{""},
					APIVersions: []string{"v1"},
					Resources:   []string{"pods"},
				},
			},
		},
	}
}

// Mutate handles the mutating admission request.
func (p *podLabelInjector) Mutate(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	klog.V(2).Infof("Mutating pod %s/%s", ar.Request.Namespace, ar.Request.Name)

	// Parse the pod
	pod := &corev1.Pod{}
	if err := json.Unmarshal(ar.Request.Object.Raw, pod); err != nil {
		klog.Errorf("Failed to unmarshal pod: %v", err)
		return admission.Errored(err)
	}

	// Make a copy for modification
	modifiedPod := pod.DeepCopy()

	// Inject labels
	if modifiedPod.Labels == nil {
		modifiedPod.Labels = make(map[string]string)
	}
	modifiedPod.Labels["injected-by"] = "pod-label-injector"
	modifiedPod.Labels["injection-time"] = "admission"

	klog.V(2).Infof("Injected labels into pod %s/%s", ar.Request.Namespace, ar.Request.Name)

	return admission.PatchResponse(pod, modifiedPod)
}
