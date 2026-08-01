package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
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
	Selectable        bool     `json:"selectable"`
	UnavailableReason string   `json:"unavailable_reason,omitempty"`
	ShortfallMs       int      `json:"shortfall_ms,omitempty"`
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
	clipIndex, clip, ok := findEditPlanClip(plan.Clips, c.Param("clipID"))
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
	result, err := s.assetEmbeddingService.SearchAssets(ctx, services.AssetSemanticSearchInput{
		Query: query,
		Filters: services.AssetFilters{
			ProductID:  run.ProductID,
			SourceType: "visual_only",
		},
		Limit: 100,
	})
	if err != nil {
		Fail(c, http.StatusBadGateway, "semantic_search_failed", err.Error())
		return
	}

	currentAsset, exists := s.productAssetService.GetAsset(clip.AssetID)
	if !exists {
		Fail(c, http.StatusConflict, "current_asset_unavailable", "当前镜头素材不存在")
		return
	}
	usedByOtherClip := make(map[string]int, len(plan.Clips))
	for index, item := range plan.Clips {
		if index != clipIndex {
			usedByOtherClip[item.AssetID] = index
		}
	}
	items := make([]finishedWorkClipCandidate, 0, min(len(result.Items), finishedWorkClipCandidateLimit))
	seenAssetIDs := make(map[string]struct{}, len(result.Items))
	for _, asset := range result.Items {
		if asset.ID == clip.AssetID {
			currentAsset.SemanticScore = asset.SemanticScore
			continue
		}
		_, seen := seenAssetIDs[asset.ID]
		if seen {
			continue
		}
		seenAssetIDs[asset.ID] = struct{}{}
		selectable, reason, shortfallMs := s.finishedWorkClipCandidateAvailability(
			run, plan, clipIndex, asset, usedByOtherClip,
		)
		candidate := s.finishedWorkClipCandidate(asset, clipDurationMs, 0, false, selectable, reason, shortfallMs)
		items = append(items, candidate)
		if len(items) == finishedWorkClipCandidateLimit {
			break
		}
	}

	current := s.finishedWorkClipCandidate(currentAsset, clipDurationMs, clip.SourceInMs, true, true, "", 0)
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

func findEditPlanClip(clips []services.EditPlanClip, clipID string) (int, services.EditPlanClip, bool) {
	clipID = strings.TrimSpace(clipID)
	for index, clip := range clips {
		if clip.ID == clipID {
			return index, clip, true
		}
	}
	return -1, services.EditPlanClip{}, false
}

func (s *Server) finishedWorkClipCandidateAvailability(
	run services.GenerationRun,
	plan services.EditPlan,
	clipIndex int,
	asset services.Asset,
	usedByOtherClip map[string]int,
) (bool, string, int) {
	clip := plan.Clips[clipIndex]
	clipDurationMs := clip.TimelineDurationMs
	if clipDurationMs <= 0 {
		clipDurationMs = clip.EndMs - clip.StartMs
	}
	shortfallMs := max(0, clipDurationMs-asset.DurationMs)
	if usedIndex, used := usedByOtherClip[asset.ID]; used {
		return false, fmt.Sprintf("已被镜头 %02d 使用", usedIndex+1), shortfallMs
	}
	if reason := finishedWorkReplacementAssetUnavailableReason(asset, run.ProductID); reason != "" {
		return false, reason, shortfallMs
	}
	_, err := services.MaterializeClipReplacementPlan(run, plan, []services.EditPlanClipReplacement{{
		ClipID: clip.ID, AssetID: asset.ID, SourceInMs: 0,
	}}, s.productAssetService)
	if err == nil {
		return true, "", shortfallMs
	}
	if shortfallMs > 0 {
		switch {
		case clipIndex == len(plan.Clips)-1:
			return false, fmt.Sprintf("最后一镜时长不足，还差 %dms", shortfallMs), shortfallMs
		case shortfallMs > modelgateway.MaximumEditPlanEarlyTransitionMs:
			return false, fmt.Sprintf("时长不足，还差 %dms", shortfallMs), shortfallMs
		default:
			return false, "下一镜头无法承接提前转场", shortfallMs
		}
	}
	return false, "不符合当前剪辑计划", 0
}

func finishedWorkReplacementAssetUnavailableReason(asset services.Asset, productID string) string {
	if asset.ProductID != productID || asset.SourceType != "visual_only" {
		return "素材不属于当前产品或类型"
	}
	if strings.TrimSpace(asset.StorageKey) == "" {
		return "视频文件不可用"
	}
	if asset.Status == "archived" {
		return "素材已归档"
	}
	if asset.Status != "ready" {
		return "素材状态不可用"
	}
	if asset.AnalysisStatus != "ready" {
		return "VLM 分析未完成"
	}
	if asset.UsabilityStatus == "discarded" {
		return "素材已废弃"
	}
	if asset.UsabilityStatus != "usable" && asset.UsabilityStatus != "needs_review" {
		return "素材尚不可用"
	}
	return ""
}

func (s *Server) finishedWorkClipCandidate(
	asset services.Asset,
	clipDurationMs int,
	sourceInMs int,
	current bool,
	selectable bool,
	unavailableReason string,
	shortfallMs int,
) finishedWorkClipCandidate {
	maxSourceInMs := 0
	if asset.DurationMs >= clipDurationMs {
		maxSourceInMs = asset.DurationMs - clipDurationMs
	}
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
		Selectable: selectable, UnavailableReason: unavailableReason, ShortfallMs: shortfallMs,
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
