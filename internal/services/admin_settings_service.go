package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	openAICompatibleProvider = "openai_compatible"
	openAIBaseURLKey         = "openai_compatible.base_url"
	openAIAPIKeyKey          = "openai_compatible.api_key"
)

type OpenAICompatibleSettings struct {
	Provider         string `json:"provider"`
	BaseURL          string `json:"base_url"`
	APIKeyConfigured bool   `json:"api_key_configured"`
	LLMModel         string `json:"llm_model"`
	VLMModel         string `json:"vlm_model"`
	EmbeddingModel   string `json:"embedding_model"`
}

type OpenAICompatibleSettingsUpdate struct {
	BaseURL        string `json:"base_url"`
	APIKey         string `json:"api_key"`
	LLMModel       string `json:"llm_model"`
	VLMModel       string `json:"vlm_model"`
	EmbeddingModel string `json:"embedding_model"`
}

type OpenAICompatibleResolveInput struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

type ModelOption struct {
	ID string `json:"id"`
}

type ModelDiscoveryResult struct {
	Models []ModelOption `json:"models"`
}

type ModelCapabilitySetting struct {
	Capability string `json:"capability"`
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
	Dimension  int    `json:"dimension,omitempty"`
}

type ModelCapabilitySettings struct {
	LLM       ModelCapabilitySetting `json:"llm"`
	VLM       ModelCapabilitySetting `json:"vlm"`
	Embedding ModelCapabilitySetting `json:"embedding"`
}

type RuntimeSettings struct {
	LLMMaxConcurrency     int `json:"llm_max_concurrency"`
	VLMMaxConcurrency     int `json:"vlm_max_concurrency"`
	ASRMaxConcurrency     int `json:"asr_max_concurrency"`
	TTSMaxConcurrency     int `json:"tts_max_concurrency"`
	RenderMaxConcurrency  int `json:"render_max_concurrency"`
	TaskMaxQueuedPerUser  int `json:"task_max_queued_per_user"`
	TaskMaxRunningPerUser int `json:"task_max_running_per_user"`
	VLMTimeoutSeconds     int `json:"vlm_timeout_seconds"`
	VLMMaxRetries         int `json:"vlm_max_retries"`
}

func GetOpenAICompatibleSettings(service *SystemConfigService) (OpenAICompatibleSettings, error) {
	if service == nil {
		return OpenAICompatibleSettings{}, fmt.Errorf("system config service is nil")
	}

	settings := OpenAICompatibleSettings{
		Provider: openAICompatibleProvider,
	}

	if config, err := service.Get(openAIBaseURLKey); err == nil {
		settings.BaseURL = configStringValue(config.Value)
	}
	if config, err := service.Get(openAIAPIKeyKey); err == nil {
		settings.APIKeyConfigured = strings.TrimSpace(configStringValue(config.Value)) != ""
	}
	if config, err := service.Get("llm.model"); err == nil {
		settings.LLMModel = configStringValue(config.Value)
	}
	if config, err := service.Get("vlm.model"); err == nil {
		settings.VLMModel = configStringValue(config.Value)
	}
	if config, err := service.Get("embedding.model"); err == nil {
		settings.EmbeddingModel = configStringValue(config.Value)
	}

	return settings, nil
}

func UpdateOpenAICompatibleSettings(service *SystemConfigService, input OpenAICompatibleSettingsUpdate) (OpenAICompatibleSettings, error) {
	if service == nil {
		return OpenAICompatibleSettings{}, fmt.Errorf("system config service is nil")
	}

	current, err := GetOpenAICompatibleSettings(service)
	if err != nil {
		return OpenAICompatibleSettings{}, err
	}

	baseURL := strings.TrimSpace(input.BaseURL)
	if baseURL == "" {
		baseURL = current.BaseURL
	}
	if baseURL == "" {
		return OpenAICompatibleSettings{}, fmt.Errorf("base_url is required")
	}
	if _, err := validateBaseURL(baseURL); err != nil {
		return OpenAICompatibleSettings{}, err
	}
	llmModel := strings.TrimSpace(input.LLMModel)
	if llmModel == "" {
		return OpenAICompatibleSettings{}, fmt.Errorf("llm_model is required")
	}
	vlmModel := strings.TrimSpace(input.VLMModel)
	if vlmModel == "" {
		return OpenAICompatibleSettings{}, fmt.Errorf("vlm_model is required")
	}
	embeddingModel := strings.TrimSpace(input.EmbeddingModel)
	if embeddingModel == "" {
		return OpenAICompatibleSettings{}, fmt.Errorf("embedding_model is required")
	}

	if _, err := service.Upsert(SystemConfig{
		Key:         openAIBaseURLKey,
		Value:       baseURL,
		Type:        "string",
		Description: "OpenAI-compatible API base URL",
	}); err != nil {
		return OpenAICompatibleSettings{}, err
	}

	if strings.TrimSpace(input.APIKey) != "" {
		if _, err := service.Upsert(SystemConfig{
			Key:         openAIAPIKeyKey,
			Value:       strings.TrimSpace(input.APIKey),
			Type:        "string",
			IsSecret:    true,
			Description: "OpenAI-compatible API key",
		}); err != nil {
			return OpenAICompatibleSettings{}, err
		}
	}

	if _, err := service.Upsert(SystemConfig{
		Key:         "llm.provider",
		Value:       openAICompatibleProvider,
		Type:        "string",
		Description: "Default LLM provider",
	}); err != nil {
		return OpenAICompatibleSettings{}, err
	}
	if _, err := service.Upsert(SystemConfig{
		Key:         "vlm.provider",
		Value:       openAICompatibleProvider,
		Type:        "string",
		Description: "Default VLM provider",
	}); err != nil {
		return OpenAICompatibleSettings{}, err
	}
	if _, err := service.Upsert(SystemConfig{
		Key:         "embedding.provider",
		Value:       openAICompatibleProvider,
		Type:        "string",
		Description: "Default embedding provider",
	}); err != nil {
		return OpenAICompatibleSettings{}, err
	}
	if _, err := service.Upsert(SystemConfig{
		Key:         "llm.model",
		Value:       llmModel,
		Type:        "string",
		Description: "Default LLM model",
	}); err != nil {
		return OpenAICompatibleSettings{}, err
	}
	if _, err := service.Upsert(SystemConfig{
		Key:         "vlm.model",
		Value:       vlmModel,
		Type:        "string",
		Description: "Default VLM model",
	}); err != nil {
		return OpenAICompatibleSettings{}, err
	}
	if _, err := service.Upsert(SystemConfig{
		Key:         "embedding.model",
		Value:       embeddingModel,
		Type:        "string",
		Description: "Default embedding model",
	}); err != nil {
		return OpenAICompatibleSettings{}, err
	}

	return GetOpenAICompatibleSettings(service)
}

func ResolveOpenAICompatibleAccess(service *SystemConfigService, input OpenAICompatibleResolveInput) (string, string, error) {
	if service == nil {
		return "", "", fmt.Errorf("system config service is nil")
	}

	settings, err := GetOpenAICompatibleSettings(service)
	if err != nil {
		return "", "", err
	}

	baseURL := strings.TrimSpace(input.BaseURL)
	if baseURL == "" {
		baseURL = settings.BaseURL
	}
	if baseURL == "" {
		return "", "", fmt.Errorf("base_url is required")
	}
	parsedURL, err := validateBaseURL(baseURL)
	if err != nil {
		return "", "", err
	}

	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		config, err := service.Get(openAIAPIKeyKey)
		if err == nil {
			apiKey = strings.TrimSpace(configStringValue(config.Value))
		}
	}

	return strings.TrimRight(parsedURL.String(), "/"), apiKey, nil
}

func FetchOpenAICompatibleModels(ctx context.Context, service *SystemConfigService, input OpenAICompatibleResolveInput) (ModelDiscoveryResult, error) {
	baseURL, apiKey, err := ResolveOpenAICompatibleAccess(service, input)
	if err != nil {
		return ModelDiscoveryResult{}, err
	}

	return FetchOpenAICompatibleModelsWithAccess(ctx, baseURL, apiKey)
}

func FetchOpenAICompatibleModelsWithAccess(ctx context.Context, baseURL string, apiKey string) (ModelDiscoveryResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, joinOpenAICompatibleURL(baseURL, "/v1/models"), nil)
	if err != nil {
		return ModelDiscoveryResult{}, err
	}
	request.Header.Set("Accept", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return ModelDiscoveryResult{}, fmt.Errorf("failed to request model list: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ModelDiscoveryResult{}, fmt.Errorf("model endpoint returned status %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return ModelDiscoveryResult{}, fmt.Errorf("failed to read model list: %w", err)
	}
	if strings.HasPrefix(strings.TrimSpace(string(body)), "<") {
		return ModelDiscoveryResult{}, fmt.Errorf("model endpoint returned HTML, please check whether Base URL is an OpenAI-compatible API address")
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ModelDiscoveryResult{}, fmt.Errorf("failed to decode model list: %w", err)
	}

	modelSet := make(map[string]struct{}, len(payload.Data))
	models := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, exists := modelSet[id]; exists {
			continue
		}
		modelSet[id] = struct{}{}
		models = append(models, id)
	}
	slices.Sort(models)

	result := ModelDiscoveryResult{Models: make([]ModelOption, 0, len(models))}
	for _, modelID := range models {
		result.Models = append(result.Models, ModelOption{ID: modelID})
	}
	return result, nil
}

func FetchModelsFromProvider(ctx context.Context, providerService *ModelProviderService, providerID string) (ModelDiscoveryResult, error) {
	access, err := providerService.GetAccess(ctx, providerID)
	if err != nil {
		return ModelDiscoveryResult{}, err
	}
	if !access.Enabled {
		return ModelDiscoveryResult{}, fmt.Errorf("model provider is disabled")
	}
	if access.ProviderType != ModelProviderTypeOpenAICompatible {
		return ModelDiscoveryResult{}, fmt.Errorf("provider_type only supports openai_compatible")
	}
	return FetchOpenAICompatibleModelsWithAccess(ctx, access.BaseURL, access.APIKey)
}

func TestModelProviderConnection(ctx context.Context, providerService *ModelProviderService, providerID string) (int, error) {
	result, err := FetchModelsFromProvider(ctx, providerService, providerID)
	if err != nil {
		return 0, err
	}
	return len(result.Models), nil
}

func EnsureLegacyOpenAICompatibleProvider(ctx context.Context, systemConfigService *SystemConfigService, providerService *ModelProviderService) error {
	if systemConfigService == nil || providerService == nil {
		return nil
	}
	providers, err := providerService.List(ctx)
	if err != nil {
		return err
	}
	if len(providers) > 0 {
		return nil
	}
	settings, err := GetOpenAICompatibleSettings(systemConfigService)
	if err != nil {
		return err
	}
	if strings.TrimSpace(settings.BaseURL) == "" {
		return nil
	}
	apiKey := ""
	if config, err := systemConfigService.Get(openAIAPIKeyKey); err == nil {
		apiKey = configStringValue(config.Value)
	}
	provider, err := providerService.Create(ctx, ModelProviderInput{
		Name:         "默认 OpenAI Compatible",
		ProviderType: ModelProviderTypeOpenAICompatible,
		BaseURL:      settings.BaseURL,
		APIKey:       apiKey,
		Enabled:      true,
	})
	if err != nil {
		return err
	}
	for _, item := range []struct {
		capability string
		model      string
	}{
		{capability: "llm", model: settings.LLMModel},
		{capability: "vlm", model: settings.VLMModel},
		{capability: "embedding", model: settings.EmbeddingModel},
	} {
		if strings.TrimSpace(item.model) == "" {
			continue
		}
		if _, err := systemConfigService.Upsert(SystemConfig{
			Key:         item.capability + ".provider_id",
			Value:       provider.ID,
			Type:        "string",
			Description: "Default " + strings.ToUpper(item.capability) + " model provider ID",
		}); err != nil {
			return err
		}
	}
	return nil
}

func GetModelCapabilitySettings(service *SystemConfigService) (ModelCapabilitySettings, error) {
	if service == nil {
		return ModelCapabilitySettings{}, fmt.Errorf("system config service is nil")
	}
	return ModelCapabilitySettings{
		LLM:       getModelCapabilitySetting(service, "llm"),
		VLM:       getModelCapabilitySetting(service, "vlm"),
		Embedding: getModelCapabilitySetting(service, "embedding"),
	}, nil
}

func UpdateModelCapabilitySettings(service *SystemConfigService, input ModelCapabilitySettings) (ModelCapabilitySettings, error) {
	if service == nil {
		return ModelCapabilitySettings{}, fmt.Errorf("system config service is nil")
	}
	for _, setting := range []ModelCapabilitySetting{
		normalizeCapabilityInput("llm", input.LLM),
		normalizeCapabilityInput("vlm", input.VLM),
		normalizeCapabilityInput("embedding", input.Embedding),
	} {
		if strings.TrimSpace(setting.ProviderID) == "" {
			return ModelCapabilitySettings{}, fmt.Errorf("%s.provider_id is required", setting.Capability)
		}
		if strings.TrimSpace(setting.Model) == "" {
			return ModelCapabilitySettings{}, fmt.Errorf("%s.model is required", setting.Capability)
		}
		if setting.Capability == "embedding" && setting.Dimension <= 0 {
			return ModelCapabilitySettings{}, fmt.Errorf("embedding.dimension must be >= 1")
		}
		if _, err := service.Upsert(SystemConfig{
			Key:         setting.Capability + ".provider_id",
			Value:       strings.TrimSpace(setting.ProviderID),
			Type:        "string",
			Description: "Default " + strings.ToUpper(setting.Capability) + " model provider ID",
		}); err != nil {
			return ModelCapabilitySettings{}, err
		}
		if _, err := service.Upsert(SystemConfig{
			Key:         setting.Capability + ".provider",
			Value:       ModelProviderTypeOpenAICompatible,
			Type:        "string",
			Description: "Default " + strings.ToUpper(setting.Capability) + " provider type",
		}); err != nil {
			return ModelCapabilitySettings{}, err
		}
		if _, err := service.Upsert(SystemConfig{
			Key:         setting.Capability + ".model",
			Value:       strings.TrimSpace(setting.Model),
			Type:        "string",
			Description: "Default " + strings.ToUpper(setting.Capability) + " model",
		}); err != nil {
			return ModelCapabilitySettings{}, err
		}
		if setting.Capability == "embedding" {
			if _, err := service.Upsert(SystemConfig{
				Key:         "embedding.dimension",
				Value:       setting.Dimension,
				Type:        "number",
				Description: "Default embedding vector dimension",
			}); err != nil {
				return ModelCapabilitySettings{}, err
			}
		}
	}
	return GetModelCapabilitySettings(service)
}

func getModelCapabilitySetting(service *SystemConfigService, capability string) ModelCapabilitySetting {
	setting := ModelCapabilitySetting{Capability: capability}
	if config, err := service.Get(capability + ".provider_id"); err == nil {
		setting.ProviderID = configStringValue(config.Value)
	}
	if config, err := service.Get(capability + ".model"); err == nil {
		setting.Model = configStringValue(config.Value)
	}
	if capability == "embedding" {
		setting.Dimension = 1024
		if config, err := service.Get("embedding.dimension"); err == nil {
			if value := configIntValue(config.Value); value > 0 {
				setting.Dimension = value
			}
		}
	}
	return setting
}

func normalizeCapabilityInput(capability string, input ModelCapabilitySetting) ModelCapabilitySetting {
	return ModelCapabilitySetting{
		Capability: capability,
		ProviderID: strings.TrimSpace(input.ProviderID),
		Model:      strings.TrimSpace(input.Model),
		Dimension:  input.Dimension,
	}
}

func joinOpenAICompatibleURL(base string, suffix string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	suffix = "/" + strings.TrimLeft(suffix, "/")
	if strings.HasSuffix(base, suffix) {
		return base
	}
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(suffix, "/v1/") {
		return base + strings.TrimPrefix(suffix, "/v1")
	}
	return base + suffix
}

func GetRuntimeSettings(service *SystemConfigService) (RuntimeSettings, error) {
	if service == nil {
		return RuntimeSettings{}, fmt.Errorf("system config service is nil")
	}

	return RuntimeSettings{
		LLMMaxConcurrency:     getConfigIntOrDefault(service, "llm.max_concurrency", 2),
		VLMMaxConcurrency:     getConfigIntOrDefault(service, "vlm.max_concurrency", 2),
		ASRMaxConcurrency:     getConfigIntOrDefault(service, "asr.max_concurrency", 2),
		TTSMaxConcurrency:     getConfigIntOrDefault(service, "tts.max_concurrency", 2),
		RenderMaxConcurrency:  getConfigIntOrDefault(service, "render.max_concurrency", 1),
		TaskMaxQueuedPerUser:  getConfigIntOrDefault(service, "task.max_queued_per_user", 20),
		TaskMaxRunningPerUser: getConfigIntOrDefault(service, "task.max_running_per_user", 2),
		VLMTimeoutSeconds:     getConfigIntOrDefault(service, "vlm.timeout_seconds", 120),
		VLMMaxRetries:         getConfigIntOrDefault(service, "vlm.max_retries", 2),
	}, nil
}

func UpdateRuntimeSettings(service *SystemConfigService, input RuntimeSettings) (RuntimeSettings, error) {
	if service == nil {
		return RuntimeSettings{}, fmt.Errorf("system config service is nil")
	}
	if err := validateRuntimeSettings(input); err != nil {
		return RuntimeSettings{}, err
	}

	configs := []SystemConfig{
		{Key: "llm.max_concurrency", Value: input.LLMMaxConcurrency, Type: "number", Description: "Global LLM concurrency"},
		{Key: "vlm.max_concurrency", Value: input.VLMMaxConcurrency, Type: "number", Description: "Global VLM concurrency"},
		{Key: "asr.max_concurrency", Value: input.ASRMaxConcurrency, Type: "number", Description: "Global ASR concurrency"},
		{Key: "tts.max_concurrency", Value: input.TTSMaxConcurrency, Type: "number", Description: "Global TTS concurrency"},
		{Key: "render.max_concurrency", Value: input.RenderMaxConcurrency, Type: "number", Description: "Global render concurrency"},
		{Key: "task.max_queued_per_user", Value: input.TaskMaxQueuedPerUser, Type: "number", Description: "Max queued tasks per user"},
		{Key: "task.max_running_per_user", Value: input.TaskMaxRunningPerUser, Type: "number", Description: "Max running tasks per user"},
		{Key: "vlm.timeout_seconds", Value: input.VLMTimeoutSeconds, Type: "number", Description: "VLM request timeout seconds"},
		{Key: "vlm.max_retries", Value: input.VLMMaxRetries, Type: "number", Description: "VLM request max retries"},
	}

	for _, config := range configs {
		if _, err := service.Upsert(config); err != nil {
			return RuntimeSettings{}, err
		}
	}

	return GetRuntimeSettings(service)
}

func getConfigIntOrDefault(service *SystemConfigService, key string, fallback int) int {
	config, err := service.Get(key)
	if err != nil {
		return fallback
	}
	value := configIntValue(config.Value)
	if value == 0 && fallback != 0 {
		return fallback
	}
	return value
}

func validateBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("base_url must start with http:// or https://")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("base_url host is required")
	}
	return parsed, nil
}

func validateRuntimeSettings(input RuntimeSettings) error {
	if input.LLMMaxConcurrency < 1 {
		return fmt.Errorf("llm_max_concurrency must be >= 1")
	}
	if input.VLMMaxConcurrency < 1 {
		return fmt.Errorf("vlm_max_concurrency must be >= 1")
	}
	if input.ASRMaxConcurrency < 1 {
		return fmt.Errorf("asr_max_concurrency must be >= 1")
	}
	if input.TTSMaxConcurrency < 1 {
		return fmt.Errorf("tts_max_concurrency must be >= 1")
	}
	if input.RenderMaxConcurrency < 1 {
		return fmt.Errorf("render_max_concurrency must be >= 1")
	}
	if input.TaskMaxQueuedPerUser < 1 {
		return fmt.Errorf("task_max_queued_per_user must be >= 1")
	}
	if input.TaskMaxRunningPerUser < 1 {
		return fmt.Errorf("task_max_running_per_user must be >= 1")
	}
	if input.VLMTimeoutSeconds < 1 {
		return fmt.Errorf("vlm_timeout_seconds must be >= 1")
	}
	if input.VLMMaxRetries < 0 {
		return fmt.Errorf("vlm_max_retries must be >= 0")
	}
	return nil
}
