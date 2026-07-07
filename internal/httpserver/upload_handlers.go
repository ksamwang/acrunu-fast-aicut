package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type createUploadTokenRequest struct {
	ProductID string `json:"product_id" binding:"required"`
}

func (s *Server) handleCreateUploadToken(c *gin.Context) {
	var req createUploadTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid upload token payload")
		return
	}

	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	token, err := s.uploadTokenService.Create(req.ProductID, user.ID, 30*time.Minute)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to create upload token")
		return
	}

	Created(c, token)
}

func (s *Server) handleUploadCleanShot(c *gin.Context) {
	tokenValue := c.GetHeader("X-Upload-Token")
	if tokenValue == "" {
		tokenValue = c.PostForm("upload_token")
	}
	if tokenValue == "" {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing upload token")
		return
	}

	token, err := s.uploadTokenService.Consume(tokenValue)
	if err != nil {
		Fail(c, http.StatusUnauthorized, "unauthorized", "invalid upload token")
		return
	}

	sourceType := c.PostForm("source_type")
	if sourceType != "visual_only" && sourceType != "talking_head" {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid source type")
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "missing clean shot file")
		return
	}
	defer file.Close()

	hasher := sha256.New()
	tee := io.TeeReader(file, hasher)

	fileID := uuid.NewString()
	ext := filepath.Ext(header.Filename)
	storageKey := filepath.ToSlash(filepath.Join("assets", fileID+ext))

	fullPath, err := s.localStore.Save(storageKey, tee)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "internal_error", "failed to save clean shot")
		return
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	probeResult, probeErr := ffmpeg.Probe(context.Background(), fullPath)
	status := "ready"
	if probeErr != nil {
		status = "failed"
	}

	asset, err := s.productAssetService.CreateAsset(services.CreateAssetInput{
		ProductID:         token.ProductID,
		StorageKey:        storageKey,
		FileName:          header.Filename,
		FileExt:           ext,
		MimeType:          header.Header.Get("Content-Type"),
		FileSize:          header.Size,
		Checksum:          checksum,
		SourceType:        sourceType,
		DurationMs:        probeResult.DurationMs,
		Width:             probeResult.Width,
		Height:            probeResult.Height,
		FPS:               probeResult.FPS,
		Codec:             probeResult.Codec,
		Status:            status,
		ManualCleanStatus: "cleaned",
		CreatedByUserID:   token.UserID,
	})
	if err != nil {
		handleProductError(c, err)
		return
	}

	response := gin.H{"asset": asset}
	if probeErr != nil {
		response["probe_error"] = probeErr.Error()
	}
	Created(c, response)
}

func (s *Server) handleListAssets(c *gin.Context) {
	OK(c, s.productAssetService.ListAssets())
}

func (s *Server) handleGetAsset(c *gin.Context) {
	asset, ok := s.productAssetService.GetAsset(c.Param("assetID"))
	if !ok {
		Fail(c, http.StatusNotFound, "not_found", "asset not found")
		return
	}
	OK(c, asset)
}

func uploadErrorMessage(err error) string {
	if errors.Is(err, services.ErrUploadTokenInvalid) {
		return "invalid upload token"
	}
	return fmt.Sprintf("upload failed: %v", err)
}
