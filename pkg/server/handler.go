package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog/v2"

	"github.com/jimyag/auto-cert-webhook/pkg/webhook"
)

var (
	scheme = runtime.NewScheme()
	codecs = serializer.NewCodecFactory(scheme)
)

func init() {
	err := admissionv1.AddToScheme(scheme)
	if err != nil {
		klog.Fatalf("Failed to add admissionv1 scheme: %v", err)
	}
}

// validatingHandler handles validating admission requests.
type validatingHandler struct {
	webhook webhook.ValidatingWebhook
}

func newValidatingHandler(wh webhook.ValidatingWebhook) *validatingHandler {
	return &validatingHandler{webhook: wh}
}

func (h *validatingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handleAdmission(w, r, func(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
		return h.webhook.Validate(ar)
	})
}

// mutatingHandler handles mutating admission requests.
type mutatingHandler struct {
	webhook webhook.MutatingWebhook
}

func newMutatingHandler(wh webhook.MutatingWebhook) *mutatingHandler {
	return &mutatingHandler{webhook: wh}
}

func (h *mutatingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handleAdmission(w, r, func(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
		return h.webhook.Mutate(ar)
	})
}

// handleAdmission handles admission requests with the given handler function.
func handleAdmission(w http.ResponseWriter, r *http.Request, handle func(admissionv1.AdmissionReview) *admissionv1.AdmissionResponse) {
	klog.Infof("Handling admission request: %s %s", r.Method, r.URL.Path)
	var body []byte
	if r.Body != nil {
		defer r.Body.Close()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			klog.Errorf("Failed to read request body: %v", err)
			http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
			return
		}
		body = data
	}

	if len(body) == 0 {
		klog.Error("Empty request body")
		http.Error(w, "empty request body", http.StatusBadRequest)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		klog.Errorf("Unsupported content type: %s", contentType)
		http.Error(w, fmt.Sprintf("unsupported content type: %s", contentType), http.StatusUnsupportedMediaType)
		return
	}

	klog.Infof("Handling admission request: %s", string(body))

	// Decode the request
	requestedAdmissionReview := admissionv1.AdmissionReview{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &requestedAdmissionReview); err != nil {
		klog.Errorf("Failed to decode admission review: %v", err)
		http.Error(w, fmt.Sprintf("failed to decode admission review: %v", err), http.StatusBadRequest)
		return
	}

	// Prepare the response
	responseAdmissionReview := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: admissionv1.SchemeGroupVersion.String(),
			Kind:       "AdmissionReview",
		},
	}

	// Handle the request
	if requestedAdmissionReview.Request == nil {
		responseAdmissionReview.Response = &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: "AdmissionReview.Request is nil",
				Code:    http.StatusBadRequest,
			},
		}
	} else {
		responseAdmissionReview.Response = handle(requestedAdmissionReview)
	}

	// Set the UID
	if requestedAdmissionReview.Request != nil {
		responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID
	}

	// Match request's APIVersion for backwards compatibility
	if requestedAdmissionReview.APIVersion != "" {
		responseAdmissionReview.APIVersion = requestedAdmissionReview.APIVersion
	}

	klog.V(4).Infof("Sending admission response: %+v", responseAdmissionReview.Response)

	// Write the response
	respBytes, err := json.Marshal(responseAdmissionReview)
	if err != nil {
		klog.Errorf("Failed to marshal admission response: %v", err)
		http.Error(w, fmt.Sprintf("failed to marshal admission response: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBytes); err != nil {
		klog.Errorf("Failed to write admission response: %v", err)
	}
}
