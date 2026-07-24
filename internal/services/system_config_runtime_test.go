package services

import (
	"context"
	"testing"
	"time"

	appconfig "github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/repository/db"
)

func TestResolveVLMAnalyzerConfigPrefersSystemConfigValues(t *testing.T) {
	service := NewSystemConfigService()
	_, _ = service.Upsert(SystemConfig{Key: "vlm.provider", Value: "mock", Type: "string"})
	_, _ = service.Upsert(SystemConfig{Key: "vlm.model", Value: "vision-v1", Type: "string"})
	_, _ = service.Upsert(SystemConfig{Key: openAIBaseURLKey, Value: "https://models.example.test/v1", Type: "string"})
	_, _ = service.Upsert(SystemConfig{Key: openAIAPIKeyKey, Value: "secret-key", Type: "string"})
	_, _ = service.Upsert(SystemConfig{Key: "vlm.timeout_seconds", Value: 45, Type: "number"})
	_, _ = service.Upsert(SystemConfig{Key: "vlm.max_tokens", Value: 4096, Type: "number"})
	_, _ = service.Upsert(SystemConfig{Key: "vlm.max_retries", Value: 4, Type: "number"})

	resolved := ResolveVLMAnalyzerConfig(service, appconfig.Config{
		VLMProvider:         "openai_compatible",
		VLMModel:            "fallback-model",
		VLMBaseURL:          "https://fallback.example.test/v1",
		VLMAPIKey:           "fallback-key",
		VLMMaxTokens:        8192,
		ModelGatewayTimeout: 2 * time.Minute,
		VLMMaxRetries:       1,
	})

	if resolved.Provider != "mock" {
		t.Fatalf("expected provider from system config, got %q", resolved.Provider)
	}
	if resolved.Model != "vision-v1" {
		t.Fatalf("expected model from system config, got %q", resolved.Model)
	}
	if resolved.BaseURL != "https://models.example.test/v1" {
		t.Fatalf("expected base url from system config, got %q", resolved.BaseURL)
	}
	if resolved.APIKey != "secret-key" {
		t.Fatalf("expected api key from system config, got %q", resolved.APIKey)
	}
	if resolved.MaxTokens != 4096 {
		t.Fatalf("expected max tokens from system config, got %d", resolved.MaxTokens)
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
		VLMBaseURL:          "https://fallback.example.test/v1",
		VLMAPIKey:           "fallback-key",
		VLMMaxTokens:        8192,
		ModelGatewayTimeout: 90 * time.Second,
		VLMMaxRetries:       3,
	})

	if resolved.Provider != "mock" {
		t.Fatalf("expected fallback provider, got %q", resolved.Provider)
	}
	if resolved.Model != "fallback-model" {
		t.Fatalf("expected fallback model, got %q", resolved.Model)
	}
	if resolved.BaseURL != "https://fallback.example.test/v1" {
		t.Fatalf("expected fallback base url, got %q", resolved.BaseURL)
	}
	if resolved.APIKey != "fallback-key" {
		t.Fatalf("expected fallback api key, got %q", resolved.APIKey)
	}
	if resolved.MaxTokens != 8192 {
		t.Fatalf("expected fallback max tokens, got %d", resolved.MaxTokens)
	}
	if resolved.Timeout != 90*time.Second {
		t.Fatalf("expected fallback timeout, got %v", resolved.Timeout)
	}
	if resolved.MaxRetries != 3 {
		t.Fatalf("expected fallback retries, got %d", resolved.MaxRetries)
	}
}

func TestResolveLLMScriptConfigLeavesMaxTokensUnsetWithoutSystemConfig(t *testing.T) {
	resolved := ResolveLLMScriptConfigWithProviders(context.Background(), NewSystemConfigService(), nil, appconfig.Config{
		VLMBaseURL:          "https://fallback.example.test/v1",
		VLMAPIKey:           "fallback-key",
		ModelGatewayTimeout: 90 * time.Second,
	})

	if resolved.MaxTokens != 0 {
		t.Fatalf("expected unset LLM max tokens, got %d", resolved.MaxTokens)
	}
}

func TestRuntimeModelResolversRefreshSharedConfig(t *testing.T) {
	queries := &mutableSystemConfigQuerier{rows: []db.SystemConfig{
		{ConfigKey: "llm.model", ConfigValue: []byte(`"fresh-llm"`), ConfigType: "string"},
		{ConfigKey: "vlm.model", ConfigValue: []byte(`"fresh-vlm"`), ConfigType: "string"},
		{ConfigKey: "embedding.model", ConfigValue: []byte(`"fresh-embedding"`), ConfigType: "string"},
	}}
	service := &SystemConfigService{
		configs: map[string]SystemConfig{
			"llm.model":       {Key: "llm.model", Value: "stale-llm", Type: "string"},
			"vlm.model":       {Key: "vlm.model", Value: "stale-vlm", Type: "string"},
			"embedding.model": {Key: "embedding.model", Value: "stale-embedding", Type: "string"},
		},
		queries: queries,
	}
	fallback := appconfig.Config{VLMProvider: "mock"}

	if resolved := ResolveLLMScriptConfigWithProviders(context.Background(), service, nil, fallback); resolved.Model != "fresh-llm" {
		t.Fatalf("expected refreshed LLM model, got %q", resolved.Model)
	}
	if resolved := ResolveVLMAnalyzerConfigWithProviders(context.Background(), service, nil, fallback); resolved.Model != "fresh-vlm" {
		t.Fatalf("expected refreshed VLM model, got %q", resolved.Model)
	}
	if resolved := ResolveEmbeddingConfigWithProviders(context.Background(), service, nil, fallback); resolved.Model != "fresh-embedding" {
		t.Fatalf("expected refreshed embedding model, got %q", resolved.Model)
	}
}
