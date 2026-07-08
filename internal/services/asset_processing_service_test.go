package services

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
	"github.com/ksamwang/acrunu-fast-aicut/internal/queue"
)

type stubAnalyzer struct {
	result modelgateway.AnalyzeAssetResult
	err    error
}

func (s stubAnalyzer) AnalyzeAsset(_ context.Context, _ modelgateway.AnalyzeAssetInput) (modelgateway.AnalyzeAssetResult, error) {
	if s.err != nil {
		return modelgateway.AnalyzeAssetResult{}, s.err
	}
	return s.result, nil
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

	processing := NewAssetProcessingService("", service, nil, stubAnalyzer{
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
	})

	if err := processing.HandleAssetAnalyze(context.Background(), queue.AssetAnalyzePayload{AssetID: asset.ID}); err != nil {
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

	processing := NewAssetProcessingService("", service, nil, stubAnalyzer{
		err: errors.New("mock provider failed"),
	})

	if err := processing.HandleAssetAnalyze(context.Background(), queue.AssetAnalyzePayload{AssetID: asset.ID}); err != nil {
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
