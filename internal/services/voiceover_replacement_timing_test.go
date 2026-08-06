package services

import (
	"context"
	"testing"
)

func TestRetimeEditPlanForVoiceoverPreservesMaterialsAndOrder(t *testing.T) {
	assets := NewProductAssetService()
	product := assets.CreateProduct(CreateProductInput{Name: "束裤带"})
	first := mustCreateReplacementAsset(t, assets, product.ID, "first-retime.mp4", 5000)
	second := mustCreateReplacementAsset(t, assets, product.ID, "second-retime.mp4", 5000)
	plan := EditPlan{
		GenerationRunID: "run-1", ScriptVariantID: "script-old", VoiceoverID: "voice-old", Status: "ready",
		SourceDurationMs: 4000, TimelineDurationMs: 4000,
		NarrationSegments: []NarrationSegment{
			{ID: "n1", StartMs: 0, EndMs: 2000, Text: "第一句。"},
			{ID: "n2", StartMs: 2000, EndMs: 4000, Text: "第二句。"},
		},
		VisualBeats: []VisualBeat{
			{ID: "b1", NarrationSegmentID: "n1", StartMs: 0, EndMs: 2000, DurationClass: VisualBeatDurationStandard, Label: "第一段", VisualGoal: "展示第一段", SourceType: "visual_only"},
			{ID: "b2", NarrationSegmentID: "n2", StartMs: 2000, EndMs: 4000, DurationClass: VisualBeatDurationStandard, Label: "第二段", VisualGoal: "展示第二段", SourceType: "visual_only"},
		},
		Clips: []EditPlanClip{
			{ID: "c1", VisualBeatID: "b1", NarrationSegmentID: "n1", AssetID: first.ID, SourceInMs: 0, SourceOutMs: 2000, StartMs: 0, EndMs: 2000, TimelineDurationMs: 2000, SourceType: "visual_only"},
			{ID: "c2", VisualBeatID: "b2", NarrationSegmentID: "n2", AssetID: second.ID, SourceInMs: 0, SourceOutMs: 2000, StartMs: 2000, EndMs: 4000, TimelineDurationMs: 2000, SourceType: "visual_only"},
		},
	}
	oldWork := VoiceoverWork{ScriptText: "第一句。第二句。", DurationMs: 4000, NarrationSegments: cloneNarrationSegments(plan.NarrationSegments)}
	newWork := VoiceoverWork{ScriptText: oldWork.ScriptText, DurationMs: 4400, NarrationSegments: []NarrationSegment{
		{ID: "new-n1", StartMs: 0, EndMs: 2200, Text: "第一句。"},
		{ID: "new-n2", StartMs: 2200, EndMs: 4400, Text: "第二句。"},
	}}

	retimed, err := RetimeEditPlanForVoiceover(plan, oldWork, newWork, "script-new", "voice-new", assets)
	if err != nil {
		t.Fatalf("retime edit plan: %v", err)
	}
	if retimed.ScriptVariantID != "script-new" || retimed.VoiceoverID != "voice-new" || retimed.TimelineDurationMs != 4400 {
		t.Fatalf("unexpected replacement plan metadata %#v", retimed)
	}
	if retimed.Clips[0].AssetID != first.ID || retimed.Clips[1].AssetID != second.ID || retimed.Clips[0].EndMs != 2200 || retimed.Clips[1].StartMs != 2200 || retimed.Clips[1].EndMs != 4400 {
		t.Fatalf("materials or order changed %#v", retimed.Clips)
	}
	if retimed.VisualBeats[0].NarrationSegmentID != "new-n1" || retimed.Clips[1].NarrationSegmentID != "new-n2" {
		t.Fatalf("new narration references were not applied %#v %#v", retimed.VisualBeats, retimed.Clips)
	}
}

func TestVoiceoverReplacementCommitKeepsOriginalUntilSuccess(t *testing.T) {
	ctx := context.Background()
	runs := NewGenerationRunService(nil)
	run, err := runs.Create(ctx, CreateGenerationRunInput{ProductID: "product-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	runs.mu.Lock()
	run.VoiceoverTaskID, run.ScriptVariantID, run.VoiceoverID = "task-old", "script-old", "voice-old"
	runs.memoryRuns[run.ID] = run
	runs.mu.Unlock()
	plan := replacementTestPlan(run.ID, "asset-1", "asset-2")
	stored, err := runs.SaveEditPlan(ctx, plan)
	if err != nil {
		t.Fatalf("save original plan: %v", err)
	}
	oldOutput := GenerationRenderOutput{StorageKey: "renders/old.mp4", MimeType: "video/mp4", DurationMs: 2000, Width: 1080, Height: 1920, FileSizeBytes: 1024, Renderer: "ffmpeg", RenderVersion: "v1"}
	if err := runs.MarkRenderCompleted(ctx, run.ID, oldOutput); err != nil {
		t.Fatalf("complete original render: %v", err)
	}
	replacement, err := runs.CreateVoiceoverReplacement(ctx, run.ID, "task-new", "script-new", "voice-new", "user-1")
	if err != nil {
		t.Fatalf("create replacement: %v", err)
	}
	if err := runs.MarkVoiceoverReplacementReady(ctx, replacement.ID); err != nil {
		t.Fatalf("mark replacement ready: %v", err)
	}
	if err := runs.MarkVoiceoverReplacementApplying(ctx, replacement.ID, "render-task"); err != nil {
		t.Fatalf("mark replacement applying: %v", err)
	}
	before, _ := runs.Get(ctx, run.ID)
	if before.OutputStorageKey != oldOutput.StorageKey || before.VoiceoverTaskID != "task-old" {
		t.Fatalf("replacement changed original before commit %#v", before)
	}
	plan.ScriptVariantID, plan.VoiceoverID = "script-new", "voice-new"
	newOutput := GenerationRenderOutput{StorageKey: "renders/new.mp4", MimeType: "video/mp4", DurationMs: 2000, Width: 1080, Height: 1920, FileSizeBytes: 2048, Renderer: "ffmpeg", RenderVersion: "v2"}
	oldKey, err := runs.CommitVoiceoverReplacementRender(ctx, replacement.ID, stored.UpdatedAt, plan, newOutput)
	if err != nil {
		t.Fatalf("commit replacement: %v", err)
	}
	after, _ := runs.Get(ctx, run.ID)
	if oldKey != oldOutput.StorageKey || after.OutputStorageKey != newOutput.StorageKey || after.VoiceoverTaskID != "task-new" || after.ScriptVariantID != "script-new" || after.VoiceoverID != "voice-new" {
		t.Fatalf("replacement commit was not atomic %#v old=%q", after, oldKey)
	}
}
