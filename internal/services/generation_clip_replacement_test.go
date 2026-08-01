package services

import (
	"context"
	"errors"
	"testing"
)

func TestMaterializeClipReplacementPlanKeepsTimelineAndRejectsReuse(t *testing.T) {
	assets := NewProductAssetService()
	product := assets.CreateProduct(CreateProductInput{Name: "束裤带"})
	first := mustCreateReplacementAsset(t, assets, product.ID, "first.mp4", 2500)
	second := mustCreateReplacementAsset(t, assets, product.ID, "second.mp4", 3200)
	third := mustCreateReplacementAsset(t, assets, product.ID, "third.mp4", 2400)
	plan := replacementTestPlan(product.ID, first.ID, second.ID)

	replaced, err := MaterializeClipReplacementPlan(GenerationRun{ProductID: product.ID}, plan, []EditPlanClipReplacement{{
		ClipID: plan.Clips[0].ID, AssetID: third.ID, SourceInMs: 300,
	}}, assets)
	if err != nil {
		t.Fatalf("materialize replacement: %v", err)
	}
	clip := replaced.Clips[0]
	if clip.AssetID != third.ID || clip.SourceInMs != 300 || clip.SourceOutMs != 1300 || clip.StartMs != 0 || clip.EndMs != 1000 {
		t.Fatalf("unexpected replaced clip %#v", clip)
	}
	if _, err := MaterializeClipReplacementPlan(GenerationRun{ProductID: product.ID}, plan, []EditPlanClipReplacement{{
		ClipID: plan.Clips[0].ID, AssetID: second.ID, SourceInMs: 0,
	}}, assets); err == nil || !errors.Is(err, ErrClipReplacementInvalid) {
		t.Fatalf("expected duplicate material to be rejected, got %v", err)
	}
}

func TestClipReplacementRenderStatePreservesOldOutputUntilCommit(t *testing.T) {
	ctx := context.Background()
	assets := NewProductAssetService()
	product := assets.CreateProduct(CreateProductInput{Name: "束裤带"})
	first := mustCreateReplacementAsset(t, assets, product.ID, "first.mp4", 2500)
	second := mustCreateReplacementAsset(t, assets, product.ID, "second.mp4", 3200)
	third := mustCreateReplacementAsset(t, assets, product.ID, "third.mp4", 2400)
	runs := NewGenerationRunService(nil)
	run, err := runs.Create(ctx, CreateGenerationRunInput{ProductID: product.ID})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	plan := replacementTestPlan(run.ID, first.ID, second.ID)
	stored, err := runs.SaveEditPlan(ctx, plan)
	if err != nil {
		t.Fatalf("save plan: %v", err)
	}
	oldOutput := GenerationRenderOutput{
		StorageKey: "renders/old.mp4", MimeType: "video/mp4", DurationMs: 2000,
		Width: 1080, Height: 1920, FileSizeBytes: 1024, Renderer: "ffmpeg", RenderVersion: "v1",
	}
	if err := runs.MarkRenderCompleted(ctx, run.ID, oldOutput); err != nil {
		t.Fatalf("complete original render: %v", err)
	}
	if _, err := runs.PrepareClipReplacementRender(ctx, run.ID, stored.UpdatedAt); err != nil {
		t.Fatalf("prepare replacement render: %v", err)
	}
	preparing, _ := runs.Get(ctx, run.ID)
	if preparing.Status != generationRunStatusGenerating || preparing.OutputStorageKey != oldOutput.StorageKey {
		t.Fatalf("old output was not preserved while rendering %#v", preparing)
	}

	replacedPlan, err := MaterializeClipReplacementPlan(preparing, stored, []EditPlanClipReplacement{{
		ClipID: stored.Clips[0].ID, AssetID: third.ID, SourceInMs: 200,
	}}, assets)
	if err != nil {
		t.Fatalf("materialize replacement: %v", err)
	}
	newOutput := GenerationRenderOutput{
		StorageKey: "renders/new.mp4", MimeType: "video/mp4", DurationMs: 2000,
		Width: 1080, Height: 1920, FileSizeBytes: 2048, Renderer: "ffmpeg", RenderVersion: "v2",
	}
	oldKey, err := runs.CommitClipReplacementRender(ctx, run.ID, stored.UpdatedAt, replacedPlan, newOutput)
	if err != nil {
		t.Fatalf("commit replacement render: %v", err)
	}
	if oldKey != oldOutput.StorageKey {
		t.Fatalf("unexpected old output key %q", oldKey)
	}
	completed, _ := runs.Get(ctx, run.ID)
	committedPlan, _ := runs.GetEditPlan(ctx, run.ID)
	if completed.Status != generationRunStatusCompleted || completed.OutputStorageKey != newOutput.StorageKey || committedPlan.Clips[0].AssetID != third.ID {
		t.Fatalf("replacement was not committed run=%#v plan=%#v", completed, committedPlan.Clips)
	}
	if err := runs.MarkClipReplacementFailed(ctx, run.ID, errors.New("late task state failure")); err != nil {
		t.Fatalf("ignore late replacement failure: %v", err)
	}
	stillCompleted, _ := runs.Get(ctx, run.ID)
	if stillCompleted.Status != generationRunStatusCompleted || stillCompleted.OutputStorageKey != newOutput.StorageKey || stillCompleted.ErrorMessage != "" {
		t.Fatalf("late task failure overwrote committed replacement %#v", stillCompleted)
	}
}

func TestClipReplacementFailureRestoresCompletedRunAndPlan(t *testing.T) {
	ctx := context.Background()
	runs := NewGenerationRunService(nil)
	run, _ := runs.Create(ctx, CreateGenerationRunInput{ProductID: "product-1"})
	stored, err := runs.SaveEditPlan(ctx, replacementTestPlan(run.ID, "asset-1", "asset-2"))
	if err != nil {
		t.Fatalf("save plan: %v", err)
	}
	if err := runs.MarkRenderCompleted(ctx, run.ID, GenerationRenderOutput{
		StorageKey: "renders/original.mp4", MimeType: "video/mp4", DurationMs: 2000,
		Width: 1080, Height: 1920, FileSizeBytes: 1024, Renderer: "ffmpeg", RenderVersion: "v1",
	}); err != nil {
		t.Fatalf("complete original render: %v", err)
	}
	if _, err := runs.PrepareClipReplacementRender(ctx, run.ID, stored.UpdatedAt); err != nil {
		t.Fatalf("prepare replacement: %v", err)
	}
	if err := runs.MarkClipReplacementFailed(ctx, run.ID, errors.New("ffmpeg failed")); err != nil {
		t.Fatalf("restore completed run: %v", err)
	}
	restored, _ := runs.Get(ctx, run.ID)
	unchangedPlan, _ := runs.GetEditPlan(ctx, run.ID)
	if restored.Status != generationRunStatusCompleted || restored.OutputStorageKey != "renders/original.mp4" || restored.ErrorMessage != "ffmpeg failed" {
		t.Fatalf("unexpected restored run %#v", restored)
	}
	if unchangedPlan.Clips[0].AssetID != "asset-1" || !unchangedPlan.UpdatedAt.Equal(stored.UpdatedAt) {
		t.Fatalf("failed render changed the plan %#v", unchangedPlan)
	}
}

func mustCreateReplacementAsset(t *testing.T, service *ProductAssetService, productID string, fileName string, durationMs int) Asset {
	t.Helper()
	asset, err := service.CreateAsset(CreateAssetInput{
		ProductID: productID, FileName: fileName, StorageKey: "assets/" + fileName,
		SourceType: "visual_only", Status: "ready", AnalysisStatus: "ready",
		UsabilityStatus: "usable", ManualCleanStatus: "cleaned", DurationMs: durationMs,
	})
	if err != nil {
		t.Fatalf("create replacement asset: %v", err)
	}
	return asset
}

func replacementTestPlan(runID string, firstAssetID string, secondAssetID string) EditPlan {
	return EditPlan{
		GenerationRunID: runID, ScriptVariantID: "script-1", VoiceoverID: "voiceover-1", Status: "ready",
		VisualBeats: []VisualBeat{
			{ID: "visual-1", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 1000, Label: "第一段", VisualGoal: "展示第一段", SourceType: "visual_only"},
			{ID: "visual-2", NarrationSegmentID: "narration-2", StartMs: 1000, EndMs: 2000, Label: "第二段", VisualGoal: "展示第二段", SourceType: "visual_only"},
		},
		Clips: []EditPlanClip{
			{ID: "clip-1", VisualBeatID: "visual-1", NarrationSegmentID: "narration-1", AssetID: firstAssetID, SourceInMs: 0, SourceOutMs: 1000, StartMs: 0, EndMs: 1000, TimelineDurationMs: 1000, SourceType: "visual_only"},
			{ID: "clip-2", VisualBeatID: "visual-2", NarrationSegmentID: "narration-2", AssetID: secondAssetID, SourceInMs: 0, SourceOutMs: 1000, StartMs: 1000, EndMs: 2000, TimelineDurationMs: 1000, SourceType: "visual_only"},
		},
	}
}
