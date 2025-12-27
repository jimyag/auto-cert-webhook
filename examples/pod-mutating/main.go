package main

import (
	"encoding/json"
	"flag"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	webhook "github.com/jimyag/auto-cert-webhook"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	if err := webhook.Run(&podLabelInjector{}); err != nil {
		klog.Fatalf("Failed to run webhook: %v", err)
	}
}

// podLabelInjector is a mutating webhook that injects labels into pods.
type podLabelInjector struct{}

// Configure returns the server-level configuration.
func (p *podLabelInjector) Configure() webhook.Config {
	return webhook.Config{
		Name: "pod-label-injector",
	}
}

// Webhooks returns all webhook definitions.
func (p *podLabelInjector) Webhooks() []webhook.Hook {
	return []webhook.Hook{
		{
			Path:  "/mutate-pods",
			Type:  webhook.Mutating,
			Admit: p.mutatePod,
		},
	}
}

// mutatePod handles the mutating admission request.
func (p *podLabelInjector) mutatePod(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	klog.V(2).Infof("Mutating pod %s/%s", ar.Request.Namespace, ar.Request.Name)

	// Parse the pod
	pod := &corev1.Pod{}
	if err := json.Unmarshal(ar.Request.Object.Raw, pod); err != nil {
		klog.Errorf("Failed to unmarshal pod: %v", err)
		return webhook.Errored(err)
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

	return webhook.PatchResponse(pod, modifiedPod)
}
