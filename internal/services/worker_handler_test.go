package services

import (
	"context"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
)

func TestWorkerHandlerHandleTestTaskDelegatesToTaskService(t *testing.T) {
	taskService := NewTaskService(t.TempDir())
	task, err := taskService.CreateTestTask(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("create test task failed: %v", err)
	}

	handler := NewWorkerHandler(taskService, nil)
	if err := handler.HandleTestTask(context.Background(), queue.TestTaskPayload{TaskID: task.ID}); err != nil {
		t.Fatalf("handle test task failed: %v", err)
	}

	storedTask, err := taskService.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task failed: %v", err)
	}
	if storedTask.Status != "completed" {
		t.Fatalf("expected completed task, got %s", storedTask.Status)
	}
}

func TestWorkerHandlerHandleAssetAnalyzeDelegatesToAssetProcessingService(t *testing.T) {
	productService := NewProductAssetService()
	product := productService.CreateProduct(CreateProductInput{Name: "P1"})
	asset, err := productService.CreateAsset(CreateAssetInput{
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

	processing := NewAssetProcessingService("", productService, nil, nil, stubAnalyzer{
		result: modelgateway.AnalyzeAssetResult{
			UsabilityStatus:  "usable",
			SceneDescription: "delegated result",
		},
	}, nil)

	handler := NewWorkerHandler(nil, processing)
	if err := handler.HandleAssetAnalyze(context.Background(), queue.AssetAnalyzePayload{AssetID: asset.ID}); err != nil {
		t.Fatalf("handle asset analyze failed: %v", err)
	}

	updated, ok := productService.GetAsset(asset.ID)
	if !ok {
		t.Fatalf("expected asset to exist")
	}
	if updated.SceneDescription != "delegated result" {
		t.Fatalf("expected delegated analysis result, got %s", updated.SceneDescription)
	}
}

func TestWorkerHandlerAssetMethodsAllowNilProcessingService(t *testing.T) {
	handler := NewWorkerHandler(nil, nil)

	if err := handler.HandleAssetExtractFrames(context.Background(), queue.AssetExtractFramesPayload{AssetID: "asset-1"}); err != nil {
		t.Fatalf("expected nil processing service to be ignored for extract frames, got %v", err)
	}
	if err := handler.HandleAssetAnalyze(context.Background(), queue.AssetAnalyzePayload{AssetID: "asset-1"}); err != nil {
		t.Fatalf("expected nil processing service to be ignored for analyze, got %v", err)
	}
	if err := handler.HandleAssetEmbedding(context.Background(), queue.AssetEmbeddingPayload{AssetID: "asset-1"}); err != nil {
		t.Fatalf("expected nil processing service to be ignored for embedding, got %v", err)
	}
}

func TestWorkerHandlerIgnoresSupersededEditPlanTask(t *testing.T) {
	ctx := context.Background()
	taskService := NewTaskService(t.TempDir())
	runs := NewGenerationRunService(nil)
	run, err := runs.Create(ctx, CreateGenerationRunInput{ProductID: "product-1"})
	if err != nil {
		t.Fatalf("create generation run: %v", err)
	}
	currentTask, err := taskService.CreateEditPlanGenerateTask(ctx, "user-1", "product-1", queue.EditPlanGeneratePayload{GenerationRunID: run.ID})
	if err != nil {
		t.Fatalf("create current edit plan task: %v", err)
	}
	if err := runs.LinkTask(ctx, run.ID, currentTask.ID, generationRunTaskStageEditPlan); err != nil {
		t.Fatalf("link current edit plan task: %v", err)
	}
	staleTask, err := taskService.CreateEditPlanGenerateTask(ctx, "user-1", "product-1", queue.EditPlanGeneratePayload{GenerationRunID: run.ID})
	if err != nil {
		t.Fatalf("create stale edit plan task: %v", err)
	}

	handler := NewWorkerHandler(taskService, nil).WithGenerationPipeline(runs, nil, nil)
	if err := handler.HandleEditPlanGenerate(ctx, queue.EditPlanGeneratePayload{
		TaskID: staleTask.ID, GenerationRunID: run.ID, ScriptVariantID: "stale-script", VoiceoverID: "stale-voiceover",
	}); err != nil {
		t.Fatalf("superseded task should be acknowledged without planning: %v", err)
	}
	storedTask, err := taskService.GetTask(ctx, staleTask.ID)
	if err != nil {
		t.Fatalf("get stale edit plan task: %v", err)
	}
	if storedTask.Status != "completed" {
		t.Fatalf("expected superseded task to be completed, got %q", storedTask.Status)
	}
	storedRun, err := runs.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("get generation run: %v", err)
	}
	if storedRun.Status == generationRunStatusFailed {
		t.Fatalf("superseded task must not fail the current generation run: %#v", storedRun)
	}
}
