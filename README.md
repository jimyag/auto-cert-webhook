# auto-cert-webhook

A lightweight framework for building Kubernetes admission webhooks with automatic TLS certificate management.

## Features

- Self-signed CA and serving certificate generation
- Automatic certificate rotation using [openshift/library-go](https://github.com/openshift/library-go)
- Hot-reload certificates via Secret informer (no file watching)
- Automatic `caBundle` synchronization to WebhookConfiguration
- Leader election for multi-replica deployments
- Support multiple webhooks in a single server
- Prometheus metrics for certificate monitoring

## Installation

```bash
go get github.com/jimyag/auto-cert-webhook
```

## Quick Start

```go
package main

import (
    "encoding/json"

    admissionv1 "k8s.io/api/admission/v1"
    corev1 "k8s.io/api/core/v1"

    webhook "github.com/jimyag/auto-cert-webhook"
)

func main() {
    webhook.Run(&myWebhook{})
}

type myWebhook struct{}

func (m *myWebhook) Configure() webhook.Config {
    return webhook.Config{
        Name: "my-webhook",
    }
}

func (m *myWebhook) Webhooks() []webhook.Hook {
    return []webhook.Hook{
        {
            Path:  "/mutate-pods",
            Type:  webhook.Mutating,
            Admit: m.mutatePod,
        },
        {
            Path:  "/validate-pods",
            Type:  webhook.Validating,
            Admit: m.validatePod,
        },
    }
}

func (m *myWebhook) mutatePod(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
    pod := &corev1.Pod{}
    json.Unmarshal(ar.Request.Object.Raw, pod)

    modified := pod.DeepCopy()
    if modified.Labels == nil {
        modified.Labels = make(map[string]string)
    }
    modified.Labels["mutated"] = "true"

    return webhook.PatchResponse(pod, modified)
}

func (m *myWebhook) validatePod(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
    // validation logic
    return webhook.Allowed()
}
```

## Configuration

All configuration is done through the `Config` struct returned by `Configure()`:

```go
func (m *myWebhook) Configure() webhook.Config {
    return webhook.Config{
        // Required
        Name: "my-webhook",

        // Optional - all have sensible defaults
        Namespace:             "webhook-system",     // default: auto-detected
        ServiceName:           "my-webhook-svc",     // default: Name
        Port:                  8443,                 // default: 8443
        MetricsEnabled:        ptr(true),            // default: true
        MetricsPort:           8080,                 // default: 8080
        MetricsPath:           "/metrics",           // default: /metrics
        HealthzPath:           "/healthz",           // default: /healthz
        ReadyzPath:            "/readyz",            // default: /readyz
        CASecretName:          "my-webhook-ca",      // default: <Name>-ca
        CertSecretName:        "my-webhook-cert",    // default: <Name>-cert
        CABundleConfigMapName: "my-webhook-bundle",  // default: <Name>-ca-bundle
        CAValidity:            365 * 24 * time.Hour, // default: 2 days
        CARefresh:             30 * 24 * time.Hour,  // default: 1 day
        CertValidity:          30 * 24 * time.Hour,  // default: 1 day
        CertRefresh:           12 * time.Hour,       // default: 12 hours
        LeaderElection:        ptr(true),            // default: true
        LeaderElectionID:      "my-webhook-leader",  // default: <Name>-leader
        LeaseDuration:         30 * time.Second,     // default: 30s
        RenewDeadline:         10 * time.Second,     // default: 10s
        RetryPeriod:           5 * time.Second,      // default: 5s
    }
}

func ptr[T any](v T) *T { return &v }
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Webhook Pod                              │
├─────────────────────────────────────────────────────────────┤
│  Leader Only:                                                │
│    - CertManager (certificate rotation)                      │
│    - CABundleSyncer (patch WebhookConfiguration)            │
│                                                              │
│  All Pods:                                                   │
│    - CertProvider (watch Secret, hot-reload)                │
│    - TLS Server (serve admission requests)                  │
│    - Metrics Server (Prometheus metrics)                    │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

The framework creates Secrets and ConfigMaps automatically. You need to create the WebhookConfiguration manually or via Helm/Kustomize:

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: my-webhook
webhooks:
- name: my-webhook.default.svc
  clientConfig:
    service:
      name: my-webhook
      namespace: default
      path: /mutate-pods
      port: 8443
    caBundle: ""  # auto-populated by the framework
  rules:
  - operations: ["CREATE"]
    apiGroups: [""]
    apiVersions: ["v1"]
    resources: ["pods"]
  sideEffects: None
  admissionReviewVersions: ["v1"]
```

## Required RBAC

```yaml
rules:
- apiGroups: [""]
  resources: ["secrets", "configmaps"]
  verbs: ["get", "list", "watch", "create", "update"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "create", "update"]
- apiGroups: ["admissionregistration.k8s.io"]
  resources: ["mutatingwebhookconfigurations", "validatingwebhookconfigurations"]
  verbs: ["get", "update", "patch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `POD_NAMESPACE` | Namespace for certificate secrets | Auto-detected from ServiceAccount |
| `POD_NAME` | Pod identity for leader election | hostname |

The namespace is automatically detected from `/var/run/secrets/kubernetes.io/serviceaccount/namespace` (mounted by Kubernetes). You only need to set `POD_NAMESPACE` if running outside a Kubernetes cluster or without a ServiceAccount.

## Metrics

The framework exposes Prometheus metrics on a separate HTTP port (default: 8080).

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `admission_webhook_certificate_expiry_timestamp_seconds` | Gauge | `type` | Certificate expiry timestamp (unix seconds) |
| `admission_webhook_certificate_not_before_timestamp_seconds` | Gauge | `type` | Certificate not-before timestamp (unix seconds) |
| `admission_webhook_certificate_valid_duration_seconds` | Gauge | `type` | Total certificate validity duration (seconds) |

Example Prometheus alert:

```yaml
groups:
- name: webhook-certificates
  rules:
  - alert: WebhookCertificateExpiringSoon
    expr: admission_webhook_certificate_expiry_timestamp_seconds{type="serving"} - time() < 86400 * 7
    for: 1h
    labels:
      severity: warning
    annotations:
      summary: "Webhook certificate expiring in less than 7 days"
```

## License

Apache-2.0
