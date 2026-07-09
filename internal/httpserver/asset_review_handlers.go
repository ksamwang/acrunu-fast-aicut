package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type updateAssetReviewRequest struct {
	SceneDescription string   `json:"scene_description"`
	ShotSize         string   `json:"shot_size"`
	CameraMovement   string   `json:"camera_movement"`
	Subjects         []string `json:"subjects"`
	SceneTags        []string `json:"scene_tags"`
	QualityTags      []string `json:"quality_tags"`
	UsabilityStatus  string   `json:"usability_status"`
	ReviewerNotes    string   `json:"reviewer_notes"`
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
		SceneDescription: req.SceneDescription,
		ShotSize:         req.ShotSize,
		CameraMovement:   req.CameraMovement,
		Subjects:         req.Subjects,
		SceneTags:        req.SceneTags,
		QualityTags:      req.QualityTags,
		UsabilityStatus:  req.UsabilityStatus,
		ReviewerNotes:    req.ReviewerNotes,
		UpdatedByUserID:  user.ID,
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
