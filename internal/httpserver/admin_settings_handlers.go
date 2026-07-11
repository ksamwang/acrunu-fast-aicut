package httpserver

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type updateOpenAICompatibleSettingsRequest struct {
	BaseURL        string `json:"base_url"`
	APIKey         string `json:"api_key"`
	LLMModel       string `json:"llm_model"`
	VLMModel       string `json:"vlm_model"`
	EmbeddingModel string `json:"embedding_model"`
}

type resolveOpenAICompatibleRequest struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

type updateRuntimeSettingsRequest struct {
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

type updateModelCapabilitySettingsRequest struct {
	LLM       services.ModelCapabilitySetting `json:"llm"`
	VLM       services.ModelCapabilitySetting `json:"vlm"`
	Embedding services.ModelCapabilitySetting `json:"embedding"`
}

func (s *Server) handleGetOpenAICompatibleSettings(c *gin.Context) {
	settings, err := services.GetOpenAICompatibleSettings(s.systemConfigService)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to load model access settings")
		return
	}
	OK(c, settings)
}

func (s *Server) handleUpdateOpenAICompatibleSettings(c *gin.Context) {
	var req updateOpenAICompatibleSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid model access payload")
		return
	}

	settings, err := services.UpdateOpenAICompatibleSettings(s.systemConfigService, services.OpenAICompatibleSettingsUpdate{
		BaseURL:        req.BaseURL,
		APIKey:         req.APIKey,
		LLMModel:       req.LLMModel,
		VLMModel:       req.VLMModel,
		EmbeddingModel: req.EmbeddingModel,
	})
	if err != nil {
		status := http.StatusInternalServerError
		code := "internal_error"
		if isAdminSettingsBadRequest(err) {
			status = http.StatusBadRequest
			code = "bad_request"
		}
		Fail(c, status, code, err.Error())
		return
	}

	OK(c, settings)
}

func (s *Server) handleTestOpenAICompatibleConnection(c *gin.Context) {
	var req resolveOpenAICompatibleRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid connection test payload")
		return
	}

	models, err := services.FetchOpenAICompatibleModels(c.Request.Context(), s.systemConfigService, services.OpenAICompatibleResolveInput{
		BaseURL: req.BaseURL,
		APIKey:  req.APIKey,
	})
	if err != nil {
		status := http.StatusBadGateway
		code := "provider_error"
		if isAdminSettingsBadRequest(err) {
			status = http.StatusBadRequest
			code = "bad_request"
		}
		Fail(c, status, code, err.Error())
		return
	}

	OK(c, gin.H{
		"reachable":   true,
		"model_count": len(models.Models),
	})
}

func (s *Server) handleListOpenAICompatibleModels(c *gin.Context) {
	var req resolveOpenAICompatibleRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid model discovery payload")
		return
	}

	models, err := services.FetchOpenAICompatibleModels(c.Request.Context(), s.systemConfigService, services.OpenAICompatibleResolveInput{
		BaseURL: req.BaseURL,
		APIKey:  req.APIKey,
	})
	if err != nil {
		status := http.StatusBadGateway
		code := "provider_error"
		if isAdminSettingsBadRequest(err) {
			status = http.StatusBadRequest
			code = "bad_request"
		}
		Fail(c, status, code, err.Error())
		return
	}

	OK(c, models)
}

func (s *Server) handleListModelProviders(c *gin.Context) {
	if err := services.EnsureLegacyOpenAICompatibleProvider(c.Request.Context(), s.systemConfigService, s.modelProviderService); err != nil {
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to migrate legacy model provider settings")
		return
	}
	providers, err := s.modelProviderService.List(c.Request.Context())
	if err != nil {
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to list model providers")
		return
	}
	OK(c, providers)
}

func (s *Server) handleCreateModelProvider(c *gin.Context) {
	var req services.ModelProviderInput
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid model provider payload")
		return
	}
	provider, err := s.modelProviderService.Create(c.Request.Context(), req)
	if err != nil {
		Fail(c, statusForModelProviderError(err), codeForModelProviderError(err), err.Error())
		return
	}
	OK(c, provider)
}

func (s *Server) handleUpdateModelProvider(c *gin.Context) {
	var req services.ModelProviderInput
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid model provider payload")
		return
	}
	provider, err := s.modelProviderService.Update(c.Request.Context(), c.Param("providerID"), req)
	if err != nil {
		Fail(c, statusForModelProviderError(err), codeForModelProviderError(err), err.Error())
		return
	}
	OK(c, provider)
}

func (s *Server) handleDeleteModelProvider(c *gin.Context) {
	if err := s.modelProviderService.Delete(c.Request.Context(), c.Param("providerID")); err != nil {
		Fail(c, statusForModelProviderError(err), codeForModelProviderError(err), err.Error())
		return
	}
	OK(c, gin.H{"deleted": true})
}

func (s *Server) handleTestModelProvider(c *gin.Context) {
	count, err := services.TestModelProviderConnection(c.Request.Context(), s.modelProviderService, c.Param("providerID"))
	if err != nil {
		Fail(c, statusForModelProviderError(err), codeForModelProviderError(err), err.Error())
		return
	}
	OK(c, gin.H{"reachable": true, "model_count": count})
}

func (s *Server) handleListModelProviderModels(c *gin.Context) {
	models, err := services.FetchModelsFromProvider(c.Request.Context(), s.modelProviderService, c.Param("providerID"))
	if err != nil {
		Fail(c, statusForModelProviderError(err), codeForModelProviderError(err), err.Error())
		return
	}
	OK(c, models)
}

func (s *Server) handleGetModelCapabilitySettings(c *gin.Context) {
	if err := services.EnsureLegacyOpenAICompatibleProvider(c.Request.Context(), s.systemConfigService, s.modelProviderService); err != nil {
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to migrate legacy model provider settings")
		return
	}
	settings, err := services.GetModelCapabilitySettings(s.systemConfigService)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to load model settings")
		return
	}
	OK(c, settings)
}

func (s *Server) handleUpdateModelCapabilitySettings(c *gin.Context) {
	var req updateModelCapabilitySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid model settings payload")
		return
	}
	settings, err := services.UpdateModelCapabilitySettings(s.systemConfigService, services.ModelCapabilitySettings{
		LLM:       req.LLM,
		VLM:       req.VLM,
		Embedding: req.Embedding,
	})
	if err != nil {
		Fail(c, statusForModelProviderError(err), codeForModelProviderError(err), err.Error())
		return
	}
	OK(c, settings)
}

func (s *Server) handleGetRuntimeSettings(c *gin.Context) {
	settings, err := services.GetRuntimeSettings(s.systemConfigService)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to load runtime settings")
		return
	}
	OK(c, settings)
}

func (s *Server) handleUpdateRuntimeSettings(c *gin.Context) {
	var req updateRuntimeSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid runtime settings payload")
		return
	}

	settings, err := services.UpdateRuntimeSettings(s.systemConfigService, services.RuntimeSettings{
		LLMMaxConcurrency:     req.LLMMaxConcurrency,
		VLMMaxConcurrency:     req.VLMMaxConcurrency,
		ASRMaxConcurrency:     req.ASRMaxConcurrency,
		TTSMaxConcurrency:     req.TTSMaxConcurrency,
		RenderMaxConcurrency:  req.RenderMaxConcurrency,
		TaskMaxQueuedPerUser:  req.TaskMaxQueuedPerUser,
		TaskMaxRunningPerUser: req.TaskMaxRunningPerUser,
		VLMTimeoutSeconds:     req.VLMTimeoutSeconds,
		VLMMaxRetries:         req.VLMMaxRetries,
	})
	if err != nil {
		status := http.StatusInternalServerError
		code := "internal_error"
		if isAdminSettingsBadRequest(err) {
			status = http.StatusBadRequest
			code = "bad_request"
		}
		Fail(c, status, code, err.Error())
		return
	}
	OK(c, settings)
}

func isAdminSettingsBadRequest(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return containsAny(message,
		"base_url",
		"llm_model",
		"vlm_model",
		"embedding_model",
		"llm_max_concurrency",
		"vlm_max_concurrency",
		"asr_max_concurrency",
		"tts_max_concurrency",
		"render_max_concurrency",
		"task_max_queued_per_user",
		"task_max_running_per_user",
		"vlm_timeout_seconds",
		"vlm_max_retries",
		"failed to decode model list",
		"model endpoint returned HTML",
		"model endpoint returned status",
		"failed to request model list",
		"failed to read model list",
	)
}

func statusForModelProviderError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, services.ErrModelProviderNotFound) {
		return http.StatusNotFound
	}
	if isAdminSettingsBadRequest(err) || containsAny(err.Error(), "provider name", "provider_type", "provider_id", ".model", "disabled") {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func codeForModelProviderError(err error) string {
	if errors.Is(err, services.ErrModelProviderNotFound) {
		return "not_found"
	}
	if isAdminSettingsBadRequest(err) || containsAny(err.Error(), "provider name", "provider_type", "provider_id", ".model", "disabled") {
		return "bad_request"
	}
	return "provider_error"
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
