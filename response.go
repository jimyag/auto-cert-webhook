package autocertwebhook

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/appscode/jsonpatch"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Allowed returns an admission response that allows the request.
func Allowed() *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: true,
	}
}

// AllowedWithMessage returns an admission response that allows the request with a message.
func AllowedWithMessage(message string) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: true,
		Result: &metav1.Status{
			Message: message,
		},
	}
}

// Denied returns an admission response that denies the request.
func Denied(message string) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Status:  metav1.StatusFailure,
			Message: message,
			Reason:  metav1.StatusReasonForbidden,
			Code:    http.StatusForbidden,
		},
	}
}

// DeniedWithReason returns an admission response that denies the request with a specific reason.
func DeniedWithReason(message string, reason metav1.StatusReason, code int32) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Status:  metav1.StatusFailure,
			Message: message,
			Reason:  reason,
			Code:    code,
		},
	}
}

// Errored returns an admission response for an error.
func Errored(err error) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Status:  metav1.StatusFailure,
			Message: err.Error(),
			Reason:  metav1.StatusReasonInternalError,
			Code:    http.StatusInternalServerError,
		},
	}
}

// ErroredWithCode returns an admission response for an error with a specific code.
func ErroredWithCode(err error, code int32) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Status:  metav1.StatusFailure,
			Message: err.Error(),
			Code:    code,
		},
	}
}

// PatchResponse creates a patch response from the original and modified objects.
func PatchResponse(original, modified interface{}) *admissionv1.AdmissionResponse {
	originalBytes, err := json.Marshal(original)
	if err != nil {
		return Errored(fmt.Errorf("failed to marshal original object: %w", err))
	}

	modifiedBytes, err := json.Marshal(modified)
	if err != nil {
		return Errored(fmt.Errorf("failed to marshal modified object: %w", err))
	}

	return PatchResponseFromRaw(originalBytes, modifiedBytes)
}

// PatchResponseFromRaw creates a patch response from raw JSON bytes.
func PatchResponseFromRaw(original, modified []byte) *admissionv1.AdmissionResponse {
	patches, err := jsonpatch.CreatePatch(original, modified)
	if err != nil {
		return Errored(fmt.Errorf("failed to create patch: %w", err))
	}

	if len(patches) == 0 {
		return Allowed()
	}

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return Errored(fmt.Errorf("failed to marshal patch: %w", err))
	}

	patchType := admissionv1.PatchTypeJSONPatch
	return &admissionv1.AdmissionResponse{
		Allowed:   true,
		Patch:     patchBytes,
		PatchType: &patchType,
	}
}

// PatchResponseFromPatches creates a patch response from pre-built patches.
func PatchResponseFromPatches(patches []jsonpatch.JsonPatchOperation) *admissionv1.AdmissionResponse {
	if len(patches) == 0 {
		return Allowed()
	}

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return Errored(fmt.Errorf("failed to marshal patch: %w", err))
	}

	patchType := admissionv1.PatchTypeJSONPatch
	return &admissionv1.AdmissionResponse{
		Allowed:   true,
		Patch:     patchBytes,
		PatchType: &patchType,
	}
}
