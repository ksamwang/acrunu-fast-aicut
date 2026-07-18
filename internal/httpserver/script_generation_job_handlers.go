package httpserver

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type createScriptGenerationJobRequest struct {
	ProductID           string   `json:"product_id"`
	SellingPointIDs     []string `json:"selling_point_ids"`
	CustomSellingPoints []string `json:"custom_selling_points"`
	VariantCount        int      `json:"variant_count"`
	Mode                string   `json:"mode"`
	TargetVariantID     string   `json:"target_variant_id"`
	BaseRevision        string   `json:"base_revision"`
}

type resolveScriptGenerationJobRequest struct {
	Resolution string `json:"resolution"`
}

func (s *Server) handleCreateScriptGenerationJob(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	var request createScriptGenerationJobRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_script_generation", "文案生成参数格式不正确")
		return
	}
	job, err := s.scriptGenerationJobService.Create(c.Request.Context(), services.CreateScriptGenerationJobInput{
		CreatedByUserID: user.ID,
		Mode:            request.Mode,
		TargetVariantID: request.TargetVariantID,
		BaseRevision:    request.BaseRevision,
		GenerationInput: services.WorkbenchScriptGenerationInput{
			ProductID:           request.ProductID,
			SellingPointIDs:     request.SellingPointIDs,
			CustomSellingPoints: request.CustomSellingPoints,
			VariantCount:        request.VariantCount,
		},
	})
	if err != nil {
		handleScriptGenerationJobError(c, err)
		return
	}
	if err := s.queueClient.EnqueueWorkbenchScriptGenerate(queue.WorkbenchScriptGeneratePayload{JobID: job.ID}); err != nil {
		_ = s.scriptGenerationJobService.MarkFailed(c.Request.Context(), job.ID, err)
		Fail(c, http.StatusServiceUnavailable, "script_generation_queue_failed", "文案生成任务入队失败")
		return
	}
	Created(c, job)
}

func (s *Server) handleGetScriptGenerationJob(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	job, err := s.scriptGenerationJobService.GetForUser(c.Request.Context(), c.Param("jobID"), user.ID)
	if err != nil {
		handleScriptGenerationJobError(c, err)
		return
	}
	OK(c, job)
}

func (s *Server) handleGetLatestScriptGenerationJob(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	job, err := s.scriptGenerationJobService.GetLatestUnresolvedForUser(c.Request.Context(), user.ID)
	if errors.Is(err, services.ErrScriptGenerationJobNotFound) {
		OK(c, nil)
		return
	}
	if err != nil {
		handleScriptGenerationJobError(c, err)
		return
	}
	OK(c, job)
}

func (s *Server) handleCancelScriptGenerationJob(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	job, err := s.scriptGenerationJobService.CancelForUser(c.Request.Context(), c.Param("jobID"), user.ID)
	if err != nil {
		handleScriptGenerationJobError(c, err)
		return
	}
	OK(c, job)
}

func (s *Server) handleResolveScriptGenerationJob(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	var request resolveScriptGenerationJobRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_script_generation_resolution", "文案生成结果处理参数不正确")
		return
	}
	job, err := s.scriptGenerationJobService.ResolveForUser(c.Request.Context(), c.Param("jobID"), user.ID, request.Resolution)
	if err != nil {
		handleScriptGenerationJobError(c, err)
		return
	}
	OK(c, job)
}

func handleScriptGenerationJobError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrScriptGenerationJobNotFound):
		Fail(c, http.StatusNotFound, "script_generation_job_not_found", "文案生成任务不存在")
	case errors.Is(err, services.ErrScriptGenerationJobActive):
		Fail(c, http.StatusConflict, "script_generation_job_active", "已有文案正在生成")
	case errors.Is(err, services.ErrScriptGenerationJobState):
		Fail(c, http.StatusConflict, "script_generation_job_state", "当前文案生成状态不允许此操作")
	default:
		handleScriptGenerationError(c, err)
	}
}
