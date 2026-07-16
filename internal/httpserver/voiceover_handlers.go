package httpserver

import (
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
	ProductID      string                           `json:"product_id"`
	VoiceProfileID string                           `json:"voice_profile_id"`
	Variants       []services.VoiceoverVariantInput `json:"variants"`
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
	for _, variant := range request.Variants {
		if strings.TrimSpace(variant.ScriptText) == "" {
			Fail(c, http.StatusBadRequest, "invalid_voiceover_tasks", "文案不能为空")
			return
		}
	}

	works := make([]services.VoiceoverWork, 0, len(request.Variants))
	for index, variant := range request.Variants {
		run, err := s.generationRunService.Create(c.Request.Context(), services.CreateGenerationRunInput{
			ProductID:       product.ID,
			CreatedByUserID: user.ID,
			ConfigSnapshot: map[string]any{
				"voice_profile_id": request.VoiceProfileID,
				"variant_index":    index + 1,
			},
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
