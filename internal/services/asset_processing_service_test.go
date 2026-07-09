package services

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
)

type stubAnalyzer struct {
	result modelgateway.AnalyzeAssetResult
	err    error
}

type stubAssetQueue struct {
	enqueuedAnalyze []queue.AssetAnalyzePayload
}

func (s stubAnalyzer) AnalyzeAsset(_ context.Context, _ modelgateway.AnalyzeAssetInput) (modelgateway.AnalyzeAssetResult, error) {
	if s.err != nil {
		return modelgateway.AnalyzeAssetResult{}, s.err
	}
	return s.result, nil
}

func (s *stubAssetQueue) EnqueueAssetAnalyze(taskID string, assetID string) error {
	s.enqueuedAnalyze = append(s.enqueuedAnalyze, queue.AssetAnalyzePayload{
		TaskID:  taskID,
		AssetID: assetID,
	})
	return nil
}

func TestBuildFrameTimestamps(t *testing.T) {
	tests := []struct {
		name       string
		duration   int
		frameCount int
		want       []int
	}{
		{name: "zero duration", duration: 0, frameCount: 3, want: []int{0}},
		{name: "single frame", duration: 900, frameCount: 1, want: []int{450}},
		{name: "three frames", duration: 1200, frameCount: 3, want: []int{300, 600, 900}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildFrameTimestamps(tt.duration, tt.frameCount)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestFrameCountForDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration int
		want     int
	}{
		{name: "zero duration", duration: 0, want: 1},
		{name: "very short clip", duration: 1200, want: 1},
		{name: "short clip", duration: 3000, want: 3},
		{name: "medium clip", duration: 9000, want: 5},
		{name: "long clip", duration: 20000, want: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frameCountForDuration(tt.duration); got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}

func TestHandleAssetAnalyzeUpdatesAsset(t *testing.T) {
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})
	asset, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "demo.mp4",
		StorageKey:        "assets/demo.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "pending_analysis",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		Width:             1080,
		Height:            1920,
		DurationMs:        1500,
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}

	processing := NewAssetProcessingService("", service, nil, nil, stubAnalyzer{
		result: modelgateway.AnalyzeAssetResult{
			UsabilityStatus:  "usable",
			SceneDescription: "product demo shot",
			ShotSize:         "close_up",
			CameraMovement:   "static",
			Subjects:         []string{"product"},
			SceneTags:        []string{"indoor"},
			QualityTags:      []string{},
			ModelResult:      map[string]any{"provider": "mock"},
		},
	}, nil)

	if err := processing.HandleAssetAnalyze(context.Background(), queue.AssetAnalyzePayload{TaskID: "", AssetID: asset.ID}); err != nil {
		t.Fatalf("handle asset analyze failed: %v", err)
	}

	updated, ok := service.GetAsset(asset.ID)
	if !ok {
		t.Fatalf("expected asset to exist")
	}
	if updated.AnalysisStatus != "ready" {
		t.Fatalf("expected analysis status ready, got %s", updated.AnalysisStatus)
	}
	if updated.SceneDescription != "product demo shot" {
		t.Fatalf("expected scene description to update, got %s", updated.SceneDescription)
	}
	if updated.AnalyzedAt == nil {
		t.Fatalf("expected analyzed_at to be set")
	}
}

func TestHandleAssetAnalyzeMarksFailure(t *testing.T) {
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})
	asset, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "demo.mp4",
		StorageKey:        "assets/demo.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "pending_analysis",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}

	processing := NewAssetProcessingService("", service, nil, nil, stubAnalyzer{
		err: errors.New("mock provider failed"),
	}, nil)

	if err := processing.HandleAssetAnalyze(context.Background(), queue.AssetAnalyzePayload{TaskID: "", AssetID: asset.ID}); err != nil {
		t.Fatalf("expected failure to be persisted without bubbling error, got %v", err)
	}

	updated, ok := service.GetAsset(asset.ID)
	if !ok {
		t.Fatalf("expected asset to exist")
	}
	if updated.AnalysisStatus != "failed" {
		t.Fatalf("expected analysis status failed, got %s", updated.AnalysisStatus)
	}
	if updated.AnalysisError != "mock provider failed" {
		t.Fatalf("expected analysis error to persist, got %s", updated.AnalysisError)
	}
}

func TestHandleAssetAnalyzeTracksTaskStatus(t *testing.T) {
	taskService := NewTaskService(t.TempDir())
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})
	asset, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "demo.mp4",
		StorageKey:        "assets/demo.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "pending_analysis",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		CreatedByUserID:   "user-1",
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}

	task, err := taskService.CreateAssetAnalyzeTask(context.Background(), "user-1", product.ID, queue.AssetAnalyzePayload{AssetID: asset.ID})
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	processing := NewAssetProcessingService("", service, taskService, &stubAssetQueue{}, stubAnalyzer{
		result: modelgateway.AnalyzeAssetResult{
			UsabilityStatus:  "usable",
			SceneDescription: "product demo shot",
		},
	}, nil)

	if err := processing.HandleAssetAnalyze(context.Background(), queue.AssetAnalyzePayload{TaskID: task.ID, AssetID: asset.ID}); err != nil {
		t.Fatalf("handle asset analyze failed: %v", err)
	}

	storedTask, err := taskService.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task failed: %v", err)
	}
	if storedTask.Status != "completed" {
		t.Fatalf("expected completed task, got %s", storedTask.Status)
	}
	if storedTask.AssetID != asset.ID {
		t.Fatalf("expected asset id %s, got %s", asset.ID, storedTask.AssetID)
	}
	if storedTask.StartedAt == nil || storedTask.FinishedAt == nil {
		t.Fatalf("expected task timing fields to be recorded")
	}
	if storedTask.DurationMs < 0 {
		t.Fatalf("expected non-negative task duration, got %d", storedTask.DurationMs)
	}
}

func TestHandleAssetExtractFramesMarksAssetFailure(t *testing.T) {
	taskService := NewTaskService(t.TempDir())
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})
	asset, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "missing.mp4",
		StorageKey:        "assets/missing.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "pending_analysis",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
		CreatedByUserID:   "user-1",
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}

	task, err := taskService.CreateAssetExtractFramesTask(context.Background(), "user-1", product.ID, queue.AssetExtractFramesPayload{
		AssetID:    asset.ID,
		StorageKey: asset.StorageKey,
		DurationMs: 1200,
	})
	if err != nil {
		t.Fatalf("create extract task failed: %v", err)
	}

	processing := NewAssetProcessingService(t.TempDir(), service, taskService, nil, nil, nil)

	if err := processing.HandleAssetExtractFrames(context.Background(), queue.AssetExtractFramesPayload{
		TaskID:     task.ID,
		AssetID:    asset.ID,
		StorageKey: asset.StorageKey,
		DurationMs: 1200,
	}); err != nil {
		t.Fatalf("expected extract failure to be persisted without bubbling error, got %v", err)
	}

	updated, ok := service.GetAsset(asset.ID)
	if !ok {
		t.Fatalf("expected asset to exist")
	}
	if updated.AnalysisStatus != "failed" {
		t.Fatalf("expected analysis status failed, got %s", updated.AnalysisStatus)
	}
	if updated.AnalysisError == "" {
		t.Fatalf("expected analysis error to persist")
	}
	if updated.UsabilityStatus != "needs_review" {
		t.Fatalf("expected usability status needs_review, got %s", updated.UsabilityStatus)
	}

	storedTask, err := taskService.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task failed: %v", err)
	}
	if storedTask.Status != "failed" {
		t.Fatalf("expected task status failed, got %s", storedTask.Status)
	}
	if storedTask.ErrorMessage == "" {
		t.Fatalf("expected task error message to be set")
	}
}

func TestRunTrackedTaskLogsTaskAndAssetIdentifiers(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	processing := NewAssetProcessingService("", nil, nil, nil, nil, logger)

	if err := processing.runTrackedTask(context.Background(), "task-1", "asset-1", "asset_analyze", func(context.Context) error {
		return nil
	}); err != nil {
		t.Fatalf("run tracked task failed: %v", err)
	}

	output := logs.String()
	if !strings.Contains(output, `"task_id":"task-1"`) {
		t.Fatalf("expected logs to include task_id, got %s", output)
	}
	if !strings.Contains(output, `"asset_id":"asset-1"`) {
		t.Fatalf("expected logs to include asset_id, got %s", output)
	}
	if !strings.Contains(output, `"duration_ms":`) {
		t.Fatalf("expected logs to include duration_ms, got %s", output)
	}
}
