package httpserver

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func (s *Server) handlePreprocessVLMLabel(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(64 << 20); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_multipart", err.Error())
		return
	}

	tempDir := filepath.Join(s.cfg.StorageRoot, "tmp", "preprocess-vlm", uuid.NewString())
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		Fail(c, http.StatusInternalServerError, "create_temp_dir_failed", err.Error())
		return
	}
	defer os.RemoveAll(tempDir)

	frames := make([]modelgateway.FrameReference, 0, 3)
	for index := 0; index < 3; index++ {
		formKey := fmt.Sprintf("frame_%d", index)
		file, err := c.FormFile(formKey)
		if err != nil {
			Fail(c, http.StatusBadRequest, "missing_frame", fmt.Sprintf("%s is required", formKey))
			return
		}

		framePath := filepath.Join(tempDir, fmt.Sprintf("frame_%03d%s", index, safeFrameExt(file.Filename)))
		if err := c.SaveUploadedFile(file, framePath); err != nil {
			Fail(c, http.StatusInternalServerError, "save_frame_failed", err.Error())
			return
		}

		frames = append(frames, modelgateway.FrameReference{
			FrameIndex:  index,
			TimestampMs: formInt(c, fmt.Sprintf("frame_%d_timestamp_ms", index)),
			StorageKey:  framePath,
		})
	}

	analyzer := modelgateway.NewAnalyzer(services.ResolveVLMAnalyzerConfigWithProviders(c.Request.Context(), s.systemConfigService, s.modelProviderService, s.cfg), nil)
	result, err := analyzer.AnalyzeAsset(c.Request.Context(), modelgateway.AnalyzeAssetInput{
		AssetID:        c.PostForm("asset_id"),
		SourceType:     c.PostForm("source_type"),
		ProductName:    c.PostForm("product_name"),
		DurationMs:     formInt(c, "duration_ms"),
		Width:          formInt(c, "width"),
		Height:         formInt(c, "height"),
		HasAudio:       c.PostForm("has_audio") == "true",
		AudioCodec:     c.PostForm("audio_codec"),
		FrameSnapshots: frames,
	})
	if err != nil {
		Fail(c, http.StatusBadGateway, "vlm_label_failed", err.Error())
		return
	}

	OK(c, gin.H{"analysis": result})
}

func safeFrameExt(name string) string {
	ext := filepath.Ext(name)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return ext
	default:
		return ".jpg"
	}
}

func formInt(c *gin.Context, key string) int {
	value, err := strconv.Atoi(c.PostForm(key))
	if err != nil {
		return 0
	}
	return value
}
