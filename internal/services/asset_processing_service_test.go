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
	result   modelgateway.AnalyzeAssetResult
	err      error
	captured *modelgateway.AnalyzeAssetInput
}

type stubAssetQueue struct {
	enqueuedAnalyze   []queue.AssetAnalyzePayload
	enqueuedEmbedding []queue.AssetEmbeddingPayload
}

type stubAssetVectorizer struct {
	assetIDs []string
	err      error
}

func (s *stubAssetVectorizer) VectorizeAsset(_ context.Context, assetID string) (AssetEmbeddingRunResult, error) {
	s.assetIDs = append(s.assetIDs, assetID)
	return AssetEmbeddingRunResult{AssetID: assetID}, s.err
}

func (s *stubAssetQueue) EnqueueAssetEmbedding(payload queue.AssetEmbeddingPayload) error {
	s.enqueuedEmbedding = append(s.enqueuedEmbedding, payload)
	return nil
}

func (s stubAnalyzer) AnalyzeAsset(_ context.Context, input modelgateway.AnalyzeAssetInput) (modelgateway.AnalyzeAssetResult, error) {
	if s.captured != nil {
		*s.captured = input
	}
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

func TestNormalizeFrameExtractionStrategy(t *testing.T) {
	tests := []struct {
		name     string
		duration int
		input    queue.FrameExtractionStrategy
		want     queue.FrameExtractionStrategy
	}{
		{
			name:     "default strategy from duration",
			duration: 9000,
			input:    queue.FrameExtractionStrategy{},
			want: queue.FrameExtractionStrategy{
				Mode:       queue.FrameExtractionModeFixedInterval,
				FrameCount: 5,
			},
		},
		{
			name:     "fixed interval preserves explicit frame count",
			duration: 9000,
			input:    queue.FrameExtractionStrategy{Mode: queue.FrameExtractionModeFixedInterval, FrameCount: 4},
			want: queue.FrameExtractionStrategy{
				Mode:       queue.FrameExtractionModeFixedInterval,
				FrameCount: 4,
			},
		},
		{
			name:     "keyframe uses fallback placeholders",
			duration: 9000,
			input:    queue.FrameExtractionStrategy{Mode: queue.FrameExtractionModeKeyframe},
			want: queue.FrameExtractionStrategy{
				Mode:             queue.FrameExtractionModeKeyframe,
				FrameCount:       keyframeFallbackFrameCount,
				KeyframeWindowMs: keyframeWindowMs,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeFrameExtractionStrategy(tt.duration, tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected %+v, got %+v", tt.want, got)
			}
		})
	}
}

func TestAssetProcessingVLMConcurrencyUsesRuntimeSetting(t *testing.T) {
	settings := NewSystemConfigService()
	if _, err := settings.Upsert(SystemConfig{Key: "vlm.max_concurrency", Value: 7, Type: "number"}); err != nil {
		t.Fatalf("set VLM concurrency failed: %v", err)
	}
	processing := NewAssetProcessingService("", nil, nil, nil, nil, nil).WithRuntimeSettings(settings)
	if got := processing.vlmMaxConcurrency(context.Background()); got != 7 {
		t.Fatalf("expected runtime VLM concurrency 7, got %d", got)
	}
}

func TestHandleAssetAnalyzeUpdatesAsset(t *testing.T) {
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{
		Name:     "P1",
		Metadata: map[string]any{"reference_image": "data:image/png;base64,cmVm"},
	})
	if _, err := service.CreateSellingPoint(product.ID, CreateSellingPointInput{Title: "解决骑行痛点", Description: "避免裤脚靠近链条"}); err != nil {
		t.Fatalf("create selling point failed: %v", err)
	}
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

	var analyzerInput modelgateway.AnalyzeAssetInput
	processing := NewAssetProcessingService("", service, nil, nil, stubAnalyzer{
		captured: &analyzerInput,
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
	if analyzerInput.ProductName != product.Name {
		t.Fatalf("expected analyzer input to include product name %q, got %q", product.Name, analyzerInput.ProductName)
	}
	if analyzerInput.ProductID != product.ID {
		t.Fatalf("expected analyzer input product id %s, got %s", product.ID, analyzerInput.ProductID)
	}
	if len(analyzerInput.CandidateSellingPoints) != 1 || analyzerInput.CandidateSellingPoints[0].Title != "解决骑行痛点" {
		t.Fatalf("expected active selling point context, got %#v", analyzerInput.CandidateSellingPoints)
	}
	if analyzerInput.ProductReferenceImage == nil || analyzerInput.ProductReferenceImage.DataURL == "" {
		t.Fatalf("expected product reference image data URL, got %#v", analyzerInput.ProductReferenceImage)
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

	if err := processing.HandleAssetAnalyze(context.Background(), queue.AssetAnalyzePayload{TaskID: "", AssetID: asset.ID}); err == nil {
		t.Fatalf("expected failure to bubble for retry handling")
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

func TestHandleAssetAnalyzeQueuesEmbeddingBeforeMarkingAssetReady(t *testing.T) {
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

	assetQueue := &stubAssetQueue{}
	vectorizer := &stubAssetVectorizer{}
	processing := NewAssetProcessingService("", service, taskService, assetQueue, stubAnalyzer{
		result: modelgateway.AnalyzeAssetResult{
			UsabilityStatus:   "usable",
			SceneDescription:  "产品完成固定",
			ActionDescription: "手将产品粘合固定",
			ShotSize:          "close_up",
			CameraMovement:    "static",
			VisualTags:        []string{"产品固定"},
			SceneContext:      "使用场景",
			ProductPosition:   "裤脚处",
			VisibleProduct:    true,
		},
	}, nil).WithAssetEmbeddingService(vectorizer)

	analyzeTask, err := taskService.CreateAssetAnalyzeTask(context.Background(), "user-1", product.ID, queue.AssetAnalyzePayload{AssetID: asset.ID})
	if err != nil {
		t.Fatalf("create analyze task failed: %v", err)
	}
	if err := processing.HandleAssetAnalyze(context.Background(), queue.AssetAnalyzePayload{TaskID: analyzeTask.ID, AssetID: asset.ID}); err != nil {
		t.Fatalf("analyze asset failed: %v", err)
	}
	queuedAsset, ok := service.GetAsset(asset.ID)
	if !ok || queuedAsset.AnalysisStatus != "analyzing" {
		t.Fatalf("expected asset to remain analyzing until embedding completes, got %#v", queuedAsset)
	}
	if len(assetQueue.enqueuedEmbedding) != 1 || assetQueue.enqueuedEmbedding[0].AssetID != asset.ID {
		t.Fatalf("expected one embedding task, got %#v", assetQueue.enqueuedEmbedding)
	}

	embeddingPayload := assetQueue.enqueuedEmbedding[0]
	if err := processing.HandleAssetEmbedding(context.Background(), embeddingPayload); err != nil {
		t.Fatalf("embed asset failed: %v", err)
	}
	readyAsset, ok := service.GetAsset(asset.ID)
	if !ok || readyAsset.AnalysisStatus != "ready" || readyAsset.AnalysisError != "" {
		t.Fatalf("expected asset ready after embedding, got %#v", readyAsset)
	}
	if len(vectorizer.assetIDs) != 1 || vectorizer.assetIDs[0] != asset.ID {
		t.Fatalf("expected vectorizer to receive asset, got %#v", vectorizer.assetIDs)
	}
}

func TestHandleAssetEmbeddingCannotCompleteSupersededAnalysis(t *testing.T) {
	service := NewProductAssetService()
	product := service.CreateProduct(CreateProductInput{Name: "P1"})
	asset, err := service.CreateAsset(CreateAssetInput{
		ProductID:         product.ID,
		FileName:          "demo.mp4",
		StorageKey:        "assets/demo.mp4",
		SourceType:        "visual_only",
		Status:            "ready",
		AnalysisStatus:    "analyzing",
		UsabilityStatus:   "usable",
		ManualCleanStatus: "cleaned",
	})
	if err != nil {
		t.Fatalf("create asset failed: %v", err)
	}
	if err := service.UpdateAssetAnalysis(asset.ID, AssetAnalysisUpdate{
		AnalysisStatus: "analyzing",
		ModelLabels:    map[string]any{"scene_description": "new analysis"},
		ModelResult:    map[string]any{"analysis_task_id": "analysis-new"},
	}); err != nil {
		t.Fatalf("seed current analysis failed: %v", err)
	}

	vectorizer := &stubAssetVectorizer{}
	processing := NewAssetProcessingService("", service, nil, nil, nil, nil).WithAssetEmbeddingService(vectorizer)
	if err := processing.HandleAssetEmbedding(context.Background(), queue.AssetEmbeddingPayload{
		TaskID:         "embedding-old",
		AssetID:        asset.ID,
		AnalysisTaskID: "analysis-old",
	}); err != nil {
		t.Fatalf("superseded embedding should be skipped cleanly: %v", err)
	}
	updated, ok := service.GetAsset(asset.ID)
	if !ok || updated.AnalysisStatus != "analyzing" {
		t.Fatalf("expected current analysis to remain in progress, got %#v", updated)
	}
	if len(vectorizer.assetIDs) != 0 {
		t.Fatalf("expected superseded embedding not to run, got %#v", vectorizer.assetIDs)
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
	}); err == nil {
		t.Fatalf("expected extract failure to bubble for retry handling")
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

func TestRunTrackedTaskSkipsCompletedTask(t *testing.T) {
	taskService := NewTaskService(t.TempDir())
	task, err := taskService.CreateTestTask(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	if err := taskService.MarkCompleted(context.Background(), task.ID); err != nil {
		t.Fatalf("mark completed failed: %v", err)
	}

	processing := NewAssetProcessingService("", nil, taskService, nil, nil, nil)
	called := false
	if err := processing.runTrackedTask(context.Background(), task.ID, "asset-1", "asset_extract_frames", func(context.Context) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("run tracked task failed: %v", err)
	}
	if called {
		t.Fatalf("expected completed task to be skipped")
	}
}
