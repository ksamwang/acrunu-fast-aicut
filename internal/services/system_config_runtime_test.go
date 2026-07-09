package services

import (
	"testing"
	"time"

	appconfig "github.com/ksamwang/acrunu-fast-aicut/internal/config"
)

func TestResolveVLMAnalyzerConfigPrefersSystemConfigValues(t *testing.T) {
	service := NewSystemConfigService()
	_, _ = service.Upsert(SystemConfig{Key: "vlm.provider", Value: "mock", Type: "string"})
	_, _ = service.Upsert(SystemConfig{Key: "vlm.model", Value: "vision-v1", Type: "string"})
	_, _ = service.Upsert(SystemConfig{Key: "vlm.timeout_seconds", Value: 45, Type: "number"})
	_, _ = service.Upsert(SystemConfig{Key: "vlm.max_retries", Value: 4, Type: "number"})

	resolved := ResolveVLMAnalyzerConfig(service, appconfig.Config{
		VLMProvider:         "openai_compatible",
		VLMModel:            "fallback-model",
		ModelGatewayTimeout: 2 * time.Minute,
		VLMMaxRetries:       1,
	})

	if resolved.Provider != "mock" {
		t.Fatalf("expected provider from system config, got %q", resolved.Provider)
	}
	if resolved.Model != "vision-v1" {
		t.Fatalf("expected model from system config, got %q", resolved.Model)
	}
	if resolved.Timeout != 45*time.Second {
		t.Fatalf("expected timeout from system config, got %v", resolved.Timeout)
	}
	if resolved.MaxRetries != 4 {
		t.Fatalf("expected retries from system config, got %d", resolved.MaxRetries)
	}
}

func TestResolveVLMAnalyzerConfigFallsBackToEnvConfig(t *testing.T) {
	resolved := ResolveVLMAnalyzerConfig(nil, appconfig.Config{
		VLMProvider:         "mock",
		VLMModel:            "fallback-model",
		ModelGatewayTimeout: 90 * time.Second,
		VLMMaxRetries:       3,
	})

	if resolved.Provider != "mock" {
		t.Fatalf("expected fallback provider, got %q", resolved.Provider)
	}
	if resolved.Model != "fallback-model" {
		t.Fatalf("expected fallback model, got %q", resolved.Model)
	}
	if resolved.Timeout != 90*time.Second {
		t.Fatalf("expected fallback timeout, got %v", resolved.Timeout)
	}
	if resolved.MaxRetries != 3 {
		t.Fatalf("expected fallback retries, got %d", resolved.MaxRetries)
	}
}
