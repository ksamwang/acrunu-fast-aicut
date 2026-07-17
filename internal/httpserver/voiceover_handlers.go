package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

const (
	maxVoiceProfileRequestBytes = (20 << 20) + (1 << 20)
	voiceProfileMemoryBytes     = 8 << 20
)

type voiceProfileForm struct {
	input     services.VoiceProfileInput
	reference *services.VoiceReferenceAudio
	cleanup   func()
}

type createVoiceAuditionRequest struct {
	Text string `json:"text"`
}

type createVoiceoverTasksRequest struct {
	ProductID        string                           `json:"product_id"`
	VoiceProfileID   string                           `json:"voice_profile_id"`
	OutputRatio      string                           `json:"output_ratio"`
	SubtitlePresetID string                           `json:"subtitle_preset_id"`
	Variants         []services.VoiceoverVariantInput `json:"variants"`
}

func (s *Server) handleListVoiceProfiles(c *gin.Context) {
	profiles, err := s.voiceoverService.ListVoiceProfiles(c.Request.Context())
	if err != nil {
		Fail(c, http.StatusInternalServerError, "voice_profile_error", "无法读取音色")
		return
	}
	OK(c, profiles)
}

func (s *Server) handleCreateVoiceProfile(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	form, err := parseVoiceProfileForm(c, true)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid_voice_profile", err.Error())
		return
	}
	defer form.cleanup()

	profile, err := s.voiceoverService.CreateVoiceProfile(c.Request.Context(), form.input, *form.reference, user.ID)
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	if err := s.enqueueVoiceProfilePreview(c, user.ID, profile.ID); err != nil {
		_ = s.voiceoverService.MarkVoiceProfilePreviewFailed(c.Request.Context(), profile.ID, err)
		handleVoiceoverError(c, err)
		return
	}
	Created(c, profile)
}

func (s *Server) handleUpdateVoiceProfile(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	form, err := parseVoiceProfileForm(c, false)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid_voice_profile", err.Error())
		return
	}
	defer form.cleanup()

	profile, err := s.voiceoverService.UpdateVoiceProfile(c.Request.Context(), c.Param("voiceProfileID"), form.input, form.reference, user.ID)
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	if err := s.enqueueVoiceProfilePreview(c, user.ID, profile.ID); err != nil {
		_ = s.voiceoverService.MarkVoiceProfilePreviewFailed(c.Request.Context(), profile.ID, err)
		handleVoiceoverError(c, err)
		return
	}
	OK(c, profile)
}

func (s *Server) handleDeleteVoiceProfile(c *gin.Context) {
	if err := s.voiceoverService.DeleteVoiceProfile(c.Request.Context(), c.Param("voiceProfileID")); err != nil {
		handleVoiceoverError(c, err)
		return
	}
	OK(c, gin.H{"deleted": true})
}

func (s *Server) handleSetDefaultVoiceProfile(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	profile, err := s.voiceoverService.SetDefaultVoiceProfile(c.Request.Context(), c.Param("voiceProfileID"), user.ID)
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	OK(c, profile)
}

func (s *Server) handleCreateVoiceAudition(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	var request createVoiceAuditionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_voice_audition", "试听文本格式不正确")
		return
	}
	profileID := c.Param("voiceProfileID")
	if profile, err := s.voiceoverService.GetVoiceProfile(c.Request.Context(), profileID); err != nil {
		handleVoiceoverError(c, err)
		return
	} else if profile.Status != "enabled" || profile.PreviewStatus != "ready" {
		Fail(c, http.StatusConflict, "voice_profile_not_ready", "音色样音尚未可用")
		return
	}

	task, err := s.taskService.CreateVoiceAuditionTask(c.Request.Context(), user.ID, queue.VoiceAuditionPayload{VoiceProfileID: profileID})
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	audition, err := s.voiceoverService.CreateVoiceAudition(c.Request.Context(), task.ID, profileID, user.ID, request.Text)
	if err != nil {
		_ = s.taskService.MarkFailed(c.Request.Context(), task.ID, err.Error())
		handleVoiceoverError(c, err)
		return
	}
	if err := s.queueClient.EnqueueVoiceAudition(queue.VoiceAuditionPayload{TaskID: task.ID, AuditionID: audition.ID}); err != nil {
		_ = s.taskService.MarkFailed(c.Request.Context(), task.ID, err.Error())
		_ = s.voiceoverService.MarkVoiceAuditionFailed(c.Request.Context(), audition.ID, err)
		handleVoiceoverError(c, err)
		return
	}
	Created(c, audition)
}

func (s *Server) handleGetVoiceAudition(c *gin.Context) {
	audition, err := s.voiceoverService.GetVoiceAudition(c.Request.Context(), c.Param("auditionID"))
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	OK(c, audition)
}

func (s *Server) handleCreateVoiceoverTasks(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	var request createVoiceoverTasksRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_voiceover_tasks", "配音任务格式不正确")
		return
	}
	if request.ProductID == "" || request.VoiceProfileID == "" || len(request.Variants) == 0 || len(request.Variants) > 8 {
		Fail(c, http.StatusBadRequest, "invalid_voiceover_tasks", "产品、音色和 1 至 8 条文案均为必填")
		return
	}
	product, err := s.productAssetService.GetProduct(request.ProductID)
	if err != nil || product.Status == "archived" {
		Fail(c, http.StatusNotFound, "product_not_found", "产品不存在或已归档")
		return
	}
	profile, err := s.voiceoverService.GetVoiceProfile(c.Request.Context(), request.VoiceProfileID)
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	if profile.Status != "enabled" || profile.PreviewStatus != "ready" {
		Fail(c, http.StatusConflict, "voice_profile_not_ready", "音色样音尚未可用")
		return
	}
	renderConfig, err := s.subtitleStylePresetService.Resolve(c.Request.Context(), request.SubtitlePresetID, request.OutputRatio)
	if err != nil {
		handleSubtitleStyleError(c, err)
		return
	}
	for _, variant := range request.Variants {
		if strings.TrimSpace(variant.ScriptText) == "" {
			Fail(c, http.StatusBadRequest, "invalid_voiceover_tasks", "文案不能为空")
			return
		}
	}
	resolvedBGM := make([]*services.ResolvedBGMConfig, len(request.Variants))
	usedBGMTracks := map[string]struct{}{}
	for index, variant := range request.Variants {
		config, resolveErr := s.bgmTrackService.Resolve(c.Request.Context(), variant.BGM, usedBGMTracks)
		if resolveErr != nil {
			handleBGMTrackError(c, resolveErr)
			return
		}
		resolvedBGM[index] = config
		if config != nil {
			usedBGMTracks[config.TrackID] = struct{}{}
		}
	}

	works := make([]services.VoiceoverWork, 0, len(request.Variants))
	creatorName := strings.TrimSpace(user.DisplayName)
	if creatorName == "" {
		creatorName = strings.TrimSpace(user.Username)
	}
	for index, variant := range request.Variants {
		configSnapshot := renderConfig.Snapshot()
		configSnapshot["voice_profile_id"] = request.VoiceProfileID
		configSnapshot["variant_index"] = index + 1
		if resolvedBGM[index] != nil {
			configSnapshot["bgm"] = resolvedBGM[index]
		}
		run, err := s.generationRunService.Create(c.Request.Context(), services.CreateGenerationRunInput{
			ProductID:       product.ID,
			CreatedByUserID: user.ID,
			CreatedByName:   creatorName,
			ConfigSnapshot:  configSnapshot,
		})
		if err != nil {
			s.handleVoiceoverTaskCreateError(c, "create_generation_run", err)
			return
		}
		task, err := s.taskService.CreateVoiceoverGenerateTask(c.Request.Context(), user.ID, product.ID, queue.VoiceoverGeneratePayload{GenerationRunID: run.ID})
		if err != nil {
			_ = s.generationRunService.MarkFailed(c.Request.Context(), run.ID, err)
			s.handleVoiceoverTaskCreateError(c, "create_generation_task", err)
			return
		}
		if err := s.generationRunService.LinkTask(c.Request.Context(), run.ID, task.ID, "voiceover"); err != nil {
			_ = s.taskService.MarkFailed(c.Request.Context(), task.ID, err.Error())
			_ = s.generationRunService.MarkFailed(c.Request.Context(), run.ID, err)
			s.handleVoiceoverTaskCreateError(c, "link_generation_task", err)
			return
		}
		work, scriptVariantID, voiceoverID, err := s.voiceoverService.CreateVoiceoverWork(c.Request.Context(), services.CreateVoiceoverWorkInput{
			TaskID:         task.ID,
			ProductID:      product.ID,
			ProductName:    product.Name,
			VoiceProfileID: request.VoiceProfileID,
			VariantIndex:   index + 1,
			Variant:        variant,
		})
		if err != nil {
			_ = s.taskService.MarkFailed(c.Request.Context(), task.ID, err.Error())
			_ = s.generationRunService.MarkFailed(c.Request.Context(), run.ID, err)
			s.handleVoiceoverTaskCreateError(c, "create_voiceover_work", err)
			return
		}
		if err := s.generationRunService.AttachVoiceoverArtifacts(c.Request.Context(), run.ID, task.ID, scriptVariantID, voiceoverID); err != nil {
			_ = s.taskService.MarkFailed(c.Request.Context(), task.ID, err.Error())
			_ = s.generationRunService.MarkFailed(c.Request.Context(), run.ID, err)
			s.handleVoiceoverTaskCreateError(c, "attach_voiceover_artifacts", err)
			return
		}
		if err := s.queueClient.EnqueueVoiceoverGenerate(queue.VoiceoverGeneratePayload{
			TaskID:          task.ID,
			GenerationRunID: run.ID,
			ScriptVariantID: scriptVariantID,
			VoiceoverID:     voiceoverID,
		}); err != nil {
			_ = s.taskService.MarkFailed(c.Request.Context(), task.ID, err.Error())
			_ = s.generationRunService.MarkFailed(c.Request.Context(), run.ID, err)
			s.handleVoiceoverTaskCreateError(c, "enqueue_voiceover_generate", err)
			return
		}
		if generatedWork, err := s.generationRunService.GetWork(c.Request.Context(), run.ID); err == nil {
			work = generatedWork
		}
		works = append(works, work)
	}
	Created(c, works)
}

func (s *Server) handleVoiceoverTaskCreateError(c *gin.Context, stage string, err error) {
	s.logger.Error("create voiceover task failed", "stage", stage, "error", err)
	handleVoiceoverError(c, err)
}

func (s *Server) handleListVoiceoverWorks(c *gin.Context) {
	works, err := s.generationRunService.ListWorks(c.Request.Context())
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	OK(c, works)
}

func (s *Server) handleGetVoiceoverWork(c *gin.Context) {
	work, err := s.generationRunService.GetWork(c.Request.Context(), c.Param("taskID"))
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	OK(c, work)
}

func (s *Server) handleRetryVoiceoverWork(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	ctx := c.Request.Context()
	run, err := s.generationRunService.Get(ctx, c.Param("taskID"))
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	if run.Status != "failed" {
		Fail(c, http.StatusConflict, "generation_not_retryable", "只有生成失败的成品可以重试")
		return
	}
	if run.VoiceoverTaskID == "" {
		Fail(c, http.StatusConflict, "generation_not_retryable", "原始配音任务不完整，无法重试")
		return
	}
	product, err := s.productAssetService.GetProduct(run.ProductID)
	if err != nil || product.Status == "archived" {
		Fail(c, http.StatusConflict, "product_not_available", "产品不存在或已归档，无法重试")
		return
	}
	plan, planErr := s.generationRunService.GetEditPlan(ctx, run.ID)
	if planErr != nil && !errors.Is(planErr, services.ErrEditPlanNotFound) {
		handleVoiceoverError(c, planErr)
		return
	}
	if planErr == nil && plan.Status == "ready" && len(plan.Clips) > 0 {
		work, retryErr := s.retryRender(ctx, run)
		if retryErr != nil {
			s.handleVoiceoverTaskCreateError(c, "retry_render", retryErr)
			return
		}
		OK(c, work)
		return
	}
	original, err := s.voiceoverService.GetVoiceoverWork(ctx, run.VoiceoverTaskID)
	if err != nil {
		Fail(c, http.StatusConflict, "generation_not_retryable", "原始配音任务不完整，无法重试")
		return
	}

	if original.Status == "completed" && original.DurationMs > 0 && len(original.NarrationSegments) > 0 && run.ScriptVariantID != "" && run.VoiceoverID != "" {
		work, err := s.retryEditPlan(ctx, run)
		if err != nil {
			s.handleVoiceoverTaskCreateError(c, "retry_edit_plan", err)
			return
		}
		OK(c, work)
		return
	}

	work, err := s.retryVoiceover(ctx, user.ID, run, product, original)
	if err != nil {
		s.handleVoiceoverTaskCreateError(c, "retry_voiceover", err)
		return
	}
	OK(c, work)
}

func (s *Server) handleRegenerateVoiceoverWork(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	ctx := c.Request.Context()
	run, err := s.generationRunService.Get(ctx, c.Param("taskID"))
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	if run.Status == "generating" {
		Fail(c, http.StatusConflict, "generation_active", "成品正在生成，不能重复生成")
		return
	}
	if run.VoiceoverTaskID == "" {
		Fail(c, http.StatusConflict, "generation_not_retryable", "原始配音任务不完整，无法重新生成")
		return
	}
	product, err := s.productAssetService.GetProduct(run.ProductID)
	if err != nil || product.Status == "archived" {
		Fail(c, http.StatusConflict, "product_not_available", "产品不存在或已归档，无法重新生成")
		return
	}
	original, err := s.voiceoverService.GetVoiceoverWork(ctx, run.VoiceoverTaskID)
	if err != nil {
		Fail(c, http.StatusConflict, "generation_not_retryable", "原始配音任务不完整，无法重新生成")
		return
	}
	work, err := s.retryVoiceover(ctx, user.ID, run, product, original)
	if err != nil {
		if current, currentErr := s.generationRunService.Get(ctx, run.ID); currentErr == nil && current.Status != "completed" {
			s.removeGenerationOutput(run.OutputStorageKey)
		}
		s.handleVoiceoverTaskCreateError(c, "regenerate_voiceover", err)
		return
	}
	s.removeGenerationOutput(run.OutputStorageKey)
	OK(c, work)
}

func (s *Server) handleDeleteVoiceoverWork(c *gin.Context) {
	run, err := s.generationRunService.Delete(c.Request.Context(), c.Param("taskID"))
	if err != nil {
		if errors.Is(err, services.ErrGenerationRunActive) {
			Fail(c, http.StatusConflict, "generation_active", "成品正在生成，不能删除")
			return
		}
		handleVoiceoverError(c, err)
		return
	}
	s.removeGenerationOutput(run.OutputStorageKey)
	OK(c, gin.H{"deleted": true})
}

func (s *Server) removeGenerationOutput(storageKey string) {
	if storageKey == "" {
		return
	}
	if err := s.localStore.Delete(storageKey); err != nil {
		s.logger.Warn("remove generation output failed", "storage_key", storageKey, "error", err)
	}
}

func (s *Server) retryRender(ctx context.Context, run services.GenerationRun) (services.VoiceoverWork, error) {
	if _, err := s.generationRunService.PrepareRetry(ctx, run.ID, services.GenerationRunRetryRender); err != nil {
		return services.VoiceoverWork{}, err
	}
	task, err := s.taskService.CreateGenerationRenderTask(ctx, run.CreatedByUserID, run.ProductID, queue.GenerationRenderPayload{GenerationRunID: run.ID})
	if err != nil {
		return services.VoiceoverWork{}, s.failGenerationRetry(ctx, run.ID, "", err)
	}
	if err := s.generationRunService.LinkTask(ctx, run.ID, task.ID, "render"); err != nil {
		return services.VoiceoverWork{}, s.failGenerationRetry(ctx, run.ID, task.ID, err)
	}
	if err := s.queueClient.EnqueueGenerationRender(queue.GenerationRenderPayload{TaskID: task.ID, GenerationRunID: run.ID}); err != nil {
		return services.VoiceoverWork{}, s.failGenerationRetry(ctx, run.ID, task.ID, err)
	}
	if err := s.generationRunService.UpdateStage(ctx, run.ID, "rendering", 90); err != nil {
		return services.VoiceoverWork{}, s.failGenerationRetry(ctx, run.ID, task.ID, err)
	}
	return s.generationRunService.GetWork(ctx, run.ID)
}

func (s *Server) retryEditPlan(ctx context.Context, run services.GenerationRun) (services.VoiceoverWork, error) {
	if _, err := s.generationRunService.PrepareRetry(ctx, run.ID, services.GenerationRunRetryEditPlan); err != nil {
		return services.VoiceoverWork{}, err
	}
	task, err := s.taskService.CreateEditPlanGenerateTask(ctx, run.CreatedByUserID, run.ProductID, queue.EditPlanGeneratePayload{
		GenerationRunID: run.ID,
		ScriptVariantID: run.ScriptVariantID,
		VoiceoverID:     run.VoiceoverID,
	})
	if err != nil {
		return services.VoiceoverWork{}, s.failGenerationRetry(ctx, run.ID, "", err)
	}
	if err := s.generationRunService.LinkTask(ctx, run.ID, task.ID, "edit_plan"); err != nil {
		return services.VoiceoverWork{}, s.failGenerationRetry(ctx, run.ID, task.ID, err)
	}
	payload := queue.EditPlanGeneratePayload{
		TaskID:          task.ID,
		GenerationRunID: run.ID,
		ScriptVariantID: run.ScriptVariantID,
		VoiceoverID:     run.VoiceoverID,
	}
	if err := s.queueClient.EnqueueEditPlanGenerate(payload); err != nil {
		return services.VoiceoverWork{}, s.failGenerationRetry(ctx, run.ID, task.ID, err)
	}
	return s.generationRunService.GetWork(ctx, run.ID)
}

func (s *Server) retryVoiceover(ctx context.Context, userID string, run services.GenerationRun, product services.Product, original services.VoiceoverWork) (services.VoiceoverWork, error) {
	if original.VoiceProfileID == "" || strings.TrimSpace(original.ScriptText) == "" {
		return services.VoiceoverWork{}, services.ErrGenerationRunNotRetryable
	}
	profile, err := s.voiceoverService.GetVoiceProfile(ctx, original.VoiceProfileID)
	if err != nil {
		return services.VoiceoverWork{}, err
	}
	if profile.Status != "enabled" || profile.PreviewStatus != "ready" {
		return services.VoiceoverWork{}, services.ErrVoiceProfileNotReady
	}
	if _, err := s.generationRunService.PrepareRetry(ctx, run.ID, services.GenerationRunRetryVoiceover); err != nil {
		return services.VoiceoverWork{}, err
	}
	task, err := s.taskService.CreateVoiceoverGenerateTask(ctx, userID, run.ProductID, queue.VoiceoverGeneratePayload{GenerationRunID: run.ID})
	if err != nil {
		return services.VoiceoverWork{}, s.failGenerationRetry(ctx, run.ID, "", err)
	}
	if err := s.generationRunService.LinkTask(ctx, run.ID, task.ID, "voiceover"); err != nil {
		return services.VoiceoverWork{}, s.failGenerationRetry(ctx, run.ID, task.ID, err)
	}
	work, scriptVariantID, voiceoverID, err := s.voiceoverService.CreateVoiceoverWork(ctx, services.CreateVoiceoverWorkInput{
		TaskID:         task.ID,
		ProductID:      product.ID,
		ProductName:    product.Name,
		VoiceProfileID: original.VoiceProfileID,
		VariantIndex:   retryVariantIndex(run.ConfigSnapshot),
		Variant: services.VoiceoverVariantInput{
			Hook:          original.Hook,
			ScriptText:    original.ScriptText,
			EditingIntent: original.EditingIntent,
			Beats:         original.Beats,
		},
	})
	if err != nil {
		return services.VoiceoverWork{}, s.failGenerationRetry(ctx, run.ID, task.ID, err)
	}
	if err := s.generationRunService.AttachVoiceoverArtifacts(ctx, run.ID, task.ID, scriptVariantID, voiceoverID); err != nil {
		return services.VoiceoverWork{}, s.failGenerationRetry(ctx, run.ID, task.ID, err)
	}
	if err := s.queueClient.EnqueueVoiceoverGenerate(queue.VoiceoverGeneratePayload{
		TaskID:          task.ID,
		GenerationRunID: run.ID,
		ScriptVariantID: scriptVariantID,
		VoiceoverID:     voiceoverID,
	}); err != nil {
		return services.VoiceoverWork{}, s.failGenerationRetry(ctx, run.ID, task.ID, err)
	}
	if generated, err := s.generationRunService.GetWork(ctx, run.ID); err == nil {
		return generated, nil
	}
	return work, nil
}

func (s *Server) failGenerationRetry(ctx context.Context, runID string, taskID string, cause error) error {
	if taskID != "" {
		_ = s.taskService.MarkFailed(ctx, taskID, cause.Error())
	}
	_ = s.generationRunService.MarkFailed(ctx, runID, cause)
	return cause
}

func retryVariantIndex(snapshot map[string]any) int {
	if snapshot == nil {
		return 1
	}
	switch value := snapshot["variant_index"].(type) {
	case int:
		if value > 0 {
			return value
		}
	case int32:
		if value > 0 {
			return int(value)
		}
	case int64:
		if value > 0 {
			return int(value)
		}
	case float64:
		if value > 0 {
			return int(value)
		}
	}
	return 1
}

func (s *Server) enqueueVoiceProfilePreview(c *gin.Context, userID string, profileID string) error {
	task, err := s.taskService.CreateVoiceProfilePreviewTask(c.Request.Context(), userID, queue.VoiceProfilePreviewPayload{VoiceProfileID: profileID})
	if err != nil {
		return err
	}
	if err := s.queueClient.EnqueueVoiceProfilePreview(queue.VoiceProfilePreviewPayload{TaskID: task.ID, VoiceProfileID: profileID}); err != nil {
		_ = s.taskService.MarkFailed(c.Request.Context(), task.ID, err.Error())
		return err
	}
	return nil
}

func parseVoiceProfileForm(c *gin.Context, referenceRequired bool) (voiceProfileForm, error) {
	if c.Request.ContentLength > maxVoiceProfileRequestBytes {
		return voiceProfileForm{}, fmt.Errorf("参考音频不能超过 20 MiB")
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxVoiceProfileRequestBytes)
	if err := c.Request.ParseMultipartForm(voiceProfileMemoryBytes); err != nil {
		return voiceProfileForm{}, fmt.Errorf("无法解析音色表单")
	}
	cleanup := func() {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}
	input := services.VoiceProfileInput{
		Name:          c.PostForm("name"),
		Language:      c.PostForm("language"),
		StyleTags:     parseVoiceStyleTags(c.PostForm("style_tags_json")),
		ReferenceText: c.PostForm("reference_text"),
		PreviewText:   c.PostForm("preview_text"),
		Status:        c.PostForm("status"),
		IsDefault:     parseVoiceFormBool(c.PostForm("is_default")),
	}
	file, header, err := c.Request.FormFile("reference_audio")
	if errors.Is(err, http.ErrMissingFile) {
		if referenceRequired {
			cleanup()
			return voiceProfileForm{}, fmt.Errorf("必须上传参考音频")
		}
		return voiceProfileForm{input: input, cleanup: cleanup}, nil
	}
	if err != nil {
		cleanup()
		return voiceProfileForm{}, fmt.Errorf("无法读取参考音频")
	}
	return voiceProfileForm{
		input: input,
		reference: &services.VoiceReferenceAudio{
			Filename: header.Filename,
			MimeType: header.Header.Get("Content-Type"),
			Size:     header.Size,
			Reader:   file,
		},
		cleanup: func() {
			_ = file.Close()
			cleanup()
		},
	}, nil
}

func parseVoiceStyleTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(value), &tags); err == nil {
		return tags
	}
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' || r == '\n' })
}

func parseVoiceFormBool(value string) bool {
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func handleVoiceoverError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrVoiceProfileNotFound), errors.Is(err, services.ErrVoiceAuditionNotFound), errors.Is(err, services.ErrVoiceoverWorkNotFound), errors.Is(err, services.ErrGenerationRunNotFound):
		Fail(c, http.StatusNotFound, "not_found", "未找到对应的音色或配音任务")
	case errors.Is(err, services.ErrGenerationRunNotRetryable):
		Fail(c, http.StatusConflict, "generation_not_retryable", "当前成品不能重试")
	case errors.Is(err, services.ErrVoiceProfileDisabled):
		Fail(c, http.StatusConflict, "voice_profile_disabled", "音色已停用")
	case errors.Is(err, services.ErrVoiceProfileNotReady):
		Fail(c, http.StatusConflict, "voice_profile_not_ready", "音色样音尚未可用")
	case errors.Is(err, services.ErrVoiceProfileInUse):
		Fail(c, http.StatusConflict, "voice_profile_in_use", "该音色已被正式任务引用，不能删除")
	case errors.Is(err, services.ErrInvalidVoiceInput):
		Fail(c, http.StatusBadRequest, "invalid_voice_input", err.Error())
	default:
		Fail(c, http.StatusInternalServerError, "voiceover_error", "音色或配音任务处理失败")
	}
}
