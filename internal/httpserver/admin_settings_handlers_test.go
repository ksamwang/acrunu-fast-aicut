package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func adminAuthHeader() string {
	return "Bearer " + makeDevToken(auth.User{
		ID:          "admin-1",
		Username:    "admin",
		DisplayName: "Admin",
		Role:        auth.RoleAdmin,
	})
}

func TestHandleUpdateAndGetOpenAICompatibleSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	systemConfigService := services.NewSystemConfigService()
	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		SystemConfigService: systemConfigService,
	})

	body := bytes.NewBufferString(`{"base_url":"https://example.com/v1","api_key":"secret-key","llm_model":"gpt-4.1","vlm_model":"gpt-4o-mini","embedding_model":"text-embedding-v3"}`)
	updateReq := httptest.NewRequest(http.MethodPut, "/api/admin/model-access/openai-compatible", body)
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", adminAuthHeader())
	updateRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(updateRecorder, updateReq)

	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", updateRecorder.Code, updateRecorder.Body.String())
	}

	var updateResp struct {
		Data services.OpenAICompatibleSettings `json:"data"`
	}
	if err := json.Unmarshal(updateRecorder.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if updateResp.Data.BaseURL != "https://example.com/v1" {
		t.Fatalf("expected base url, got %#v", updateResp.Data)
	}
	if !updateResp.Data.APIKeyConfigured {
		t.Fatalf("expected api key configured")
	}
	if updateResp.Data.EmbeddingModel != "text-embedding-v3" {
		t.Fatalf("expected embedding model configured, got %#v", updateResp.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/admin/model-access/openai-compatible", nil)
	getReq.Header.Set("Authorization", adminAuthHeader())
	getRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(getRecorder, getReq)

	if getRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", getRecorder.Code, getRecorder.Body.String())
	}

	var getResp struct {
		Data services.OpenAICompatibleSettings `json:"data"`
	}
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get response failed: %v", err)
	}
	if getResp.Data.APIKeyConfigured != true {
		t.Fatalf("expected api key configured in get response")
	}
	if getResp.Data.EmbeddingModel != "text-embedding-v3" {
		t.Fatalf("expected embedding model in get response, got %#v", getResp.Data)
	}
}

func TestHandleOpenAICompatibleConnectionAndModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

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
			},
		})
	}))
	defer modelServer.Close()

	systemConfigService := services.NewSystemConfigService()
	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		SystemConfigService: systemConfigService,
	})

	payload := []byte(`{"base_url":"` + modelServer.URL + `/v1","api_key":"secret-key"}`)

	testReq := httptest.NewRequest(http.MethodPost, "/api/admin/model-access/openai-compatible/test", bytes.NewReader(payload))
	testReq.Header.Set("Content-Type", "application/json")
	testReq.Header.Set("Authorization", adminAuthHeader())
	testRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(testRecorder, testReq)

	if testRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", testRecorder.Code, testRecorder.Body.String())
	}

	modelsReq := httptest.NewRequest(http.MethodPost, "/api/admin/model-access/openai-compatible/models", bytes.NewReader(payload))
	modelsReq.Header.Set("Content-Type", "application/json")
	modelsReq.Header.Set("Authorization", adminAuthHeader())
	modelsRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(modelsRecorder, modelsReq)

	if modelsRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", modelsRecorder.Code, modelsRecorder.Body.String())
	}

	var modelsResp struct {
		Data services.ModelDiscoveryResult `json:"data"`
	}
	if err := json.Unmarshal(modelsRecorder.Body.Bytes(), &modelsResp); err != nil {
		t.Fatalf("decode models response failed: %v", err)
	}
	if len(modelsResp.Data.Models) != 2 {
		t.Fatalf("expected 2 models, got %#v", modelsResp.Data.Models)
	}
}

func TestHandleGetAndUpdateRuntimeSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	systemConfigService := services.NewSystemConfigService()
	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		SystemConfigService: systemConfigService,
	})

	updateBody := bytes.NewBufferString(`{
		"llm_max_concurrency": 5,
		"vlm_max_concurrency": 6,
		"asr_max_concurrency": 2,
		"tts_max_concurrency": 2,
		"render_max_concurrency": 1,
		"task_max_queued_per_user": 9,
		"task_max_running_per_user": 3,
		"vlm_timeout_seconds": 150,
		"vlm_max_retries": 4
	}`)
	updateReq := httptest.NewRequest(http.MethodPut, "/api/admin/runtime-settings", updateBody)
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", adminAuthHeader())
	updateRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(updateRecorder, updateReq)

	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", updateRecorder.Code, updateRecorder.Body.String())
	}

	var updateResp struct {
		Data services.RuntimeSettings `json:"data"`
	}
	if err := json.Unmarshal(updateRecorder.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("decode update response failed: %v", err)
	}
	if updateResp.Data.LLMMaxConcurrency != 5 || updateResp.Data.VLMTimeoutSeconds != 150 {
		t.Fatalf("unexpected runtime settings %#v", updateResp.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/admin/runtime-settings", nil)
	getReq.Header.Set("Authorization", adminAuthHeader())
	getRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(getRecorder, getReq)

	if getRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", getRecorder.Code, getRecorder.Body.String())
	}

	var getResp struct {
		Data services.RuntimeSettings `json:"data"`
	}
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get response failed: %v", err)
	}
	if getResp.Data.TaskMaxQueuedPerUser != 9 {
		t.Fatalf("expected queued per user 9, got %#v", getResp.Data)
	}
}

func TestHandleUpdateRuntimeSettingsValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	systemConfigService := services.NewSystemConfigService()
	server := New(Options{
		Config:              config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		SystemConfigService: systemConfigService,
	})

	updateBody := bytes.NewBufferString(`{
		"llm_max_concurrency": 0,
		"vlm_max_concurrency": 6,
		"asr_max_concurrency": 2,
		"tts_max_concurrency": 2,
		"render_max_concurrency": 1,
		"task_max_queued_per_user": 9,
		"task_max_running_per_user": 3,
		"vlm_timeout_seconds": 150,
		"vlm_max_retries": 4
	}`)
	updateReq := httptest.NewRequest(http.MethodPut, "/api/admin/runtime-settings", updateBody)
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", adminAuthHeader())
	updateRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(updateRecorder, updateReq)

	if updateRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body=%s", updateRecorder.Code, updateRecorder.Body.String())
	}
}
