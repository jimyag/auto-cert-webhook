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

// admissionHandler handles admission requests.
type admissionHandler struct {
	admit AdmitFunc
}

func newAdmissionHandler(admit AdmitFunc) *admissionHandler {
	return &admissionHandler{admit: admit}
}

func (h *admissionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	klog.V(2).Infof("Handling admission request: %s %s", r.Method, r.URL.Path)

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

	klog.V(4).Infof("Request body: %s", string(body))

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
		responseAdmissionReview.Response = h.admit(requestedAdmissionReview)
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
