package admission

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/jimyag/auto-cert-webhook/pkg/cabundle"
	"github.com/jimyag/auto-cert-webhook/pkg/certmanager"
	"github.com/jimyag/auto-cert-webhook/pkg/certprovider"
	"github.com/jimyag/auto-cert-webhook/pkg/leaderelection"
	"github.com/jimyag/auto-cert-webhook/pkg/metrics"
	"github.com/jimyag/auto-cert-webhook/pkg/server"
	"github.com/jimyag/auto-cert-webhook/pkg/webhook"
)

// Run starts the webhook server with the given webhook implementation.
// This is the main entry point for using this library.
func Run(wh Webhook, opts ...Option) error {
	// Setup signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return RunWithContext(ctx, wh, opts...)
}

// RunWithContext starts the webhook server with the given context.
func RunWithContext(ctx context.Context, wh Webhook, opts ...Option) error {
	// Get webhook configuration
	webhookConfig := wh.Configure()

	// Apply default configuration
	config := DefaultConfig()
	config.ApplyWebhookConfig(webhookConfig)

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	klog.Infof("Starting webhook %s in namespace %s", webhookConfig.Name, config.Namespace)

	// Create Kubernetes client
	k8sCfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	client, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Determine webhook types and refs
	webhookRefs := determineWebhookRefs(wh, webhookConfig)

	// Create certificate provider (runs on all pods)
	certProvider := certprovider.New(client, config.Namespace, config.CertSecretName)

	// Start certificate provider in background
	go func() {
		if err := certProvider.Start(ctx); err != nil {
			klog.Errorf("Certificate provider error: %v", err)
		}
	}()

	// Create and start HTTP server (runs on all pods)
	srv := server.New(certProvider, server.Config{
		Port:        config.Port,
		HealthzPath: config.HealthzPath,
		ReadyzPath:  config.ReadyzPath,
	})
	registerWebhookHandlers(srv, wh, webhookConfig)

	// Start HTTP server in background
	go func() {
		if err := srv.Start(ctx); err != nil {
			klog.Errorf("Server error: %v", err)
		}
	}()

	// Start metrics server if enabled
	if config.MetricsEnabled {
		metricsSrv := metrics.NewServer(metrics.ServerConfig{
			Port: config.MetricsPort,
			Path: config.MetricsPath,
		})
		go func() {
			if err := metricsSrv.Start(ctx); err != nil {
				klog.Errorf("Metrics server error: %v", err)
			}
		}()
	}

	// Create certificate manager and CA bundle syncer (runs on leader only)
	certMgr := certmanager.New(client, certmanager.Config{
		Namespace:             config.Namespace,
		ServiceName:           config.ServiceName,
		CASecretName:          config.CASecretName,
		CertSecretName:        config.CertSecretName,
		CABundleConfigMapName: config.CABundleConfigMapName,
		CAValidity:            config.CAValidity,
		CARefresh:             config.CARefresh,
		CertValidity:          config.CertValidity,
		CertRefresh:           config.CertRefresh,
	})

	caBundleSyncer := cabundle.NewSyncer(client, config.Namespace, config.CABundleConfigMapName, webhookRefs)

	if config.LeaderElection {
		// Run with leader election
		return leaderelection.Run(ctx, client, leaderelection.Config{
			Namespace:     config.Namespace,
			Name:          config.LeaderElectionID,
			LeaseDuration: config.LeaseDuration,
			RenewDeadline: config.RenewDeadline,
			RetryPeriod:   config.RetryPeriod,
		}, leaderelection.Callbacks{
			OnStartedLeading: func(leaderCtx context.Context) {
				klog.Info("Became leader, starting certificate management")

				// Start certificate manager
				go func() {
					if err := certMgr.Start(leaderCtx); err != nil {
						klog.Errorf("Certificate manager error: %v", err)
					}
				}()

				// Start CA bundle syncer
				go func() {
					if err := caBundleSyncer.Start(leaderCtx); err != nil {
						klog.Errorf("CA bundle syncer error: %v", err)
					}
				}()
			},
			OnStoppedLeading: func() {
				klog.Info("Lost leadership")
			},
		})
	}

	// Run without leader election (single replica mode)
	klog.Info("Running without leader election")

	// Start certificate manager
	go func() {
		if err := certMgr.Start(ctx); err != nil {
			klog.Errorf("Certificate manager error: %v", err)
		}
	}()

	// Start CA bundle syncer
	go func() {
		if err := caBundleSyncer.Start(ctx); err != nil {
			klog.Errorf("CA bundle syncer error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	klog.Info("Shutting down")
	return nil
}

// determineWebhookRefs determines webhook references based on the webhook implementation.
func determineWebhookRefs(wh Webhook, webhookConfig WebhookConfig) []cabundle.WebhookRef {
	var refs []cabundle.WebhookRef

	if _, ok := wh.(webhook.ValidatingWebhook); ok {
		refs = append(refs, cabundle.WebhookRef{
			Name: webhookConfig.Name,
			Type: cabundle.ValidatingWebhook,
		})
	}

	if _, ok := wh.(webhook.MutatingWebhook); ok {
		refs = append(refs, cabundle.WebhookRef{
			Name: webhookConfig.Name,
			Type: cabundle.MutatingWebhook,
		})
	}

	return refs
}

// registerWebhookHandlers registers webhook handlers based on the webhook implementation.
func registerWebhookHandlers(srv *server.Server, wh Webhook, webhookConfig WebhookConfig) {
	if v, ok := wh.(webhook.ValidatingWebhook); ok {
		path := webhookConfig.ValidatePath
		if path == "" {
			path = "/validate"
		}
		srv.RegisterValidatingWebhook(path, v)
	}

	if m, ok := wh.(webhook.MutatingWebhook); ok {
		path := webhookConfig.MutatePath
		if path == "" {
			path = "/mutate"
		}
		srv.RegisterMutatingWebhook(path, m)
	}
}
