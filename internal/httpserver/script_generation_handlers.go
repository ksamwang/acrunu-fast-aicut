package httpserver

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type generateWorkbenchScriptsRequest struct {
	ProductID             string   `json:"product_id"`
	SellingPointIDs       []string `json:"selling_point_ids"`
	CustomSellingPoints   []string `json:"custom_selling_points"`
	VariantCount          int      `json:"variant_count"`
	TargetDurationSeconds int      `json:"target_duration_seconds"`
}

func (s *Server) handleGenerateWorkbenchScripts(c *gin.Context) {
	var request generateWorkbenchScriptsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_script_generation", "文案生成参数格式不正确")
		return
	}
	variants, err := s.scriptGenerationService.Generate(c.Request.Context(), services.WorkbenchScriptGenerationInput{
		ProductID:             request.ProductID,
		SellingPointIDs:       request.SellingPointIDs,
		CustomSellingPoints:   request.CustomSellingPoints,
		VariantCount:          request.VariantCount,
		TargetDurationSeconds: request.TargetDurationSeconds,
	})
	if err != nil {
		handleScriptGenerationError(c, err)
		return
	}
	OK(c, variants)
}

func handleScriptGenerationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrProductNotFound):
		Fail(c, http.StatusNotFound, "product_not_found", "产品不存在或已归档")
	case errors.Is(err, services.ErrScriptGenerationInput):
		Fail(c, http.StatusBadRequest, "invalid_script_generation", err.Error())
	case errors.Is(err, services.ErrLLMNotConfigured):
		Fail(c, http.StatusConflict, "llm_not_configured", "请先在系统设置中配置默认 LLM 模型")
	default:
		var gatewayError *modelgateway.Error
		if errors.As(err, &gatewayError) {
			switch gatewayError.Code {
			case modelgateway.ErrorCodeConfiguration, modelgateway.ErrorCodeUnsupportedProvider:
				Fail(c, http.StatusConflict, "llm_not_configured", "默认 LLM 模型不可用")
			case modelgateway.ErrorCodeTimeout:
				Fail(c, http.StatusGatewayTimeout, "llm_timeout", "文案生成超时，请重试")
			default:
				Fail(c, http.StatusBadGateway, "llm_generation_failed", "文案生成服务返回异常")
			}
			return
		}
		Fail(c, http.StatusInternalServerError, "script_generation_error", "文案生成失败")
	}
}
