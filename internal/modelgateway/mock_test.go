package modelgateway

import (
	"context"
	"testing"
)

func TestMockAssetAnalyzerAnalyzeAsset(t *testing.T) {
	analyzer := NewMockAssetAnalyzer()

	result, err := analyzer.AnalyzeAsset(context.Background(), AnalyzeAssetInput{
		AssetID:    "asset-1",
		SourceType: "talking_head",
		DurationMs: 9000,
		Width:      1080,
		Height:     1920,
		HasAudio:   false,
		FrameSnapshots: []FrameReference{
			{FrameIndex: 0, TimestampMs: 500, StorageKey: "frames/asset-1/frame_000.jpg"},
			{FrameIndex: 1, TimestampMs: 1500, StorageKey: "frames/asset-1/frame_001.jpg"},
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if result.SceneDescription == "" {
		t.Fatalf("expected scene description")
	}
	if result.UsabilityStatus != "needs_review" {
		t.Fatalf("expected needs_review, got %s", result.UsabilityStatus)
	}
	if result.CameraMovement != "slow_push_in" {
		t.Fatalf("expected slow_push_in, got %s", result.CameraMovement)
	}
	if len(result.Subjects) == 0 {
		t.Fatalf("expected subjects")
	}
	if provider, ok := result.ModelResult["provider"].(string); !ok || provider != "mock" {
		t.Fatalf("expected mock provider, got %+v", result.ModelResult)
	}
}
