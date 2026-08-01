package httpserver

import (
	"errors"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

const finishedWorkClipCandidateLimit = 30

type finishedWorkClipCandidate struct {
	AssetID           string   `json:"asset_id"`
	AssetName         string   `json:"asset_name,omitempty"`
	FileName          string   `json:"file_name"`
	SourceType        string   `json:"source_type"`
	DurationMs        int      `json:"duration_ms"`
	SourceInMs        int      `json:"source_in_ms"`
	MaxSourceInMs     int      `json:"max_source_in_ms"`
	VideoURL          string   `json:"video_url"`
	ThumbnailURL      string   `json:"thumbnail_url,omitempty"`
	SceneDescription  string   `json:"scene_description,omitempty"`
	ActionDescription string   `json:"action_description,omitempty"`
	SemanticScore     *float64 `json:"semantic_score,omitempty"`
	IsCurrent         bool     `json:"is_current"`
}

type finishedWorkClipCandidatesResponse struct {
	ClipID         string                      `json:"clip_id"`
	Query          string                      `json:"query"`
	ClipDurationMs int                         `json:"clip_duration_ms"`
	PlanUpdatedAt  time.Time                   `json:"plan_updated_at"`
	Current        finishedWorkClipCandidate   `json:"current"`
	Items          []finishedWorkClipCandidate `json:"items"`
}

type replaceFinishedWorkClipsRequest struct {
	PlanUpdatedAt string                             `json:"plan_updated_at"`
	Replacements  []services.EditPlanClipReplacement `json:"replacements"`
}

func (s *Server) handleListFinishedWorkClipCandidates(c *gin.Context) {
	ctx := c.Request.Context()
	run, err := s.generationRunService.Get(ctx, c.Param("taskID"))
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	if run.Status != "completed" || run.OutputStorageKey == "" {
		Fail(c, http.StatusConflict, "generation_active", "只有已完成的成片可以替换镜头素材")
		return
	}
	plan, err := s.generationRunService.GetEditPlan(ctx, run.ID)
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	clip, ok := findEditPlanClip(plan.Clips, c.Param("clipID"))
	if !ok {
		Fail(c, http.StatusNotFound, "clip_not_found", "镜头不存在")
		return
	}
	if clip.SourceType != "visual_only" {
		Fail(c, http.StatusConflict, "clip_not_replaceable", "当前镜头不是可替换的纯画面素材")
		return
	}
	clipDurationMs := clip.TimelineDurationMs
	if clipDurationMs <= 0 {
		clipDurationMs = clip.EndMs - clip.StartMs
	}
	query := strings.TrimSpace(c.Query("query"))
	if query == "" {
		query = strings.TrimSpace(clip.VisualGoal)
	}
	minimumDurationMs := clipDurationMs
	result, err := s.assetEmbeddingService.SearchAssets(ctx, services.AssetSemanticSearchInput{
		Query: query,
		Filters: services.AssetFilters{
			ProductID:        run.ProductID,
			SourceType:       "visual_only",
			Status:           "ready",
			AnalysisStatus:   "ready",
			MinDurationMs:    &minimumDurationMs,
			ExcludeDiscarded: true,
		},
		Limit: 100,
	})
	if err != nil {
		Fail(c, http.StatusBadGateway, "semantic_search_failed", err.Error())
		return
	}

	usedByOtherClip := make(map[string]struct{}, len(plan.Clips))
	for _, item := range plan.Clips {
		if item.ID != clip.ID {
			usedByOtherClip[item.AssetID] = struct{}{}
		}
	}
	items := make([]finishedWorkClipCandidate, 0, min(len(result.Items), finishedWorkClipCandidateLimit))
	seenAssetIDs := make(map[string]struct{}, len(result.Items))
	for _, asset := range result.Items {
		_, seen := seenAssetIDs[asset.ID]
		_, used := usedByOtherClip[asset.ID]
		if seen || used || !finishedWorkReplacementAssetEligible(asset, run.ProductID, clipDurationMs) {
			continue
		}
		seenAssetIDs[asset.ID] = struct{}{}
		candidate := s.finishedWorkClipCandidate(asset, clipDurationMs, 0, asset.ID == clip.AssetID)
		items = append(items, candidate)
		if len(items) == finishedWorkClipCandidateLimit {
			break
		}
	}

	currentAsset, exists := s.productAssetService.GetAsset(clip.AssetID)
	if !exists {
		Fail(c, http.StatusConflict, "current_asset_unavailable", "当前镜头素材不存在")
		return
	}
	current := s.finishedWorkClipCandidate(currentAsset, clipDurationMs, clip.SourceInMs, true)
	OK(c, finishedWorkClipCandidatesResponse{
		ClipID: clip.ID, Query: query, ClipDurationMs: clipDurationMs,
		PlanUpdatedAt: plan.UpdatedAt, Current: current, Items: items,
	})
}

func (s *Server) handleReplaceFinishedWorkClips(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	var input replaceFinishedWorkClipsRequest
	if err := c.ShouldBindJSON(&input); err != nil || len(input.Replacements) == 0 {
		Fail(c, http.StatusBadRequest, "invalid_clip_replacements", "请选择需要替换的镜头素材")
		return
	}
	basePlanUpdatedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(input.PlanUpdatedAt))
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid_plan_revision", "剪辑计划版本无效，请刷新后重试")
		return
	}
	ctx := c.Request.Context()
	run, err := s.generationRunService.Get(ctx, c.Param("taskID"))
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	if run.Status != "completed" || run.OutputStorageKey == "" {
		Fail(c, http.StatusConflict, "generation_active", "只有已完成的成片可以替换镜头素材")
		return
	}
	plan, err := s.generationRunService.GetEditPlan(ctx, run.ID)
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	if !plan.UpdatedAt.Equal(basePlanUpdatedAt) {
		Fail(c, http.StatusConflict, "edit_plan_changed", "镜头编排已更新，请刷新后重新选择")
		return
	}
	if _, err := services.MaterializeClipReplacementPlan(run, plan, input.Replacements, s.productAssetService); err != nil {
		handleClipReplacementError(c, err)
		return
	}
	if _, err := s.generationRunService.PrepareClipReplacementRender(ctx, run.ID, basePlanUpdatedAt); err != nil {
		handleClipReplacementError(c, err)
		return
	}

	payload := queue.GenerationRenderPayload{
		GenerationRunID:       run.ID,
		BaseEditPlanUpdatedAt: basePlanUpdatedAt.Format(time.RFC3339Nano),
		ClipReplacements:      make([]queue.GenerationRenderClipReplacement, 0, len(input.Replacements)),
	}
	for _, replacement := range input.Replacements {
		payload.ClipReplacements = append(payload.ClipReplacements, queue.GenerationRenderClipReplacement{
			ClipID: replacement.ClipID, AssetID: replacement.AssetID, SourceInMs: replacement.SourceInMs,
		})
	}
	task, err := s.taskService.CreateGenerationRenderTask(ctx, user.ID, run.ProductID, payload)
	if err != nil {
		_ = s.generationRunService.MarkClipReplacementFailed(ctx, run.ID, err)
		s.handleVoiceoverTaskCreateError(c, "replace_clips", err)
		return
	}
	payload.TaskID = task.ID
	if err := s.generationRunService.LinkTask(ctx, run.ID, task.ID, "render"); err != nil {
		_ = s.taskService.MarkFailed(ctx, task.ID, err.Error())
		_ = s.generationRunService.MarkClipReplacementFailed(ctx, run.ID, err)
		s.handleVoiceoverTaskCreateError(c, "replace_clips", err)
		return
	}
	if err := s.queueClient.EnqueueGenerationRender(payload); err != nil {
		_ = s.taskService.MarkFailed(ctx, task.ID, err.Error())
		_ = s.generationRunService.MarkClipReplacementFailed(ctx, run.ID, err)
		s.handleVoiceoverTaskCreateError(c, "replace_clips", err)
		return
	}
	work, err := s.generationRunService.GetWork(ctx, run.ID)
	if err != nil {
		handleVoiceoverError(c, err)
		return
	}
	OK(c, work)
}

func findEditPlanClip(clips []services.EditPlanClip, clipID string) (services.EditPlanClip, bool) {
	clipID = strings.TrimSpace(clipID)
	for _, clip := range clips {
		if clip.ID == clipID {
			return clip, true
		}
	}
	return services.EditPlanClip{}, false
}

func finishedWorkReplacementAssetEligible(asset services.Asset, productID string, durationMs int) bool {
	return asset.ProductID == productID && asset.SourceType == "visual_only" && asset.Status == "ready" &&
		asset.AnalysisStatus == "ready" && (asset.UsabilityStatus == "usable" || asset.UsabilityStatus == "needs_review") &&
		asset.DurationMs >= durationMs && strings.TrimSpace(asset.StorageKey) != ""
}

func (s *Server) finishedWorkClipCandidate(asset services.Asset, clipDurationMs int, sourceInMs int, current bool) finishedWorkClipCandidate {
	maxSourceInMs := max(0, asset.DurationMs-clipDurationMs)
	if sourceInMs > maxSourceInMs {
		sourceInMs = maxSourceInMs
	}
	frames := s.productAssetService.ListAssetFrameSnapshots(asset.ID)
	sort.SliceStable(frames, func(i, j int) bool { return frames[i].FrameIndex < frames[j].FrameIndex })
	thumbnailURL := ""
	if len(frames) > 0 {
		thumbnailURL = finishedWorkStorageURL(frames[0].StorageKey)
	}
	return finishedWorkClipCandidate{
		AssetID: asset.ID, AssetName: asset.AssetName, FileName: asset.FileName,
		SourceType: asset.SourceType, DurationMs: asset.DurationMs, SourceInMs: sourceInMs,
		MaxSourceInMs: maxSourceInMs, VideoURL: finishedWorkStorageURL(asset.StorageKey),
		ThumbnailURL: thumbnailURL, SceneDescription: asset.SceneDescription,
		ActionDescription: asset.ActionDescription, SemanticScore: asset.SemanticScore, IsCurrent: current,
	}
}

func finishedWorkStorageURL(storageKey string) string {
	storageKey = strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(storageKey)), "/")
	if storageKey == "" || storageKey == "." {
		return ""
	}
	return "/storage/" + storageKey
}

func handleClipReplacementError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrGenerationRunNotFound), errors.Is(err, services.ErrEditPlanNotFound), errors.Is(err, services.ErrEditPlanClipNotFound):
		Fail(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, services.ErrGenerationRunActive):
		Fail(c, http.StatusConflict, "generation_active", "成片正在处理，请稍后再试")
	case errors.Is(err, services.ErrEditPlanConflict):
		Fail(c, http.StatusConflict, "edit_plan_changed", "镜头编排已更新，请刷新后重新选择")
	case errors.Is(err, services.ErrClipReplacementInvalid):
		Fail(c, http.StatusBadRequest, "invalid_clip_replacements", err.Error())
	default:
		Fail(c, http.StatusInternalServerError, "clip_replacement_failed", err.Error())
	}
}
