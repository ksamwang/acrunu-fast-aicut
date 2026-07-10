package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateAndGetOpenAICompatibleSettings(t *testing.T) {
	service := NewSystemConfigService()

	updated, err := UpdateOpenAICompatibleSettings(service, OpenAICompatibleSettingsUpdate{
		BaseURL:  "https://example.com/v1",
		APIKey:   "secret-key",
		LLMModel: "gpt-4.1",
		VLMModel: "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("update settings failed: %v", err)
	}

	if updated.BaseURL != "https://example.com/v1" {
		t.Fatalf("expected base url persisted, got %q", updated.BaseURL)
	}
	if !updated.APIKeyConfigured {
		t.Fatalf("expected api key configured")
	}
	if updated.LLMModel != "gpt-4.1" {
		t.Fatalf("expected llm model persisted, got %q", updated.LLMModel)
	}
	if updated.VLMModel != "gpt-4o-mini" {
		t.Fatalf("expected vlm model persisted, got %q", updated.VLMModel)
	}

	apiKeyConfig, err := service.Get(openAIAPIKeyKey)
	if err != nil {
		t.Fatalf("expected api key config, got %v", err)
	}
	if apiKeyConfig.IsSecret != true {
		t.Fatalf("expected api key config to be secret")
	}
}

func TestUpdateOpenAICompatibleSettingsRequiresModels(t *testing.T) {
	service := NewSystemConfigService()

	_, err := UpdateOpenAICompatibleSettings(service, OpenAICompatibleSettingsUpdate{
		BaseURL: "https://example.com/v1",
		APIKey:  "secret-key",
	})
	if err == nil {
		t.Fatalf("expected missing model validation error")
	}
}

func TestFetchOpenAICompatibleModels(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-key" {
			t.Fatalf("unexpected auth header %q", got)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4.1"},
				{"id": "gpt-4o-mini"},
				{"id": "gpt-4.1"},
			},
		})
	}))
	defer modelServer.Close()

	service := NewSystemConfigService()
	if _, err := UpdateOpenAICompatibleSettings(service, OpenAICompatibleSettingsUpdate{
		BaseURL:  modelServer.URL + "/v1",
		APIKey:   "secret-key",
		LLMModel: "gpt-4.1",
		VLMModel: "gpt-4o-mini",
	}); err != nil {
		t.Fatalf("seed settings failed: %v", err)
	}

	result, err := FetchOpenAICompatibleModels(t.Context(), service, OpenAICompatibleResolveInput{})
	if err != nil {
		t.Fatalf("fetch models failed: %v", err)
	}

	if len(result.Models) != 2 {
		t.Fatalf("expected 2 deduplicated models, got %d", len(result.Models))
	}
	if result.Models[0].ID != "gpt-4.1" || result.Models[1].ID != "gpt-4o-mini" {
		t.Fatalf("unexpected models %#v", result.Models)
	}
}

func TestFetchOpenAICompatibleModelsAppendsV1ForRootBaseURL(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "deepseek-chat"},
			},
		})
	}))
	defer modelServer.Close()

	service := NewSystemConfigService()
	result, err := FetchOpenAICompatibleModels(t.Context(), service, OpenAICompatibleResolveInput{
		BaseURL: modelServer.URL,
		APIKey:  "secret-key",
	})
	if err != nil {
		t.Fatalf("fetch models failed: %v", err)
	}
	if len(result.Models) != 1 || result.Models[0].ID != "deepseek-chat" {
		t.Fatalf("unexpected models %#v", result.Models)
	}
}

func TestFetchOpenAICompatibleModelsReturnsReadableHTMLResponseError(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body>login</body></html>"))
	}))
	defer modelServer.Close()

	service := NewSystemConfigService()
	_, err := FetchOpenAICompatibleModels(t.Context(), service, OpenAICompatibleResolveInput{
		BaseURL: modelServer.URL,
		APIKey:  "secret-key",
	})
	if err == nil {
		t.Fatalf("expected html response error")
	}
	if !strings.Contains(err.Error(), "model endpoint returned HTML") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestGetAndUpdateRuntimeSettings(t *testing.T) {
	service := NewSystemConfigService()

	updated, err := UpdateRuntimeSettings(service, RuntimeSettings{
		LLMMaxConcurrency:     3,
		VLMMaxConcurrency:     4,
		ASRMaxConcurrency:     2,
		TTSMaxConcurrency:     2,
		RenderMaxConcurrency:  1,
		TaskMaxQueuedPerUser:  12,
		TaskMaxRunningPerUser: 3,
		VLMTimeoutSeconds:     180,
		VLMMaxRetries:         5,
	})
	if err != nil {
		t.Fatalf("update runtime settings failed: %v", err)
	}

	if updated.VLMTimeoutSeconds != 180 {
		t.Fatalf("expected timeout 180, got %d", updated.VLMTimeoutSeconds)
	}
	if updated.TaskMaxQueuedPerUser != 12 {
		t.Fatalf("expected queued limit 12, got %d", updated.TaskMaxQueuedPerUser)
	}

	reloaded, err := GetRuntimeSettings(service)
	if err != nil {
		t.Fatalf("get runtime settings failed: %v", err)
	}
	if reloaded.LLMMaxConcurrency != 3 || reloaded.VLMMaxRetries != 5 {
		t.Fatalf("unexpected runtime settings %#v", reloaded)
	}
}

func TestUpdateRuntimeSettingsValidation(t *testing.T) {
	service := NewSystemConfigService()

	_, err := UpdateRuntimeSettings(service, RuntimeSettings{
		LLMMaxConcurrency:     0,
		VLMMaxConcurrency:     4,
		ASRMaxConcurrency:     2,
		TTSMaxConcurrency:     2,
		RenderMaxConcurrency:  1,
		TaskMaxQueuedPerUser:  12,
		TaskMaxRunningPerUser: 3,
		VLMTimeoutSeconds:     180,
		VLMMaxRetries:         5,
	})
	if err == nil {
		t.Fatalf("expected validation error for invalid runtime settings")
	}
}
