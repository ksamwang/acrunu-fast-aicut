package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func TestHandleListTasksSupportsFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	taskService := services.NewTaskService(t.TempDir())
	if _, err := taskService.CreateTestTask(t.Context(), "user-1"); err != nil {
		t.Fatalf("create test task failed: %v", err)
	}
	if _, err := taskService.CreateAssetExtractFramesTask(t.Context(), "user-1", "product-1", queue.AssetExtractFramesPayload{
		AssetID:    "asset-1",
		StorageKey: "assets/a.mp4",
		DurationMs: 1200,
	}); err != nil {
		t.Fatalf("create extract task failed: %v", err)
	}
	analyzeTask, err := taskService.CreateAssetAnalyzeTask(t.Context(), "user-1", "product-1", queue.AssetAnalyzePayload{
		AssetID: "asset-1",
	})
	if err != nil {
		t.Fatalf("create analyze task failed: %v", err)
	}
	if err := taskService.MarkRunning(t.Context(), analyzeTask.ID); err != nil {
		t.Fatalf("mark analyze task running failed: %v", err)
	}
	if err := taskService.MarkFailed(t.Context(), analyzeTask.ID, "mock provider failed"); err != nil {
		t.Fatalf("mark analyze task failed: %v", err)
	}

	server := New(Options{
		Config:      config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		TaskService: taskService,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/tasks?task_type=asset_analyze&status=failed", nil)
	req.Header.Set("Authorization", "Bearer "+makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	}))
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Data []services.GenerationTask `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(response.Data) != 1 {
		t.Fatalf("expected 1 filtered task, got %d", len(response.Data))
	}
	if response.Data[0].TaskType != "asset_analyze" || response.Data[0].Status != "failed" {
		t.Fatalf("unexpected filtered task %+v", response.Data[0])
	}
	if response.Data[0].ErrorMessage != "mock provider failed" {
		t.Fatalf("expected failed error message, got %s", response.Data[0].ErrorMessage)
	}
}

func TestHandleGetTaskReturnsTaskDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	taskService := services.NewTaskService(t.TempDir())
	task, err := taskService.CreateAssetAnalyzeTask(t.Context(), "user-1", "product-1", queue.AssetAnalyzePayload{
		AssetID: "asset-1",
	})
	if err != nil {
		t.Fatalf("create analyze task failed: %v", err)
	}
	if err := taskService.MarkRunning(t.Context(), task.ID); err != nil {
		t.Fatalf("mark task running failed: %v", err)
	}
	if err := taskService.MarkFailed(t.Context(), task.ID, "mock provider failed"); err != nil {
		t.Fatalf("mark task failed failed: %v", err)
	}

	server := New(Options{
		Config:      config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		TaskService: taskService,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/"+task.ID, nil)
	req.Header.Set("Authorization", "Bearer "+makeDevToken(auth.User{
		ID:          "editor-1",
		Username:    "editor",
		DisplayName: "Editor",
		Role:        auth.RoleUser,
	}))
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Data services.GenerationTask `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.Data.TaskType != "asset_analyze" {
		t.Fatalf("expected task_type asset_analyze, got %s", response.Data.TaskType)
	}
	if response.Data.PayloadSummary["asset_id"] != "asset-1" {
		t.Fatalf("expected payload asset id, got %#v", response.Data.PayloadSummary)
	}
	if response.Data.ErrorMessage != "mock provider failed" {
		t.Fatalf("expected error message, got %s", response.Data.ErrorMessage)
	}
	if response.Data.RetryCount != 1 {
		t.Fatalf("expected retry_count 1, got %d", response.Data.RetryCount)
	}
	if response.Data.StartedAt == nil || response.Data.FinishedAt == nil {
		t.Fatalf("expected task timing fields, got %+v", response.Data)
	}
	if response.Data.DurationMs < 0 {
		t.Fatalf("expected non-negative duration, got %d", response.Data.DurationMs)
	}
}
