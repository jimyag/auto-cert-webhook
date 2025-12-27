package certprovider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync/atomic"

	"github.com/jimyag/auto-cert-webhook/internal/metrics"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// Provider provides dynamic TLS certificates loaded from Kubernetes secrets.
type Provider struct {
	client    kubernetes.Interface
	namespace string
	name      string

	current atomic.Pointer[tls.Certificate]
	ready   atomic.Bool
}

// New creates a new certificate provider.
func New(client kubernetes.Interface, namespace, secretName string) *Provider {
	return &Provider{
		client:    client,
		namespace: namespace,
		name:      secretName,
	}
}

// Start starts watching the secret and loading certificates.
func (p *Provider) Start(ctx context.Context) error {
	// Try to load the initial certificate
	if err := p.loadCertificate(ctx); err != nil {
		klog.Warningf("Initial certificate load failed (will retry via informer): %v", err)
	}

	// Set up informer to watch for changes
	factory := informers.NewSharedInformerFactoryWithOptions(
		p.client,
		0,
		informers.WithNamespace(p.namespace),
	)

	secretInformer := factory.Core().V1().Secrets().Informer()

	_, err := secretInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			secret := obj.(*corev1.Secret)
			if secret.Name == p.name {
				p.onSecretUpdate(secret)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			secret := newObj.(*corev1.Secret)
			if secret.Name == p.name {
				p.onSecretUpdate(secret)
			}
		},
		DeleteFunc: func(obj interface{}) {
			secret, ok := obj.(*corev1.Secret)
			if !ok {
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					return
				}
				secret, ok = tombstone.Obj.(*corev1.Secret)
				if !ok {
					return
				}
			}
			if secret.Name == p.name {
				klog.Warningf("Certificate secret %s/%s deleted", p.namespace, p.name)
				p.ready.Store(false)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	factory.Start(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), secretInformer.HasSynced) {
		return fmt.Errorf("failed to sync informer cache")
	}

	klog.Infof("Certificate provider started watching secret %s/%s", p.namespace, p.name)

	<-ctx.Done()
	return nil
}

// loadCertificate loads the certificate from the secret.
func (p *Provider) loadCertificate(ctx context.Context) error {
	secret, err := p.client.CoreV1().Secrets(p.namespace).Get(ctx, p.name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("Secret %s/%s not found yet", p.namespace, p.name)
			return nil
		}
		return err
	}

	p.onSecretUpdate(secret)
	return nil
}

// onSecretUpdate handles secret updates.
func (p *Provider) onSecretUpdate(secret *corev1.Secret) {
	certPEM, ok := secret.Data["tls.crt"]
	if !ok || len(certPEM) == 0 {
		klog.V(4).Infof("Secret %s/%s has no tls.crt data yet", p.namespace, p.name)
		return
	}

	keyPEM, ok := secret.Data["tls.key"]
	if !ok || len(keyPEM) == 0 {
		klog.V(4).Infof("Secret %s/%s has no tls.key data yet", p.namespace, p.name)
		return
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		klog.Errorf("Failed to parse certificate from secret %s/%s: %v", p.namespace, p.name, err)
		return
	}

	// Update metrics
	if cert.Leaf == nil {
		cert.Leaf, _ = x509.ParseCertificate(cert.Certificate[0])
	}
	if cert.Leaf != nil {
		metrics.UpdateCertMetrics("serving", cert.Leaf)
	}

	p.current.Store(&cert)
	p.ready.Store(true)
	klog.Infof("Certificate reloaded from secret %s/%s", p.namespace, p.name)
}

// GetCertificate returns the current certificate for TLS configuration.
func (p *Provider) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert := p.current.Load()
	if cert == nil {
		return nil, fmt.Errorf("certificate not yet loaded from secret %s/%s", p.namespace, p.name)
	}
	return cert, nil
}

// Ready returns true if the certificate is loaded and ready.
func (p *Provider) Ready() bool {
	return p.ready.Load()
}
