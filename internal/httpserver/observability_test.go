package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/config"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func TestLoggingMiddlewareWritesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	server := New(Options{
		Config: config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		Logger: logger,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	req.Header.Set(requestIDHeader, "req-123")
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if recorder.Header().Get(requestIDHeader) != "req-123" {
		t.Fatalf("expected echoed request id header, got %q", recorder.Header().Get(requestIDHeader))
	}
	if !strings.Contains(logs.String(), `"request_id":"req-123"`) {
		t.Fatalf("expected request log to include request_id, got %s", logs.String())
	}
}

func TestHandleMetricsReturnsPrometheusPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	taskService := services.NewTaskService(t.TempDir())
	if _, err := taskService.CreateAssetExtractFramesTask(t.Context(), "user-1", "product-1", queue.AssetExtractFramesPayload{
		AssetID:    "asset-1",
		StorageKey: "assets/a.mp4",
		DurationMs: 1200,
	}); err != nil {
		t.Fatalf("create extract task failed: %v", err)
	}
	analyzeTask, err := taskService.CreateAssetAnalyzeTask(t.Context(), "user-1", "product-1", queue.AssetAnalyzePayload{
		AssetID: "asset-2",
	})
	if err != nil {
		t.Fatalf("create analyze task failed: %v", err)
	}
	if err := taskService.MarkRunning(t.Context(), analyzeTask.ID); err != nil {
		t.Fatalf("mark running failed: %v", err)
	}
	if err := taskService.MarkFailed(t.Context(), analyzeTask.ID, "mock provider failed"); err != nil {
		t.Fatalf("mark failed failed: %v", err)
	}

	server := New(Options{
		Config:      config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		TaskService: taskService,
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("expected text/plain metrics response, got %q", contentType)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, `aicut_http_metrics_enabled{component="api"} 1`) {
		t.Fatalf("expected metrics enabled gauge, got %s", body)
	}
	if !strings.Contains(body, `aicut_generation_tasks_total{task_type="asset_analyze",status="failed"} 1`) {
		t.Fatalf("expected failed analyze task metric, got %s", body)
	}
	if !strings.Contains(body, `aicut_generation_tasks_total{task_type="asset_extract_frames",status="queued"} 1`) {
		t.Fatalf("expected queued extract task metric, got %s", body)
	}
}

func TestRequestIDHeaderGeneratedWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := New(Options{
		Config: config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if recorder.Header().Get(requestIDHeader) == "" {
		t.Fatalf("expected generated request id header")
	}
}

func TestMetricsErrorsWhenTaskStoreFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := New(Options{
		Config:      config.Config{StorageRoot: t.TempDir(), QueueBackend: "file"},
		TaskService: services.NewTaskServiceWithStore(failingTaskStore{}),
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", recorder.Code)
	}

	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.Error.Code != "metrics_error" {
		t.Fatalf("expected metrics_error, got %+v", response.Error)
	}
}

type failingTaskStore struct{}

func (failingTaskStore) CreateTestTask(_ context.Context, _ string) (services.GenerationTask, error) {
	return services.GenerationTask{}, nil
}

func (failingTaskStore) CreateAssetExtractFramesTask(_ context.Context, _ string, _ string, _ queue.AssetExtractFramesPayload) (services.GenerationTask, error) {
	return services.GenerationTask{}, nil
}

func (failingTaskStore) CreateAssetAnalyzeTask(_ context.Context, _ string, _ string, _ queue.AssetAnalyzePayload) (services.GenerationTask, error) {
	return services.GenerationTask{}, nil
}

func (failingTaskStore) GetTask(_ context.Context, _ string) (services.GenerationTask, error) {
	return services.GenerationTask{}, nil
}

func (failingTaskStore) ListTasks(_ context.Context) ([]services.GenerationTask, error) {
	return nil, errFailingTaskStore
}

func (failingTaskStore) MarkRunning(_ context.Context, _ string) error {
	return nil
}

func (failingTaskStore) MarkCompleted(_ context.Context, _ string) error {
	return nil
}

func (failingTaskStore) MarkFailed(_ context.Context, _ string, _ string) error {
	return nil
}

var errFailingTaskStore = errors.New("task store failed")
