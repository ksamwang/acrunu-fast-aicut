package httpserver

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func TestDynamicConcurrencyGateAppliesLowerLimitWithoutInterruptingActiveWork(t *testing.T) {
	gate := newDynamicConcurrencyGate()
	gate.pollInterval = 5 * time.Millisecond
	var limit atomic.Int64
	limit.Store(2)
	currentLimit := func() int { return int(limit.Load()) }

	if err := gate.acquire(context.Background(), currentLimit); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	if err := gate.acquire(context.Background(), currentLimit); err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}
	limit.Store(1)

	acquired := make(chan struct{})
	go func() {
		if gate.acquire(context.Background(), currentLimit) == nil {
			close(acquired)
		}
	}()
	assertGateBlocked(t, acquired)

	gate.release()
	assertGateBlocked(t, acquired)

	gate.release()
	select {
	case <-acquired:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("waiting request did not start after active work dropped below the new limit")
	}
	gate.release()
}

func TestDynamicConcurrencyGateAppliesHigherLimitWithoutRestart(t *testing.T) {
	gate := newDynamicConcurrencyGate()
	gate.pollInterval = 5 * time.Millisecond
	var limit atomic.Int64
	limit.Store(1)
	currentLimit := func() int { return int(limit.Load()) }

	if err := gate.acquire(context.Background(), currentLimit); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	acquired := make(chan struct{})
	go func() {
		if gate.acquire(context.Background(), currentLimit) == nil {
			close(acquired)
		}
	}()
	assertGateBlocked(t, acquired)

	limit.Store(2)
	select {
	case <-acquired:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("waiting request did not observe the increased limit")
	}
	gate.release()
	gate.release()
}

func TestServerVLMMaxConcurrencyReadsRuntimeSettings(t *testing.T) {
	settings := services.NewSystemConfigService()
	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		SystemConfigService: settings,
	})
	if got := server.vlmMaxConcurrency(); got != 2 {
		t.Fatalf("expected default VLM concurrency 2, got %d", got)
	}
	if _, err := settings.Upsert(services.SystemConfig{Key: "vlm.max_concurrency", Value: 5, Type: "number"}); err != nil {
		t.Fatalf("update VLM concurrency: %v", err)
	}
	if got := server.vlmMaxConcurrency(); got != 5 {
		t.Fatalf("expected updated VLM concurrency 5, got %d", got)
	}
}

func assertGateBlocked(t *testing.T, acquired <-chan struct{}) {
	t.Helper()
	select {
	case <-acquired:
		t.Fatal("request acquired a concurrency slot too early")
	case <-time.After(30 * time.Millisecond):
	}
}
