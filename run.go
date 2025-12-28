package autocertwebhook

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"

	"github.com/kelseyhightower/envconfig"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/jimyag/auto-cert-webhook/internal/cabundle"
	"github.com/jimyag/auto-cert-webhook/internal/certmanager"
	"github.com/jimyag/auto-cert-webhook/internal/certprovider"
	"github.com/jimyag/auto-cert-webhook/internal/leaderelection"
	"github.com/jimyag/auto-cert-webhook/internal/metrics"
	"github.com/jimyag/auto-cert-webhook/internal/server"
)

const (
	// serviceAccountNamespaceFile is the path to the namespace file
	// automatically mounted by Kubernetes in pods with ServiceAccount.
	serviceAccountNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

	// envPrefix is the environment variable prefix for all configuration.
	envPrefix = "ACW"

	// Default namespace when not auto-detected
	defaultNamespace = "default"
)

// Run starts the webhook server with the given Admission implementation.
// This is the main entry point for using this library.
func Run(admission Admission) error {
	// Setup signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return RunWithContext(ctx, admission)
}

// RunWithContext starts the webhook server with the given context.
func RunWithContext(ctx context.Context, admission Admission) error {
	// Get user configuration
	cfg := admission.Configure()
	hooks := admission.Webhooks()

	// Apply environment variables (priority: code > env > default)
	if err := applyEnvConfig(&cfg); err != nil {
		return err
	}

	if cfg.Name == "" {
		return fmt.Errorf("webhook name is required in Configure() or ACW_NAME environment variable")
	}

	if len(hooks) == 0 {
		return fmt.Errorf("at least one webhook hook is required in Webhooks()")
	}

	// Validate hooks
	seenPaths := make(map[string]int)
	for i, hook := range hooks {
		if hook.Path == "" {
			return fmt.Errorf("hook[%d]: path is required", i)
		}
		if hook.Path[0] != '/' {
			return fmt.Errorf("hook[%d]: path must start with '/'", i)
		}
		if prev, exists := seenPaths[hook.Path]; exists {
			return fmt.Errorf("hook[%d]: path %q already defined by hook[%d]", i, hook.Path, prev)
		}
		seenPaths[hook.Path] = i
		if hook.Admit == nil {
			return fmt.Errorf("hook[%d]: admit function is required", i)
		}
		if hook.Type != Mutating && hook.Type != Validating {
			return fmt.Errorf("hook[%d]: type must be Mutating or Validating", i)
		}
	}

	// Apply defaults for any remaining unset values
	applyDefaults(&cfg)

	// Validate certificate durations
	if err := validateCertDurations(&cfg); err != nil {
		return err
	}

	klog.Infof("Starting webhook %s in namespace %s", cfg.Name, cfg.Namespace)

	// Create Kubernetes client
	k8sCfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	client, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	errCh := make(chan error, 6) // Buffer for: certProvider, server, metrics, certManager, caBundleSyncer, leaderElection

	// Determine webhook refs for CA bundle syncer
	webhookRefs := determineWebhookRefs(cfg.Name, hooks)

	// Create certificate provider (runs on all pods)
	certProvider := certprovider.New(client, cfg.Namespace, cfg.CertSecretName)

	// Start certificate provider in background
	go func() {
		if err := certProvider.Start(ctx); err != nil {
			klog.Errorf("Certificate provider error: %v", err)
			errCh <- err
		}
	}()

	// Create and start HTTP server (runs on all pods)
	srv := server.New(certProvider, server.Config{
		Port:        cfg.Port,
		HealthzPath: cfg.HealthzPath,
		ReadyzPath:  cfg.ReadyzPath,
	})

	// Register webhook handlers
	for _, hook := range hooks {
		srv.RegisterHook(hook.Path, string(hook.Type), hook.Admit)
		klog.Infof("Registered %s webhook at path %s", hook.Type, hook.Path)
	}

	// Start HTTP server in background
	go func() {
		if err := srv.Start(ctx); err != nil {
			klog.Errorf("Server error: %v", err)
			errCh <- err
		}
	}()

	// Start metrics server if enabled
	metricsEnabled := cfg.MetricsEnabled == nil || *cfg.MetricsEnabled
	if metricsEnabled {
		metricsSrv := metrics.NewServer(metrics.ServerConfig{
			Port: cfg.MetricsPort,
			Path: cfg.MetricsPath,
		})
		go func() {
			if err := metricsSrv.Start(ctx); err != nil {
				klog.Errorf("Metrics server error: %v", err)
				errCh <- err
			}
		}()
	}

	// Create certificate manager and CA bundle syncer (runs on leader only)
	certMgr := certmanager.New(client, certmanager.Config{
		Namespace:             cfg.Namespace,
		ServiceName:           cfg.ServiceName,
		CASecretName:          cfg.CASecretName,
		CertSecretName:        cfg.CertSecretName,
		CABundleConfigMapName: cfg.CABundleConfigMapName,
		CAValidity:            cfg.CAValidity,
		CARefresh:             cfg.CARefresh,
		CertValidity:          cfg.CertValidity,
		CertRefresh:           cfg.CertRefresh,
		SyncInterval:          cfg.CertSyncInterval,
	})

	caBundleSyncer := cabundle.NewSyncer(client, cfg.Namespace, cfg.CABundleConfigMapName, webhookRefs)

	leaderElectionEnabled := cfg.LeaderElection == nil || *cfg.LeaderElection
	if leaderElectionEnabled {
		// Run with leader election
		go func() {
			if err := leaderelection.Run(ctx, client, leaderelection.Config{
				Namespace:     cfg.Namespace,
				Name:          cfg.LeaderElectionID,
				LeaseDuration: cfg.LeaseDuration,
				RenewDeadline: cfg.RenewDeadline,
				RetryPeriod:   cfg.RetryPeriod,
			}, leaderelection.Callbacks{
				OnStartedLeading: func(leaderCtx context.Context) {
					klog.Info("Became leader, starting certificate management")
					startCertManagement(leaderCtx, certMgr, caBundleSyncer, errCh)
				},
				OnStoppedLeading: func() {
					klog.Info("Lost leadership")
				},
			}); err != nil {
				klog.Errorf("Leader election error: %v", err)
				errCh <- err
			}
		}()
	} else {
		// Run without leader election (single replica mode)
		klog.Info("Running without leader election")
		startCertManagement(ctx, certMgr, caBundleSyncer, errCh)
	}

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		klog.Info("Shutting down")
		return nil
	case err := <-errCh:
		klog.Errorf("Error: %v", err)
		return err
	}
}

// applyEnvConfig applies configuration from environment variables using envconfig.
// Priority: code > env > default (defaults are set via struct tags)
func applyEnvConfig(cfg *Config) error {
	// Deep copy user-defined values (including pointers)
	userCfg := deepCopyConfig(cfg)

	// Process environment variables (includes defaults from tags)
	if err := envconfig.Process(envPrefix, cfg); err != nil {
		return fmt.Errorf("failed to process environment variables: %w", err)
	}

	// Restore non-zero values from user code (code takes priority)
	userVal := reflect.ValueOf(userCfg).Elem()
	cfgVal := reflect.ValueOf(cfg).Elem()
	for i := 0; i < userVal.NumField(); i++ {
		userField := userVal.Field(i)
		if userField.Kind() == reflect.Ptr {
			if !userField.IsNil() {
				cfgVal.Field(i).Set(userField)
			}
		} else if !userField.IsZero() {
			cfgVal.Field(i).Set(userField)
		}
	}

	return nil
}

// deepCopyConfig creates a deep copy of Config, including pointer fields.
func deepCopyConfig(cfg *Config) *Config {
	copied := *cfg

	// Deep copy pointer fields using reflection
	cfgVal := reflect.ValueOf(cfg).Elem()
	copiedVal := reflect.ValueOf(&copied).Elem()

	for i := 0; i < cfgVal.NumField(); i++ {
		field := cfgVal.Field(i)
		if field.Kind() == reflect.Ptr && !field.IsNil() {
			// Create a new pointer and copy the value
			newPtr := reflect.New(field.Elem().Type())
			newPtr.Elem().Set(field.Elem())
			copiedVal.Field(i).Set(newPtr)
		}
	}

	return &copied
}

// applyDefaults applies dynamic defaults that depend on other config values.
func applyDefaults(cfg *Config) {
	// Namespace: auto-detect from ServiceAccount or POD_NAMESPACE
	if cfg.Namespace == "" {
		cfg.Namespace = getNamespace()
	}

	// ServiceName defaults to Name
	if cfg.ServiceName == "" {
		cfg.ServiceName = cfg.Name
	}

	// Resource names default to <Name>-suffix
	if cfg.CASecretName == "" {
		cfg.CASecretName = cfg.Name + "-ca"
	}
	if cfg.CertSecretName == "" {
		cfg.CertSecretName = cfg.Name + "-cert"
	}
	if cfg.CABundleConfigMapName == "" {
		cfg.CABundleConfigMapName = cfg.Name + "-ca-bundle"
	}
	if cfg.LeaderElectionID == "" {
		cfg.LeaderElectionID = cfg.Name + "-leader"
	}
}

// getNamespace returns the namespace from:
// 1. ACW_NAMESPACE environment variable (if set)
// 2. POD_NAMESPACE environment variable (if set)
// 3. ServiceAccount namespace file (auto-mounted by Kubernetes)
// 4. defaultNamespace as fallback
func getNamespace() string {
	// First, try ACW_NAMESPACE
	if ns := os.Getenv(envPrefix + "_NAMESPACE"); ns != "" {
		return ns
	}

	// Second, try POD_NAMESPACE (for backward compatibility)
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}

	// Third, try reading from ServiceAccount namespace file
	if data, err := os.ReadFile(serviceAccountNamespaceFile); err == nil {
		if ns := strings.TrimSpace(string(data)); ns != "" {
			return ns
		}
	}

	// Fallback to default
	return defaultNamespace
}

func startCertManagement(ctx context.Context, certMgr *certmanager.Manager, caBundleSyncer *cabundle.Syncer, errCh chan error) {
	go func() {
		if err := certMgr.Start(ctx); err != nil {
			klog.Errorf("Certificate manager error: %v", err)
			errCh <- err
		}
	}()

	go func() {
		if err := caBundleSyncer.Start(ctx); err != nil {
			klog.Errorf("CA bundle syncer error: %v", err)
			errCh <- err
		}
	}()
}

// validateCertDurations validates that certificate duration configurations are valid.
func validateCertDurations(cfg *Config) error {
	if cfg.CAValidity <= 0 {
		return fmt.Errorf("CA validity must be positive, got %v", cfg.CAValidity)
	}
	if cfg.CARefresh <= 0 {
		return fmt.Errorf("CA refresh must be positive, got %v", cfg.CARefresh)
	}
	if cfg.CertValidity <= 0 {
		return fmt.Errorf("cert validity must be positive, got %v", cfg.CertValidity)
	}
	if cfg.CertRefresh <= 0 {
		return fmt.Errorf("cert refresh must be positive, got %v", cfg.CertRefresh)
	}
	if cfg.CARefresh >= cfg.CAValidity {
		return fmt.Errorf("CA refresh (%v) must be less than CA validity (%v)", cfg.CARefresh, cfg.CAValidity)
	}
	if cfg.CertRefresh >= cfg.CertValidity {
		return fmt.Errorf("cert refresh (%v) must be less than cert validity (%v)", cfg.CertRefresh, cfg.CertValidity)
	}
	return nil
}

// determineWebhookRefs determines webhook references for CA bundle syncing.
func determineWebhookRefs(name string, hooks []Hook) []cabundle.WebhookRef {
	var refs []cabundle.WebhookRef
	seen := make(map[HookType]bool)

	for _, hook := range hooks {
		if seen[hook.Type] {
			continue
		}
		seen[hook.Type] = true

		var webhookType cabundle.WebhookType
		switch hook.Type {
		case Mutating:
			webhookType = cabundle.MutatingWebhook
		case Validating:
			webhookType = cabundle.ValidatingWebhook
		default:
			continue
		}
		refs = append(refs, cabundle.WebhookRef{
			Name: name,
			Type: webhookType,
		})
	}

	return refs
}
