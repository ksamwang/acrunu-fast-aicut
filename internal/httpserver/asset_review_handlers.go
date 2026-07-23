package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
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
