# auto-cert-webhook

[![Go](https://github.com/jimyag/auto-cert-admission/actions/workflows/check.yaml/badge.svg)](https://github.com/jimyag/auto-cert-admission/actions/workflows/check.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/jimyag/auto-cert-webhook)](https://goreportcard.com/report/github.com/jimyag/auto-cert-webhook)
[![GoDoc](https://pkg.go.dev/badge/github.com/jimyag/auto-cert-webhook)](https://pkg.go.dev/github.com/jimyag/auto-cert-webhook)
[![codecov](https://codecov.io/gh/jimyag/auto-cert-admission/branch/main/graph/badge.svg)](https://codecov.io/gh/jimyag/auto-cert-admission)

A lightweight framework for building Kubernetes admission webhooks with automatic TLS certificate management.

## Features

- Self-signed CA and serving certificate generation
- Automatic certificate rotation using [openshift/library-go](https://github.com/openshift/library-go)
- Hot-reload certificates via Secret informer (no file watching)
- Automatic `caBundle` synchronization to WebhookConfiguration
- Leader election for multi-replica deployments
- Support multiple webhooks in a single server
- Prometheus metrics for certificate monitoring

## Requirements

- **Go**: 1.26.0+
- **Kubernetes**: 1.16+ (uses `admissionregistration.k8s.io/v1` API)

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

## Conventions

The framework uses the following naming and structure conventions:

### Resource Naming

| Resource | Default Name | Description |
|----------|--------------|-------------|
| CA Secret | `<Name>-ca` | Stores CA certificate and private key |
| Cert Secret | `<Name>-cert` | Stores server certificate and private key |
| CA Bundle ConfigMap | `<Name>-ca-bundle` | Stores CA bundle for webhook clients |
| Leader Election Lease | `<Name>-leader` | Lease resource for leader election |
| MutatingWebhookConfiguration | `<Name>` | Must match `Config.Name` |
| ValidatingWebhookConfiguration | `<Name>` | Must match `Config.Name` |

### Secret and ConfigMap Structure

CA Secret (`kubernetes.io/tls`):
- `tls.crt`: CA certificate (PEM)
- `tls.key`: CA private key (PEM)

Cert Secret (`kubernetes.io/tls`):
- `tls.crt`: Server certificate (PEM)
- `tls.key`: Server private key (PEM)

CA Bundle ConfigMap:
- `ca-bundle.crt`: CA certificate bundle (PEM)

### Environment Variables for Pod Identity

| Variable | Description |
|----------|-------------|
| `POD_NAME` | Used as leader election identity (falls back to hostname) |
| `POD_NAMESPACE` | Namespace detection (falls back to ServiceAccount namespace file) |

## Prerequisites

OpenShift and Kubernetes modules are upgraded together. Keep `k8s.io/kube-openapi`
at the revision required by Kubernetes: newer commits can use an incompatible
`structured-merge-diff` major version. Renovate updates the toolchain separately
while preserving the library's declared minimum Go version.

The framework creates Secrets and ConfigMaps automatically. You need to create the WebhookConfiguration manually or via Helm/Kustomize.

Important: The `MutatingWebhookConfiguration` and/or `ValidatingWebhookConfiguration` must have the same name as `Config.Name`. The framework uses this name to find and patch the `caBundle` field automatically.

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: my-webhook  # Must match Config.Name
webhooks:
- name: my-webhook.default.svc
  clientConfig:
    service:
      name: my-webhook
      namespace: default
      path: /mutate-pods
      port: 443
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

The ServiceAccount running the webhook needs the following permissions:

```yaml
rules:
# Certificate management: the framework reads, creates, and rotates CA/serving
# certificate Secrets, and writes the CA bundle to a ConfigMap for client use.
- apiGroups: [""]
  resources: ["secrets", "configmaps"]
  verbs: ["get", "list", "watch", "create", "update"]
# Leader election: Lease objects are used to elect one replica as the active
# certificate manager. list/watch are required by the framework's
# leader election informer to monitor lease state and emit metrics.
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update"]
# caBundle sync: the leader patches the caBundle field in WebhookConfiguration
# objects so the API server can validate the webhook's TLS certificate.
- apiGroups: ["admissionregistration.k8s.io"]
  resources: ["mutatingwebhookconfigurations", "validatingwebhookconfigurations"]
  verbs: ["get", "update", "patch"]
# Events: leader election and certificate rotation emit Kubernetes events for
# observability.
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

## Environment Variables

All configuration options can be set via environment variables with the `ACW_` prefix. Configuration priority: **code > environment variables > defaults**.

| Variable | Description | Default |
|----------|-------------|---------|
| `ACW_NAME` | Webhook name (required if not set in code) | - |
| `ACW_NAMESPACE` | Namespace for webhook resources | Auto-detected |
| `ACW_SERVICE_NAME` | Kubernetes service name | `<Name>` |
| `ACW_PORT` | Webhook server port | `8443` |
| `ACW_METRICS_ENABLED` | Enable metrics server | `true` |
| `ACW_METRICS_PORT` | Metrics server port | `8080` |
| `ACW_METRICS_PATH` | Metrics endpoint path | `/metrics` |
| `ACW_HEALTHZ_PATH` | Health check endpoint path | `/healthz` |
| `ACW_READYZ_PATH` | Readiness endpoint path | `/readyz` |
| `ACW_CA_SECRET_NAME` | CA certificate secret name | `<Name>-ca` |
| `ACW_CERT_SECRET_NAME` | Server certificate secret name | `<Name>-cert` |
| `ACW_CA_BUNDLE_CONFIGMAP_NAME` | CA bundle configmap name | `<Name>-ca-bundle` |
| `ACW_CA_VALIDITY` | CA certificate validity (e.g., `48h`) | `48h` |
| `ACW_CA_REFRESH` | CA certificate refresh interval | `24h` |
| `ACW_CERT_VALIDITY` | Server certificate validity | `24h` |
| `ACW_CERT_REFRESH` | Server certificate refresh interval | `12h` |
| `ACW_LEADER_ELECTION` | Enable leader election | `true` |
| `ACW_LEADER_ELECTION_ID` | Leader election lease name | `<Name>-leader` |
| `ACW_LEASE_DURATION` | Leader election lease duration | `30s` |
| `ACW_RENEW_DEADLINE` | Leader election renew deadline | `10s` |
| `ACW_RETRY_PERIOD` | Leader election retry period | `5s` |
| `POD_NAMESPACE` | Namespace (backward compatibility) | Auto-detected |
| `POD_NAME` | Pod identity for leader election | hostname |

The namespace is automatically detected from `/var/run/secrets/kubernetes.io/serviceaccount/namespace` (mounted by Kubernetes). You only need to set `ACW_NAMESPACE` or `POD_NAMESPACE` if running outside a Kubernetes cluster or without a ServiceAccount.

## Metrics

The framework exposes Prometheus metrics on a separate HTTP port (default: 8080).

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `admission_webhook_certificate_expiry_timestamp_seconds` | Gauge | `type` | Certificate expiry timestamp (unix seconds) |
| `admission_webhook_certificate_not_before_timestamp_seconds` | Gauge | `type` | Certificate not-before timestamp (unix seconds) |
| `admission_webhook_certificate_valid_duration_seconds` | Gauge | `type` | Total certificate validity duration (seconds) |
| `admission_webhook_leader_info` | Gauge | `namespace`, `lease`, `holder_identity` | Current leader identity for the lease. `holder_identity=""` means no leader is currently held |
| `admission_webhook_has_leader` | Gauge | `namespace`, `lease` | Whether the leader election lease currently has a holder (`1` = yes, `0` = no) |

Recommended alerts:

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
  - alert: WebhookCertificateExpiringIn59Days
    expr: (admission_webhook_certificate_expiry_timestamp_seconds{type="serving"} - time()) / 86400 < 59
    for: 1h
    labels:
      severity: warning
    annotations:
      summary: "Webhook serving certificate expires in less than 59 days"
  - alert: WebhookHasNoLeader
    expr: admission_webhook_has_leader == 0
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Webhook leader election currently has no leader"
```

Example queries:

```promql
# Remaining serving certificate validity in days
(admission_webhook_certificate_expiry_timestamp_seconds{type="serving"} - time()) / 86400

# Current leader state
admission_webhook_leader_info

# Leases currently without a leader
admission_webhook_leader_info{holder_identity=""} == 1
```

In single-replica mode with leader election disabled, the framework reports the current instance as leader so the same alerts and dashboards keep working.

## Examples

Complete working examples with deployment manifests and test scripts:

| Example | Type | Description |
|---------|------|-------------|
| [pod-mutating](./examples/pod-mutating) | Mutating Webhook | Injects labels into pods automatically |
| [pod-validating](./examples/pod-validating) | Validating Webhook | Enforces pod policies (labels, image tags, resource limits) |

Each example includes:
- Complete Go implementation
- Dockerfile for container builds
- Makefile with `docker-build-push`, `deploy`, `undeploy`, and `test` targets
- Kubernetes manifests (namespace, RBAC, deployment, service, webhook configuration)
- Test script for validation

## License

Apache-2.0
