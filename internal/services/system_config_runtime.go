package services

import (
	"context"
	"time"

	appconfig "github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

func ResolveVLMAnalyzerConfig(service *SystemConfigService, fallback appconfig.Config) modelgateway.Config {
	resolved := modelgateway.Config{
		Provider:   fallback.VLMProvider,
		Model:      fallback.VLMModel,
		BaseURL:    fallback.VLMBaseURL,
		APIKey:     fallback.VLMAPIKey,
		MaxTokens:  fallback.VLMMaxTokens,
		Timeout:    fallback.ModelGatewayTimeout,
		MaxRetries: fallback.VLMMaxRetries,
	}
	if service == nil {
		return resolved
	}

	if config, err := service.Get("vlm.provider"); err == nil {
		if value := configStringValue(config.Value); value != "" {
			resolved.Provider = value
		}
	}
	if config, err := service.Get("vlm.model"); err == nil {
		resolved.Model = configStringValue(config.Value)
	}
	if config, err := service.Get(openAIBaseURLKey); err == nil {
		resolved.BaseURL = configStringValue(config.Value)
	}
	if config, err := service.Get(openAIAPIKeyKey); err == nil {
		resolved.APIKey = configStringValue(config.Value)
	}
	if config, err := service.Get("vlm.timeout_seconds"); err == nil {
		if value := configIntValue(config.Value); value > 0 {
			resolved.Timeout = time.Duration(value) * time.Second
		}
	}
	if config, err := service.Get("vlm.max_tokens"); err == nil {
		if value := configIntValue(config.Value); value > 0 {
			resolved.MaxTokens = value
		}
	}
	if config, err := service.Get("vlm.max_retries"); err == nil {
		if value := configIntValue(config.Value); value >= 0 {
			resolved.MaxRetries = value
		}
	}

	return resolved
}

func ResolveVLMAnalyzerConfigWithProviders(ctx context.Context, service *SystemConfigService, providerService *ModelProviderService, fallback appconfig.Config) modelgateway.Config {
	resolved := ResolveVLMAnalyzerConfig(service, fallback)
	if service == nil || providerService == nil {
		return resolved
	}
	config, err := service.Get("vlm.provider_id")
	if err != nil {
		return resolved
	}
	providerID := configStringValue(config.Value)
	if providerID == "" {
		return resolved
	}
	access, err := providerService.GetAccess(ctx, providerID)
	if err != nil || !access.Enabled {
		return resolved
	}
	resolved.Provider = access.ProviderType
	resolved.BaseURL = access.BaseURL
	resolved.APIKey = access.APIKey
	return resolved
}

func ResolveEmbeddingConfigWithProviders(ctx context.Context, service *SystemConfigService, providerService *ModelProviderService, fallback appconfig.Config) modelgateway.Config {
	resolved := modelgateway.Config{
		Provider:   "mock",
		Model:      "text-embedding-v4",
		Dimensions: 1024,
		Timeout:    fallback.ModelGatewayTimeout,
	}
	if service == nil {
		return resolved
	}
	if config, err := service.Get("embedding.model"); err == nil {
		if value := configStringValue(config.Value); value != "" {
			resolved.Model = value
		}
	}
	if config, err := service.Get("embedding.dimension"); err == nil {
		if value := configIntValue(config.Value); value > 0 {
			resolved.Dimensions = value
		}
	}
	if providerService == nil {
		return resolved
	}
	config, err := service.Get("embedding.provider_id")
	if err != nil {
		return resolved
	}
	providerID := configStringValue(config.Value)
	if providerID == "" {
		return resolved
	}
	access, err := providerService.GetAccess(ctx, providerID)
	if err != nil || !access.Enabled {
		return resolved
	}
	resolved.Provider = access.ProviderType
	resolved.BaseURL = access.BaseURL
	resolved.APIKey = access.APIKey
	return resolved
}

func configStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func configIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}
