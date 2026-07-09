package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/ffmpeg"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type createUploadTokenRequest struct {
	ProductID string `json:"product_id" binding:"required"`
}

type assetListResponse struct {
	Items    []services.Asset `json:"items"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
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
	submissionMode := firstNonEmptyForm(c.PostForm("submission_mode"), "legacy")
	manualCleanStatus := firstNonEmptyForm(c.PostForm("manual_clean_status"), "cleaned")
	usabilityStatus := firstNonEmptyForm(c.PostForm("usability_status"), "usable")
	assetName := c.PostForm("asset_name")
	sourcePath := c.PostForm("source_path")
	sourceOriginalName := c.PostForm("source_original_name")
	reviewerNotes := c.PostForm("reviewer_notes")
	sellingPointIDs := splitCommaSeparated(c.PostForm("selling_point_ids"))
	transcript := c.PostForm("transcript")

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

	assetInput := services.CreateAssetInput{
		ProductID:          token.ProductID,
		AssetName:          assetName,
		StorageKey:         storageKey,
		FileName:           header.Filename,
		FileExt:            ext,
		MimeType:           firstNonEmptyForm(header.Header.Get("Content-Type"), mimeTypeFromExtension(ext)),
		FileSize:           header.Size,
		Checksum:           checksum,
		SourceType:         sourceType,
		IngestionSource:    "local-agent",
		ManualCleanStatus:  manualCleanStatus,
		SourcePath:         sourcePath,
		SourceOriginalName: firstNonEmptyForm(sourceOriginalName, header.Filename),
		ReviewerNotes:      reviewerNotes,
		SellingPointIDs:    sellingPointIDs,
		CreatedByUserID:    token.UserID,
	}

	var probeErr error
	if submissionMode == "preprocessed" {
		if err := applyPreprocessedAssetFields(c, &assetInput, sourceType, transcript); err != nil {
			Fail(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
	} else {
		probeResult, err := ffmpeg.Probe(context.Background(), fullPath)
		probeErr = err
		assetInput.DurationMs = probeResult.DurationMs
		assetInput.Width = probeResult.Width
		assetInput.Height = probeResult.Height
		assetInput.FPS = probeResult.FPS
		assetInput.Codec = probeResult.Codec
		assetInput.HasAudio = probeResult.HasAudio
		assetInput.AudioCodec = probeResult.AudioCodec
		assetInput.BitrateKbps = probeResult.BitrateKbps
		assetInput.LikelyHasSpeech = ffmpeg.LikelyHasHumanSpeech(sourceType, probeResult)
		assetInput.UsabilityStatus = usabilityStatus
		assetInput.Status = "ready"
		assetInput.AnalysisStatus = "pending_analysis"
		if probeErr != nil {
			assetInput.Status = "failed"
			assetInput.AnalysisStatus = "failed"
			assetInput.AnalysisError = probeErr.Error()
		}
	}

	asset, err := s.productAssetService.CreateAsset(assetInput)
	if err != nil {
		handleProductError(c, err)
		return
	}
	if transcript != "" {
		if err := createTranscriptSegments(s.productAssetService, asset.ID, transcript, token.UserID); err != nil {
			Fail(c, http.StatusInternalServerError, "internal_error", "failed to persist transcript segments")
			return
		}
	}

	response := gin.H{"asset": asset}
	duplicates := s.productAssetService.FindDuplicateAssetsByChecksum(checksum, asset.ID)
	response["is_duplicate"] = len(duplicates) > 0
	if len(duplicates) > 0 {
		response["duplicate_assets"] = duplicates
	}
	if submissionMode != "preprocessed" && probeErr == nil {
		frameTask, taskErr := s.taskService.CreateAssetExtractFramesTask(c.Request.Context(), token.UserID, asset.ProductID, servicesQueueExtractPayload(asset, ""))
		if taskErr != nil {
			response["frame_task_error"] = taskErr.Error()
		} else {
			response["frame_task"] = frameTask
		}
		if frameTask.ID != "" {
			response["frame_task_id"] = frameTask.ID
		}
		if frameTask.ID != "" {
			if enqueueErr := s.queueClient.EnqueueAssetExtractFrames(servicesQueueExtractPayload(asset, frameTask.ID)); enqueueErr != nil {
				_ = s.taskService.MarkFailed(c.Request.Context(), frameTask.ID, enqueueErr.Error())
				response["frame_task_error"] = enqueueErr.Error()
			}
		}
	}
	if submissionMode != "preprocessed" && probeErr != nil {
		response["probe_error"] = probeErr.Error()
	}
	Created(c, response)
}

func (s *Server) handleListAssets(c *gin.Context) {
	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("page_size"), 20)
	assets := s.productAssetService.ListAssets(services.AssetFilters{
		ProductID:        c.Query("product_id"),
		SourceType:       c.Query("source_type"),
		Status:           c.Query("status"),
		AnalysisStatus:   c.Query("analysis_status"),
		UsabilityStatus:  c.Query("usability_status"),
		ShotSize:         c.Query("shot_size"),
		SellingPointID:   c.Query("selling_point_id"),
		Tag:              c.Query("tag"),
		Keyword:          c.Query("keyword"),
		MinDurationMs:    parseOptionalInt(c.Query("min_duration_ms")),
		MaxDurationMs:    parseOptionalInt(c.Query("max_duration_ms")),
		HasAudio:         parseOptionalBool(c.Query("has_audio")),
		LikelyHasSpeech:  parseOptionalBool(c.Query("likely_has_speech")),
		ExcludeDiscarded: parseOptionalBoolDefaultFalse(c.Query("exclude_discarded")),
		SortBy:           c.Query("sort_by"),
	})

	start := (page - 1) * pageSize
	if start > len(assets) {
		start = len(assets)
	}
	end := start + pageSize
	if end > len(assets) {
		end = len(assets)
	}

	OK(c, assetListResponse{
		Items:    assets[start:end],
		Total:    len(assets),
		Page:     page,
		PageSize: pageSize,
	})
}

func (s *Server) handleGetAsset(c *gin.Context) {
	asset, ok := s.productAssetService.GetAsset(c.Param("assetID"))
	if !ok {
		Fail(c, http.StatusNotFound, "not_found", "asset not found")
		return
	}
	OK(c, asset)
}

func (s *Server) handleListAssetFrames(c *gin.Context) {
	assetID := c.Param("assetID")
	asset, ok := s.productAssetService.GetAsset(assetID)
	if !ok {
		Fail(c, http.StatusNotFound, "not_found", "asset not found")
		return
	}

	frames := s.productAssetService.ListAssetFrameSnapshots(assetID)
	response := gin.H{
		"asset_id": asset.ID,
		"frames":   frames,
	}
	OK(c, response)
}

func uploadErrorMessage(err error) string {
	if errors.Is(err, services.ErrUploadTokenInvalid) {
		return "invalid upload token"
	}
	return fmt.Sprintf("upload failed: %v", err)
}

func splitCommaSeparated(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func firstNonEmptyForm(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func mimeTypeFromExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".mkv":
		return "video/x-matroska"
	default:
		return "application/octet-stream"
	}
}

func parseOptionalInt(value string) *int {
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseOptionalFloat(value string) *float64 {
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseOptionalBool(value string) *bool {
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func parsePositiveInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseOptionalBoolDefaultFalse(value string) bool {
	parsed := parseOptionalBool(value)
	return parsed != nil && *parsed
}

func applyPreprocessedAssetFields(c *gin.Context, input *services.CreateAssetInput, sourceType string, transcript string) error {
	if input == nil {
		return fmt.Errorf("asset input is required")
	}
	durationMs := parseOptionalInt(c.PostForm("duration_ms"))
	width := parseOptionalInt(c.PostForm("width"))
	height := parseOptionalInt(c.PostForm("height"))
	fps := parseOptionalFloat(c.PostForm("fps"))
	hasAudio := parseOptionalBool(c.PostForm("has_audio"))
	bitrateKbps := parseOptionalInt(c.PostForm("bitrate_kbps"))
	sourceInMs := parseOptionalInt(c.PostForm("source_in_ms"))
	sourceOutMs := parseOptionalInt(c.PostForm("source_out_ms"))

	if durationMs == nil || *durationMs <= 0 {
		return fmt.Errorf("duration_ms is required for preprocessed submission")
	}
	if sourceInMs == nil || sourceOutMs == nil || *sourceOutMs <= *sourceInMs {
		return fmt.Errorf("valid source_in_ms and source_out_ms are required for preprocessed submission")
	}
	if sourceType == "talking_head" && strings.TrimSpace(transcript) == "" {
		return fmt.Errorf("transcript is required for talking_head preprocessed submission")
	}

	input.DurationMs = *durationMs
	if width != nil {
		input.Width = *width
	}
	if height != nil {
		input.Height = *height
	}
	if fps != nil {
		input.FPS = *fps
	}
	if hasAudio != nil {
		input.HasAudio = *hasAudio
	}
	if bitrateKbps != nil {
		input.BitrateKbps = *bitrateKbps
	}
	input.SourceInMs = *sourceInMs
	input.SourceOutMs = *sourceOutMs
	input.Codec = c.PostForm("codec")
	input.AudioCodec = c.PostForm("audio_codec")
	input.Checksum = firstNonEmptyForm(c.PostForm("checksum"), input.Checksum)
	input.AnalysisStatus = firstNonEmptyForm(c.PostForm("analysis_status"), "ready")
	input.Status = "ready"
	input.UsabilityStatus = firstNonEmptyForm(c.PostForm("usability_status"), "usable")
	input.SceneDescription = c.PostForm("scene_description")
	input.ShotSize = c.PostForm("shot_size")
	input.CameraMovement = c.PostForm("camera_movement")
	input.Subjects = parseJSONStringArray(c.PostForm("subjects_json"))
	input.SceneTags = parseJSONStringArray(c.PostForm("scene_tags_json"))
	input.QualityTags = parseJSONStringArray(c.PostForm("quality_tags_json"))
	input.ModelResult = parseJSONObject(c.PostForm("model_result_json"))
	input.ModelLabels = map[string]any{
		"scene_description": input.SceneDescription,
		"shot_size":         input.ShotSize,
		"camera_movement":   input.CameraMovement,
		"subjects":          append([]string(nil), input.Subjects...),
		"scene_tags":        append([]string(nil), input.SceneTags...),
		"quality_tags":      append([]string(nil), input.QualityTags...),
		"usability_status":  input.UsabilityStatus,
	}
	input.LikelyHasSpeech = sourceType == "talking_head" || (hasAudio != nil && *hasAudio)
	return nil
}

func parseJSONStringArray(value string) []string {
	if value == "" {
		return nil
	}
	var decoded []string
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return nil
	}
	return decoded
}

func parseJSONObject(value string) map[string]any {
	if value == "" {
		return map[string]any{}
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return map[string]any{}
	}
	return decoded
}

func createTranscriptSegments(service *services.ProductAssetService, assetID string, transcript string, userID string) error {
	segments := parseTranscriptSegments(transcript)
	for _, segment := range segments {
		if _, err := service.CreateSpeechSegment(services.CreateSpeechSegmentInput{
			AssetID:         assetID,
			StartMs:         segment.StartMs,
			EndMs:           segment.EndMs,
			Transcript:      segment.Transcript,
			Source:          "local-agent",
			Status:          "ready",
			CreatedByUserID: userID,
		}); err != nil {
			return err
		}
	}
	return nil
}

type transcriptSegment struct {
	StartMs    int
	EndMs      int
	Transcript string
}

func parseTranscriptSegments(transcript string) []transcriptSegment {
	lines := strings.Split(strings.ReplaceAll(transcript, "\r\n", "\n"), "\n")
	segments := make([]transcriptSegment, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		startMs, endMs, text, ok := parseTimedTranscriptLine(line)
		if ok {
			segments = append(segments, transcriptSegment{
				StartMs:    startMs,
				EndMs:      endMs,
				Transcript: text,
			})
			continue
		}
		segments = append(segments, transcriptSegment{
			StartMs:    0,
			EndMs:      0,
			Transcript: line,
		})
	}
	if len(segments) == 0 {
		return nil
	}
	return segments
}

func parseTimedTranscriptLine(line string) (int, int, string, bool) {
	if !strings.HasPrefix(line, "[") {
		return 0, 0, "", false
	}
	firstEnd := strings.Index(line, "]")
	if firstEnd <= 1 {
		return 0, 0, "", false
	}
	startCode := strings.TrimSpace(line[1:firstEnd])
	rest := strings.TrimSpace(line[firstEnd+1:])
	if !strings.HasPrefix(rest, "-[") {
		return 0, 0, "", false
	}
	rest = strings.TrimPrefix(rest, "-[")
	secondEnd := strings.Index(rest, "]")
	if secondEnd <= 0 {
		return 0, 0, "", false
	}
	endCode := strings.TrimSpace(rest[:secondEnd])
	text := strings.TrimSpace(rest[secondEnd+1:])
	if text == "" {
		return 0, 0, "", false
	}
	startMs, ok := parseTimecodeToMilliseconds(startCode)
	if !ok {
		return 0, 0, "", false
	}
	endMs, ok := parseTimecodeToMilliseconds(endCode)
	if !ok {
		return 0, 0, "", false
	}
	return startMs, endMs, text, true
}

func parseTimecodeToMilliseconds(value string) (int, bool) {
	var hh, mm, ss, ff int
	if _, err := fmt.Sscanf(value, "%d:%d:%d:%d", &hh, &mm, &ss, &ff); err != nil {
		return 0, false
	}
	totalMs := (((hh*60)+mm)*60+ss)*1000 + ff*40
	return totalMs, true
}

func servicesQueueExtractPayload(asset services.Asset, taskID string) queue.AssetExtractFramesPayload {
	return queue.AssetExtractFramesPayload{
		TaskID:     taskID,
		AssetID:    asset.ID,
		StorageKey: asset.StorageKey,
		DurationMs: asset.DurationMs,
		Strategy: queue.FrameExtractionStrategy{
			Mode:       queue.FrameExtractionModeFixedInterval,
			FrameCount: suggestedFrameCountForDuration(asset.DurationMs),
		},
	}
}

func suggestedFrameCountForDuration(durationMs int) int {
	switch {
	case durationMs <= 0:
		return 1
	case durationMs <= 1500:
		return 1
	case durationMs <= 5000:
		return 3
	case durationMs <= 15000:
		return 5
	default:
		return 7
	}
}
