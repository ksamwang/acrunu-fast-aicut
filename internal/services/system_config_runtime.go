package services

import (
	appconfig "github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"time"
)

func ResolveVLMAnalyzerConfig(service *SystemConfigService, fallback appconfig.Config) modelgateway.Config {
	resolved := modelgateway.Config{
		Provider:   fallback.VLMProvider,
		Model:      fallback.VLMModel,
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
	if config, err := service.Get("vlm.timeout_seconds"); err == nil {
		if value := configIntValue(config.Value); value > 0 {
			resolved.Timeout = time.Duration(value) * time.Second
		}
	}
	if config, err := service.Get("vlm.max_retries"); err == nil {
		if value := configIntValue(config.Value); value >= 0 {
			resolved.MaxRetries = value
		}
	}

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
