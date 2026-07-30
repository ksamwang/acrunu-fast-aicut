package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type updateAssetReviewRequest struct {
	SceneDescription  string   `json:"scene_description"`
	ActionDescription string   `json:"action_description"`
	ShotSize          string   `json:"shot_size"`
	CameraMovement    string   `json:"camera_movement"`
	Subjects          []string `json:"subjects"`
	SceneTags         []string `json:"scene_tags"`
	QualityTags       []string `json:"quality_tags"`
	UsabilityStatus   string   `json:"usability_status"`
	ReviewerNotes     string   `json:"reviewer_notes"`
}

type updateAssetSellingPointsRequest struct {
	SellingPointIDs []string `json:"selling_point_ids"`
}

type updateAssetBusinessTagsRequest struct {
	IsCurated      bool     `json:"is_curated"`
	BusinessTags   []string `json:"business_tags"`
	NarrativeRoles []string `json:"narrative_roles"`
	UsageNotes     string   `json:"usage_notes"`
}

type archiveAssetsRequest struct {
	AssetIDs []string `json:"asset_ids"`
}

type reanalyzeAssetsRequest struct {
	AssetIDs []string `json:"asset_ids"`
}

type assetReanalysisQueueItem struct {
	Asset       services.Asset `json:"asset"`
	FrameTaskID string         `json:"frame_task_id"`
}

type assetReanalysisFailure struct {
	AssetID string `json:"asset_id"`
	Message string `json:"message"`
}

type assetBulkReanalysisResult struct {
	Queued     []assetReanalysisQueueItem `json:"queued"`
	SkippedIDs []string                   `json:"skipped_ids"`
	Failures   []assetReanalysisFailure   `json:"failures"`
}

const assetReanalysisFrameCount = 9

func (s *Server) handleReanalyzeAsset(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	asset, ok := s.productAssetService.GetAsset(c.Param("assetID"))
	if !ok {
		Fail(c, http.StatusNotFound, "not_found", "asset not found")
		return
	}
	if asset.Status == "archived" {
		Fail(c, http.StatusConflict, "asset_archived", "archived asset cannot be reanalyzed")
		return
	}
	if assetAnalysisInProgress(asset.AnalysisStatus) {
		Fail(c, http.StatusConflict, "analysis_in_progress", "asset analysis is already queued or running")
		return
	}

	queued, err := s.enqueueAssetReanalysis(c.Request.Context(), user.ID, asset)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "reanalyze_enqueue_failed", err.Error())
		return
	}
	OK(c, queued)
}

func (s *Server) handleReanalyzeAssets(c *gin.Context) {
	var req reanalyzeAssetsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.AssetIDs) == 0 {
		Fail(c, http.StatusBadRequest, "bad_request", "asset_ids are required")
		return
	}
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	result := assetBulkReanalysisResult{
		Queued:     []assetReanalysisQueueItem{},
		SkippedIDs: []string{},
		Failures:   []assetReanalysisFailure{},
	}
	seen := make(map[string]struct{}, len(req.AssetIDs))
	for _, rawAssetID := range req.AssetIDs {
		assetID := strings.TrimSpace(rawAssetID)
		if assetID == "" {
			continue
		}
		if _, exists := seen[assetID]; exists {
			result.SkippedIDs = append(result.SkippedIDs, assetID)
			continue
		}
		seen[assetID] = struct{}{}

		asset, exists := s.productAssetService.GetAsset(assetID)
		if !exists {
			result.Failures = append(result.Failures, assetReanalysisFailure{AssetID: assetID, Message: "asset not found"})
			continue
		}
		if asset.Status == "archived" || assetAnalysisInProgress(asset.AnalysisStatus) {
			result.SkippedIDs = append(result.SkippedIDs, assetID)
			continue
		}
		queued, err := s.enqueueAssetReanalysis(c.Request.Context(), user.ID, asset)
		if err != nil {
			result.Failures = append(result.Failures, assetReanalysisFailure{AssetID: assetID, Message: err.Error()})
			continue
		}
		result.Queued = append(result.Queued, queued)
	}
	OK(c, result)
}

func (s *Server) enqueueAssetReanalysis(ctx context.Context, userID string, asset services.Asset) (assetReanalysisQueueItem, error) {
	updated, err := s.productAssetService.UpdateAssetAnalysisState(asset.ID, services.AssetAnalysisStateUpdate{
		AnalysisStatus:  "pending_analysis",
		AnalysisError:   "",
		UpdatedByUserID: userID,
	})
	if err != nil {
		return assetReanalysisQueueItem{}, err
	}

	payload := queue.AssetExtractFramesPayload{
		AssetID:    asset.ID,
		StorageKey: asset.StorageKey,
		DurationMs: asset.DurationMs,
		Strategy: queue.FrameExtractionStrategy{
			Mode:       queue.FrameExtractionModeFixedInterval,
			FrameCount: assetReanalysisFrameCount,
		},
		SkipAnalyze: false,
	}
	frameTask, err := s.taskService.CreateAssetExtractFramesTask(ctx, userID, asset.ProductID, payload)
	if err != nil {
		_, _ = s.productAssetService.UpdateAssetAnalysisState(asset.ID, services.AssetAnalysisStateUpdate{AnalysisStatus: "failed", AnalysisError: err.Error(), UpdatedByUserID: userID})
		return assetReanalysisQueueItem{}, fmt.Errorf("create frame extraction task: %w", err)
	}
	payload.TaskID = frameTask.ID
	if err := s.queueClient.EnqueueAssetExtractFrames(payload); err != nil {
		_ = s.taskService.MarkFailed(context.Background(), frameTask.ID, err.Error())
		_, _ = s.productAssetService.UpdateAssetAnalysisState(asset.ID, services.AssetAnalysisStateUpdate{AnalysisStatus: "failed", AnalysisError: err.Error(), UpdatedByUserID: userID})
		return assetReanalysisQueueItem{}, fmt.Errorf("enqueue frame extraction task: %w", err)
	}
	return assetReanalysisQueueItem{Asset: updated, FrameTaskID: frameTask.ID}, nil
}

func assetAnalysisInProgress(status string) bool {
	return status == "pending_analysis" || status == "analyzing"
}

func (s *Server) handleUpdateAssetReview(c *gin.Context) {
	var req updateAssetReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid asset review payload")
		return
	}

	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	asset, err := s.productAssetService.UpdateAssetReview(c.Param("assetID"), services.AssetReviewUpdate{
		SceneDescription:  req.SceneDescription,
		ActionDescription: req.ActionDescription,
		ShotSize:          req.ShotSize,
		CameraMovement:    req.CameraMovement,
		Subjects:          req.Subjects,
		SceneTags:         req.SceneTags,
		QualityTags:       req.QualityTags,
		UsabilityStatus:   req.UsabilityStatus,
		ReviewerNotes:     req.ReviewerNotes,
		UpdatedByUserID:   user.ID,
	})
	if err != nil {
		handleProductError(c, err)
		return
	}

	OK(c, asset)
}

func (s *Server) handleArchiveAsset(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	asset, err := s.productAssetService.ArchiveAsset(c.Param("assetID"), services.AssetArchiveUpdate{
		UpdatedByUserID: user.ID,
	})
	if err != nil {
		handleProductError(c, err)
		return
	}

	OK(c, asset)
}

func (s *Server) handleArchiveAssets(c *gin.Context) {
	var req archiveAssetsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.AssetIDs) == 0 {
		Fail(c, http.StatusBadRequest, "bad_request", "asset_ids are required")
		return
	}

	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	result := s.productAssetService.ArchiveAssets(req.AssetIDs, services.AssetArchiveUpdate{
		UpdatedByUserID: user.ID,
	})
	OK(c, result)
}

func (s *Server) handleRestoreAsset(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	asset, err := s.productAssetService.RestoreAsset(c.Param("assetID"), services.AssetArchiveUpdate{
		UpdatedByUserID: user.ID,
	})
	if err != nil {
		handleProductError(c, err)
		return
	}

	OK(c, asset)
}

func (s *Server) handleListAssetSellingPoints(c *gin.Context) {
	items, err := s.productAssetService.ListAssetSellingPoints(c.Param("assetID"))
	if err != nil {
		handleProductError(c, err)
		return
	}
	OK(c, items)
}

func (s *Server) handleListAssetSpeechSegments(c *gin.Context) {
	items, err := s.productAssetService.ListSpeechSegmentsByAsset(c.Param("assetID"))
	if err != nil {
		handleProductError(c, err)
		return
	}
	OK(c, items)
}

func (s *Server) handleGetAssetSemanticPreview(c *gin.Context) {
	preview, err := s.productAssetService.BuildAssetSemanticPreview(c.Param("assetID"))
	if err != nil {
		handleProductError(c, err)
		return
	}
	OK(c, preview)
}

func (s *Server) handleListAssetEmbeddings(c *gin.Context) {
	items, err := s.assetEmbeddingService.ListAssetEmbeddingObjects(c.Request.Context(), c.Param("assetID"))
	if err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	OK(c, gin.H{
		"asset_id": c.Param("assetID"),
		"items":    items,
	})
}

func (s *Server) handleVectorizeAsset(c *gin.Context) {
	result, err := s.assetEmbeddingService.VectorizeAsset(c.Request.Context(), c.Param("assetID"))
	if err != nil {
		if err == services.ErrAssetNotFound {
			handleProductError(c, err)
			return
		}
		Fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	OK(c, result)
}

func (s *Server) handleUpdateAssetSellingPoints(c *gin.Context) {
	var req updateAssetSellingPointsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid asset selling points payload")
		return
	}

	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	items, err := s.productAssetService.UpdateAssetSellingPoints(c.Param("assetID"), services.AssetSellingPointsUpdate{
		SellingPointIDs: req.SellingPointIDs,
		UpdatedByUserID: user.ID,
	})
	if err != nil {
		handleProductError(c, err)
		return
	}

	OK(c, items)
}

func (s *Server) handleUpdateAssetBusinessTags(c *gin.Context) {
	var req updateAssetBusinessTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid asset business tags payload")
		return
	}

	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	asset, err := s.productAssetService.UpdateAssetBusinessTags(c.Param("assetID"), services.AssetBusinessTagUpdate{
		IsCurated:       req.IsCurated,
		BusinessTags:    req.BusinessTags,
		NarrativeRoles:  req.NarrativeRoles,
		UsageNotes:      req.UsageNotes,
		UpdatedByUserID: user.ID,
	})
	if err != nil {
		handleProductError(c, err)
		return
	}

	OK(c, asset)
}
