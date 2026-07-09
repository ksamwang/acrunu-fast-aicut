package httpserver

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func TestPhase2AssetLifecycleAPIIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tempDir := t.TempDir()
	productAssetService := services.NewProductAssetService()
	taskService := services.NewTaskService(tempDir)
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "P1"})
	sellingPointA, err := productAssetService.CreateSellingPoint(product.ID, services.CreateSellingPointInput{
		Title:    "Auto Wake",
		Priority: 1,
	})
	if err != nil {
		t.Fatalf("create selling point A failed: %v", err)
	}
	sellingPointB, err := productAssetService.CreateSellingPoint(product.ID, services.CreateSellingPointInput{
		Title:    "Battery Saver",
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create selling point B failed: %v", err)
	}

	server := New(Options{
		Config:              config.Config{StorageRoot: tempDir, QueueBackend: "file"},
		ProductAssetService: productAssetService,
		TaskService:         taskService,
	})

	ffprobeOutputPath := filepath.Join(tempDir, "ffprobe-output.json")
	ffprobeScriptPath := filepath.Join(tempDir, "ffprobe-mock.cmd")
	ffprobeOutput := `{"streams":[{"codec_type":"video","codec_name":"h264","width":1080,"height":1920,"avg_frame_rate":"30000/1001"},{"codec_type":"audio","codec_name":"aac","avg_frame_rate":"0/0"}],"format":{"duration":"2.066000","bit_rate":"3200000"}}`
	if err := os.WriteFile(ffprobeOutputPath, []byte(ffprobeOutput), 0644); err != nil {
		t.Fatalf("write ffprobe output failed: %v", err)
	}
	ffprobeScript := "@echo off\r\ntype \"" + ffprobeOutputPath + "\"\r\n"
	if err := os.WriteFile(ffprobeScriptPath, []byte(ffprobeScript), 0644); err != nil {
		t.Fatalf("write ffprobe mock failed: %v", err)
	}
	t.Setenv("FFPROBE_PATH", ffprobeScriptPath)

	userToken := "Bearer " + makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	})

	uploadTokenResp := performJSONRequest[services.UploadToken](t, server, http.MethodPost, "/api/uploads/tokens", userToken, map[string]any{
		"product_id": product.ID,
	})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	mustWriteFormField(t, writer, "source_type", "visual_only")
	mustWriteFormField(t, writer, "asset_name", "Demo Asset")
	mustWriteFormField(t, writer, "source_original_name", "demo-original.mp4")
	mustWriteFormField(t, writer, "reviewer_notes", "initial note")
	mustWriteFormField(t, writer, "selling_point_ids", sellingPointA.ID)
	part, err := writer.CreateFormFile("file", "demo.mp4")
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := part.Write([]byte("mock-video-content")); err != nil {
		t.Fatalf("write upload file content failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	uploadReq := httptest.NewRequest(http.MethodPost, "/api/uploads/clean-shot", &body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadReq.Header.Set("X-Upload-Token", uploadTokenResp.Token)
	uploadRecorder := httptest.NewRecorder()
	server.engine.ServeHTTP(uploadRecorder, uploadReq)

	if uploadRecorder.Code != http.StatusCreated {
		t.Fatalf("expected upload status 201, got %d, body=%s", uploadRecorder.Code, uploadRecorder.Body.String())
	}

	var uploadResp struct {
		Data struct {
			Asset       services.Asset `json:"asset"`
			IsDuplicate bool           `json:"is_duplicate"`
			FrameTaskID string         `json:"frame_task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(uploadRecorder.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("unmarshal upload response failed: %v", err)
	}
	if uploadResp.Data.Asset.ID == "" {
		t.Fatalf("expected asset id after upload")
	}
	if uploadResp.Data.IsDuplicate {
		t.Fatalf("expected first upload not duplicate")
	}
	if uploadResp.Data.FrameTaskID == "" {
		t.Fatalf("expected frame task id after successful upload")
	}
	if uploadResp.Data.Asset.Status != "ready" {
		t.Fatalf("expected asset status ready, got %s", uploadResp.Data.Asset.Status)
	}
	if uploadResp.Data.Asset.AnalysisStatus != "pending_analysis" {
		t.Fatalf("expected analysis status pending_analysis, got %s", uploadResp.Data.Asset.AnalysisStatus)
	}

	fullAssetPath := filepath.Join(tempDir, filepath.FromSlash(uploadResp.Data.Asset.StorageKey))
	if _, err := os.Stat(fullAssetPath); err != nil {
		t.Fatalf("expected uploaded file to exist at %s: %v", fullAssetPath, err)
	}

	assetListResp := performRequest[assetListResponse](t, server, http.MethodGet,
		"/api/assets?product_id="+product.ID+"&selling_point_id="+sellingPointA.ID+"&status=ready&page=1&page_size=10",
		userToken,
		nil,
	)
	if assetListResp.Total != 1 || len(assetListResp.Items) != 1 {
		t.Fatalf("expected one listed asset, got %+v", assetListResp)
	}

	assetDetail := performRequest[services.Asset](t, server, http.MethodGet, "/api/assets/"+uploadResp.Data.Asset.ID, userToken, nil)
	if assetDetail.AssetName != "Demo Asset" {
		t.Fatalf("expected asset name Demo Asset, got %s", assetDetail.AssetName)
	}
	if assetDetail.ReviewerNotes != "initial note" {
		t.Fatalf("expected initial reviewer note, got %s", assetDetail.ReviewerNotes)
	}
	if assetDetail.DurationMs != 2066 || assetDetail.Width != 1080 || assetDetail.Height != 1920 {
		t.Fatalf("expected probed media info persisted, got %+v", assetDetail)
	}

	initialSellingPoints := performRequest[[]services.SellingPoint](t, server, http.MethodGet, "/api/assets/"+uploadResp.Data.Asset.ID+"/selling-points", userToken, nil)
	if len(initialSellingPoints) != 1 || initialSellingPoints[0].ID != sellingPointA.ID {
		t.Fatalf("expected initial selling point A, got %#v", initialSellingPoints)
	}

	updatedReview := performJSONRequest[services.Asset](t, server, http.MethodPut, "/api/assets/"+uploadResp.Data.Asset.ID+"/review", userToken, map[string]any{
		"scene_description": "manual revised description",
		"shot_size":         "close_up",
		"camera_movement":   "static",
		"subjects":          []string{"product", "hand"},
		"scene_tags":        []string{"indoor"},
		"quality_tags":      []string{"soft_focus"},
		"usability_status":  "needs_review",
		"reviewer_notes":    "adjust crop",
	})
	if updatedReview.SceneDescription != "manual revised description" {
		t.Fatalf("expected updated scene description, got %s", updatedReview.SceneDescription)
	}
	if updatedReview.ReviewerNotes != "adjust crop" {
		t.Fatalf("expected updated reviewer notes, got %s", updatedReview.ReviewerNotes)
	}
	if updatedReview.UsabilityStatus != "needs_review" {
		t.Fatalf("expected updated usability status, got %s", updatedReview.UsabilityStatus)
	}

	updatedSellingPoints := performJSONRequest[[]services.SellingPoint](t, server, http.MethodPut, "/api/assets/"+uploadResp.Data.Asset.ID+"/selling-points", userToken, map[string]any{
		"selling_point_ids": []string{sellingPointB.ID},
	})
	if len(updatedSellingPoints) != 1 || updatedSellingPoints[0].ID != sellingPointB.ID {
		t.Fatalf("expected selling point B after update, got %#v", updatedSellingPoints)
	}

	reloadedSellingPoints := performRequest[[]services.SellingPoint](t, server, http.MethodGet, "/api/assets/"+uploadResp.Data.Asset.ID+"/selling-points", userToken, nil)
	if len(reloadedSellingPoints) != 1 || reloadedSellingPoints[0].ID != sellingPointB.ID {
		t.Fatalf("expected persisted selling point B, got %#v", reloadedSellingPoints)
	}

	archivedAsset := performRequest[services.Asset](t, server, http.MethodPost, "/api/assets/"+uploadResp.Data.Asset.ID+"/archive", userToken, nil)
	if archivedAsset.Status != "archived" || archivedAsset.ArchivedAt == nil {
		t.Fatalf("expected archived asset response, got %+v", archivedAsset)
	}

	restoredAsset := performRequest[services.Asset](t, server, http.MethodPost, "/api/assets/"+uploadResp.Data.Asset.ID+"/restore", userToken, nil)
	if restoredAsset.Status != "ready" || restoredAsset.ArchivedAt != nil {
		t.Fatalf("expected restored asset response, got %+v", restoredAsset)
	}

	taskDetail := performRequest[services.GenerationTask](t, server, http.MethodGet, "/api/tasks/"+uploadResp.Data.FrameTaskID, userToken, nil)
	if taskDetail.TaskType != "asset_extract_frames" {
		t.Fatalf("expected asset_extract_frames task, got %s", taskDetail.TaskType)
	}
	if taskDetail.Status != "queued" {
		t.Fatalf("expected queued frame task before worker consumption, got %s", taskDetail.Status)
	}
	if taskDetail.PayloadSummary["asset_id"] != uploadResp.Data.Asset.ID {
		t.Fatalf("expected payload asset id %s, got %#v", uploadResp.Data.Asset.ID, taskDetail.PayloadSummary)
	}
	if taskDetail.PayloadSummary["storage_key"] != uploadResp.Data.Asset.StorageKey {
		t.Fatalf("expected payload storage key %s, got %#v", uploadResp.Data.Asset.StorageKey, taskDetail.PayloadSummary)
	}

	filteredTasks := performRequest[[]services.GenerationTask](t, server, http.MethodGet, "/api/tasks?task_type=asset_extract_frames&status=queued", userToken, nil)
	if len(filteredTasks) != 1 || filteredTasks[0].ID != uploadResp.Data.FrameTaskID {
		t.Fatalf("expected one queued frame task, got %#v", filteredTasks)
	}
}

func performJSONRequest[T any](t *testing.T, server *Server, method string, path string, authHeader string, body any) T {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body failed: %v", err)
	}
	return performRequest[T](t, server, method, path, authHeader, bytes.NewReader(raw))
}

func performRequest[T any](t *testing.T, server *Server, method string, path string, authHeader string, body *bytes.Reader) T {
	t.Helper()

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = body
	}

	req := httptest.NewRequest(method, path, reader)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 200/201 for %s %s, got %d, body=%s", method, path, recorder.Code, recorder.Body.String())
	}

	var response struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response failed for %s %s: %v; body=%s", method, path, err, recorder.Body.String())
	}
	return response.Data
}

func mustWriteFormField(t *testing.T, writer *multipart.Writer, key string, value string) {
	t.Helper()
	if err := writer.WriteField(key, value); err != nil {
		t.Fatalf("write form field %s failed: %v", key, err)
	}
}

func TestPhase2AssetLifecycleAPIIntegrationRejectsMissingAuthOnProtectedEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := New(Options{
		Config: config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/assets", nil)
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized assets request, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "unauthorized") {
		t.Fatalf("expected unauthorized error response, got %s", recorder.Body.String())
	}
}
