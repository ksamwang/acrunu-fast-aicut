package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func TestPhase2VLMMockAnalysisIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tempDir := t.TempDir()
	productAssetService := services.NewProductAssetService()
	taskService := services.NewTaskService(tempDir)
	product := productAssetService.CreateProduct(services.CreateProductInput{Name: "P1"})

	server := New(Options{
		Config:              config.Config{StorageRoot: tempDir, QueueBackend: "file"},
		ProductAssetService: productAssetService,
		TaskService:         taskService,
	})

	ffprobeOutputPath := filepath.Join(tempDir, "ffprobe-output.json")
	ffprobeScriptPath := filepath.Join(tempDir, "ffprobe-mock.cmd")
	ffmpegScriptPath := filepath.Join(tempDir, "ffmpeg-mock.cmd")

	ffprobeOutput := `{"streams":[{"codec_type":"video","codec_name":"h264","width":1080,"height":1920,"avg_frame_rate":"30000/1001"}],"format":{"duration":"2.066000","bit_rate":"3200000"}}`
	if err := os.WriteFile(ffprobeOutputPath, []byte(ffprobeOutput), 0644); err != nil {
		t.Fatalf("write ffprobe output failed: %v", err)
	}
	ffprobeScript := "@echo off\r\ntype \"" + ffprobeOutputPath + "\"\r\n"
	if err := os.WriteFile(ffprobeScriptPath, []byte(ffprobeScript), 0644); err != nil {
		t.Fatalf("write ffprobe mock failed: %v", err)
	}
	ffmpegScript := "@echo off\r\nset output=\r\n:loop\r\nif \"%~1\"==\"\" goto done\r\nset output=%~1\r\nshift\r\ngoto loop\r\n:done\r\nbreak>\"%output%\"\r\n"
	if err := os.WriteFile(ffmpegScriptPath, []byte(ffmpegScript), 0644); err != nil {
		t.Fatalf("write ffmpeg mock failed: %v", err)
	}
	t.Setenv("FFPROBE_PATH", ffprobeScriptPath)
	t.Setenv("FFMPEG_PATH", ffmpegScriptPath)

	assetProcessingService := services.NewAssetProcessingService(tempDir, productAssetService, taskService, server.queueClient, nil, nil)
	workerHandler := services.NewWorkerHandler(taskService, assetProcessingService)
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()

	workerDone := make(chan error, 1)
	go func() {
		workerDone <- queue.RunFileWorker(workerCtx, tempDir, workerHandler, 10*time.Millisecond)
	}()

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
	mustWriteFormField(t, writer, "source_type", "talking_head")
	mustWriteFormField(t, writer, "asset_name", "Talking Head Demo")
	part, err := writer.CreateFormFile("file", "talking-head.mp4")
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := part.Write([]byte("mock-talking-head-video")); err != nil {
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
			FrameTaskID string         `json:"frame_task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(uploadRecorder.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("unmarshal upload response failed: %v", err)
	}
	if uploadResp.Data.Asset.ID == "" {
		t.Fatalf("expected asset id after upload")
	}
	if uploadResp.Data.FrameTaskID == "" {
		t.Fatalf("expected frame task id after upload")
	}

	assetID := uploadResp.Data.Asset.ID
	analyzeTaskID := waitForAnalyzeTaskCompletion(t, taskService, assetID, 3*time.Second)

	cancelWorker()
	select {
	case err := <-workerDone:
		if err != nil && err != context.Canceled {
			t.Fatalf("worker returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("worker did not stop after cancellation")
	}

	asset, ok := productAssetService.GetAsset(assetID)
	if !ok {
		t.Fatalf("expected asset to exist after worker processing")
	}
	if asset.AnalysisStatus != "ready" {
		t.Fatalf("expected analysis status ready, got %s", asset.AnalysisStatus)
	}
	if asset.SceneDescription != "presenter delivers product message to camera" {
		t.Fatalf("unexpected scene description: %s", asset.SceneDescription)
	}
	if asset.ShotSize != "medium_close_up" {
		t.Fatalf("expected medium_close_up shot size, got %s", asset.ShotSize)
	}
	if asset.CameraMovement != "static" {
		t.Fatalf("expected static camera movement, got %s", asset.CameraMovement)
	}
	if asset.UsabilityStatus != "needs_review" {
		t.Fatalf("expected needs_review usability, got %s", asset.UsabilityStatus)
	}
	if len(asset.QualityTags) != 1 || asset.QualityTags[0] != "missing_expected_audio" {
		t.Fatalf("expected missing_expected_audio quality tag, got %#v", asset.QualityTags)
	}
	if provider, ok := asset.ModelResult["provider"].(string); !ok || provider != "mock" {
		t.Fatalf("expected mock provider in model result, got %#v", asset.ModelResult)
	}
	if sourceType, ok := asset.ModelResult["source_type"].(string); !ok || sourceType != "talking_head" {
		t.Fatalf("expected talking_head source_type in model result, got %#v", asset.ModelResult)
	}
	if asset.AnalyzedAt == nil {
		t.Fatalf("expected analyzed_at to be set")
	}
	for i := 0; i < 3; i++ {
		framePath := filepath.Join(tempDir, "frames", assetID, fmt.Sprintf("frame_%03d.jpg", i))
		if _, err := os.Stat(framePath); err != nil {
			t.Fatalf("expected extracted frame file %s: %v", framePath, err)
		}
	}

	frameTask, err := taskService.GetTask(context.Background(), uploadResp.Data.FrameTaskID)
	if err != nil {
		t.Fatalf("get frame task failed: %v", err)
	}
	if frameTask.Status != "completed" {
		t.Fatalf("expected completed frame task, got %s", frameTask.Status)
	}
	if frameTask.StartedAt == nil || frameTask.FinishedAt == nil {
		t.Fatalf("expected frame task timing fields")
	}

	analyzeTask, err := taskService.GetTask(context.Background(), analyzeTaskID)
	if err != nil {
		t.Fatalf("get analyze task failed: %v", err)
	}
	if analyzeTask.Status != "completed" {
		t.Fatalf("expected completed analyze task, got %s", analyzeTask.Status)
	}
	if analyzeTask.AssetID != assetID {
		t.Fatalf("expected analyze task asset id %s, got %s", assetID, analyzeTask.AssetID)
	}
	if analyzeTask.StartedAt == nil || analyzeTask.FinishedAt == nil {
		t.Fatalf("expected analyze task timing fields")
	}

	filteredTasks := performRequest[[]services.GenerationTask](t, server, http.MethodGet, "/api/tasks?task_type=asset_analyze&status=completed", userToken, nil)
	if len(filteredTasks) != 1 || filteredTasks[0].ID != analyzeTaskID {
		t.Fatalf("expected completed analyze task in filtered API response, got %#v", filteredTasks)
	}

	assetDetail := performRequest[services.Asset](t, server, http.MethodGet, "/api/assets/"+assetID, userToken, nil)
	if assetDetail.SceneDescription != asset.SceneDescription {
		t.Fatalf("expected API asset detail to expose analysis result, got %+v", assetDetail)
	}
}

func waitForAnalyzeTaskCompletion(t *testing.T, taskService *services.TaskService, assetID string, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		tasks, err := taskService.ListTasks(context.Background(), services.TaskFilters{TaskType: "asset_analyze"})
		if err != nil {
			t.Fatalf("list analyze tasks failed: %v", err)
		}
		for _, task := range tasks {
			if task.AssetID == assetID && task.Status == "completed" {
				return task.ID
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for completed analyze task for asset %s", assetID)
	return ""
}
