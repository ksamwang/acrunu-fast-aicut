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
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	LLMModel string `json:"llm_model"`
	VLMModel string `json:"vlm_model"`
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
		BaseURL:  req.BaseURL,
		APIKey:   req.APIKey,
		LLMModel: req.LLMModel,
		VLMModel: req.VLMModel,
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

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
