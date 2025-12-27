package leaderelection

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestGetIdentity(t *testing.T) {
	t.Run("from POD_NAME env", func(t *testing.T) {
		os.Setenv("POD_NAME", "test-pod-123")
		defer os.Unsetenv("POD_NAME")

		identity := getIdentity()
		if identity != "test-pod-123" {
			t.Errorf("identity: got %q, want %q", identity, "test-pod-123")
		}
	})

	t.Run("falls back to hostname", func(t *testing.T) {
		os.Unsetenv("POD_NAME")

		identity := getIdentity()
		if identity == "" {
			t.Error("identity should not be empty")
		}

		// Should be hostname
		hostname, _ := os.Hostname()
		if identity != hostname {
			t.Errorf("identity: got %q, want %q", identity, hostname)
		}
	})
}

func TestCallbacks(t *testing.T) {
	// Test that callbacks struct can be initialized
	var startedCalled bool
	var stoppedCalled bool
	var newLeaderCalled bool
	var newLeaderIdentity string

	callbacks := Callbacks{
		OnStartedLeading: func(ctx context.Context) {
			startedCalled = true
		},
		OnStoppedLeading: func() {
			stoppedCalled = true
		},
		OnNewLeader: func(identity string) {
			newLeaderCalled = true
			newLeaderIdentity = identity
		},
	}

	// Verify callbacks can be called
	callbacks.OnStartedLeading(nil)
	callbacks.OnStoppedLeading()
	callbacks.OnNewLeader("leader-1")

	if !startedCalled {
		t.Error("OnStartedLeading not called")
	}
	if !stoppedCalled {
		t.Error("OnStoppedLeading not called")
	}
	if !newLeaderCalled {
		t.Error("OnNewLeader not called")
	}
	if newLeaderIdentity != "leader-1" {
		t.Errorf("newLeaderIdentity: got %q, want %q", newLeaderIdentity, "leader-1")
	}
}

func TestConfig(t *testing.T) {
	config := Config{
		Namespace:     "webhook-system",
		Name:          "my-webhook-leader",
		LeaseDuration: 60 * time.Second,
		RenewDeadline: 20 * time.Second,
		RetryPeriod:   10 * time.Second,
	}

	if config.Namespace != "webhook-system" {
		t.Errorf("Namespace: got %q, want %q", config.Namespace, "webhook-system")
	}
	if config.Name != "my-webhook-leader" {
		t.Errorf("Name: got %q, want %q", config.Name, "my-webhook-leader")
	}
	if config.LeaseDuration != 60*time.Second {
		t.Errorf("LeaseDuration: got %v, want %v", config.LeaseDuration, 60*time.Second)
	}
}
