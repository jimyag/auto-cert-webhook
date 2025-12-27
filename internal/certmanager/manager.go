package certmanager

import (
	"context"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/openshift/library-go/pkg/crypto"
	"github.com/openshift/library-go/pkg/operator/certrotation"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
)

// Config holds the certificate manager configuration.
type Config struct {
	// Namespace is the namespace where certificates are stored.
	Namespace string

	// ServiceName is the name of the service for the webhook.
	ServiceName string

	// CASecretName is the name of the CA secret.
	CASecretName string

	// CertSecretName is the name of the serving certificate secret.
	CertSecretName string

	// CABundleConfigMapName is the name of the CA bundle configmap.
	CABundleConfigMapName string

	// CAValidity is the validity duration of the CA certificate.
	CAValidity time.Duration

	// CARefresh is the refresh interval for the CA certificate.
	CARefresh time.Duration

	// CertValidity is the validity duration of the server certificate.
	CertValidity time.Duration

	// CertRefresh is the refresh interval for the server certificate.
	CertRefresh time.Duration
}

// Manager handles certificate rotation using openshift/library-go.
type Manager struct {
	config Config

	k8sClient     kubernetes.Interface
	informers     v1helpers.KubeInformersForNamespaces
	eventRecorder events.Recorder

	secretLister    listerscorev1.SecretLister
	configMapLister listerscorev1.ConfigMapLister
}

// New creates a new certificate manager.
func New(client kubernetes.Interface, config Config) *Manager {
	informers := v1helpers.NewKubeInformersForNamespaces(client, config.Namespace)

	controllerRef, err := events.GetControllerReferenceForCurrentPod(context.TODO(), client, config.Namespace, nil)
	if err != nil {
		klog.V(4).Infof("Unable to get controller reference: %v", err)
	}

	eventRecorder := events.NewRecorder(client.CoreV1().Events(config.Namespace), config.Namespace, controllerRef, clock.RealClock{})

	return &Manager{
		config:        config,
		k8sClient:     client,
		informers:     informers,
		eventRecorder: eventRecorder,
	}
}

// Start starts the certificate manager and blocks until the context is cancelled.
func (m *Manager) Start(ctx context.Context) error {
	// Start informers
	m.informers.Start(ctx.Done())

	secretInformer := m.informers.InformersFor(m.config.Namespace).Core().V1().Secrets().Informer()
	go secretInformer.Run(ctx.Done())

	configMapInformer := m.informers.InformersFor(m.config.Namespace).Core().V1().ConfigMaps().Informer()
	go configMapInformer.Run(ctx.Done())

	if !toolscache.WaitForCacheSync(ctx.Done(), secretInformer.HasSynced, configMapInformer.HasSynced) {
		return fmt.Errorf("could not sync informer cache")
	}

	m.secretLister = m.informers.InformersFor(m.config.Namespace).Core().V1().Secrets().Lister()
	m.configMapLister = m.informers.InformersFor(m.config.Namespace).Core().V1().ConfigMaps().Lister()

	// Start the sync loop
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	// Run immediately on start
	if err := m.sync(ctx); err != nil {
		klog.Errorf("Initial certificate sync failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			klog.Info("Certificate manager stopped")
			return nil
		case <-ticker.C:
			if err := m.sync(ctx); err != nil {
				klog.Errorf("Certificate sync failed: %v", err)
			}
		}
	}
}

// sync performs a single synchronization cycle.
func (m *Manager) sync(ctx context.Context) error {
	klog.V(4).Info("Syncing certificates")

	// Ensure CA
	ca, err := m.ensureCA(ctx)
	if err != nil {
		return fmt.Errorf("failed to ensure CA: %w", err)
	}

	// Ensure CA Bundle
	bundle, err := m.ensureCABundle(ctx, ca)
	if err != nil {
		return fmt.Errorf("failed to ensure CA bundle: %w", err)
	}

	// Ensure serving certificate
	if err := m.ensureServingCert(ctx, ca, bundle); err != nil {
		return fmt.Errorf("failed to ensure serving certificate: %w", err)
	}

	klog.V(4).Info("Certificate sync completed")
	return nil
}

// ensureCA ensures the CA certificate exists and is valid.
func (m *Manager) ensureCA(ctx context.Context) (*crypto.CA, error) {
	secret, err := m.secretLister.Secrets(m.config.Namespace).Get(m.config.CASecretName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}

		secret, err = m.createSecret(ctx, m.config.Namespace, m.config.CASecretName)
		if err != nil {
			return nil, err
		}
	}

	sr := certrotation.RotatedSigningCASecret{
		Name:          secret.Name,
		Namespace:     secret.Namespace,
		Validity:      m.config.CAValidity,
		Refresh:       m.config.CARefresh,
		Lister:        m.secretLister,
		Client:        m.k8sClient.CoreV1(),
		EventRecorder: m.eventRecorder,
	}

	ca, _, err := sr.EnsureSigningCertKeyPair(ctx)
	if err != nil {
		return nil, err
	}

	return ca, nil
}

// ensureCABundle ensures the CA bundle configmap exists and contains the current CA.
func (m *Manager) ensureCABundle(ctx context.Context, ca *crypto.CA) ([]*x509.Certificate, error) {
	br := certrotation.CABundleConfigMap{
		Name:          m.config.CABundleConfigMapName,
		Namespace:     m.config.Namespace,
		Lister:        m.configMapLister,
		Client:        m.k8sClient.CoreV1(),
		EventRecorder: m.eventRecorder,
	}

	signerName := fmt.Sprintf("%s/%s", m.config.Namespace, m.config.CASecretName)
	certs, err := br.EnsureConfigMapCABundle(ctx, ca, signerName)
	if err != nil {
		return nil, err
	}

	return certs, nil
}

// ensureServingCert ensures the serving certificate exists and is valid.
func (m *Manager) ensureServingCert(ctx context.Context, ca *crypto.CA, bundle []*x509.Certificate) error {
	secret, err := m.secretLister.Secrets(m.config.Namespace).Get(m.config.CertSecretName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		secret, err = m.createSecret(ctx, m.config.Namespace, m.config.CertSecretName)
		if err != nil {
			return err
		}
	}

	tr := certrotation.RotatedSelfSignedCertKeySecret{
		Name:      secret.Name,
		Namespace: secret.Namespace,
		Validity:  m.config.CertValidity,
		Refresh:   m.config.CertRefresh,
		CertCreator: &certrotation.ServingRotation{
			Hostnames: func() []string {
				return []string{
					m.config.ServiceName,
					fmt.Sprintf("%s.%s", m.config.ServiceName, m.config.Namespace),
					fmt.Sprintf("%s.%s.svc", m.config.ServiceName, m.config.Namespace),
				}
			},
		},
		Lister:        m.secretLister,
		Client:        m.k8sClient.CoreV1(),
		EventRecorder: m.eventRecorder,
	}

	if _, err := tr.EnsureTargetCertKeyPair(ctx, ca, bundle); err != nil {
		return err
	}

	return nil
}

// createSecret creates a new TLS secret.
func (m *Manager) createSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": {},
			"tls.key": {},
		},
	}

	return m.k8sClient.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
}

// GetCABundle returns the current CA bundle from the configmap.
func (m *Manager) GetCABundle(ctx context.Context) ([]byte, error) {
	cm, err := m.k8sClient.CoreV1().ConfigMaps(m.config.Namespace).Get(ctx, m.config.CABundleConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	caBundle, ok := cm.Data["ca-bundle.crt"]
	if !ok {
		return nil, fmt.Errorf("ca-bundle.crt not found in configmap %s/%s", m.config.Namespace, m.config.CABundleConfigMapName)
	}

	return []byte(caBundle), nil
}
