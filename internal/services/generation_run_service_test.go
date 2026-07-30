package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type staticVoiceoverWorkLoader struct {
	work VoiceoverWork
	err  error
}

func (l staticVoiceoverWorkLoader) GetVoiceoverWork(_ context.Context, _ string) (VoiceoverWork, error) {
	return l.work, l.err
}

func TestGenerationRunKeepsPlanReadyWorkGenerating(t *testing.T) {
	loader := staticVoiceoverWorkLoader{work: VoiceoverWork{
		ID:          "voiceover-task-1",
		ProductID:   "product-1",
		ProductName: "束裤带",
		Title:       "裤脚不再蹭链条",
		ScriptText:  "骑行时固定裤脚。",
		Status:      "completed",
		Progress:    100,
	}}
	service := NewGenerationRunService(loader)
	run, err := service.Create(context.Background(), CreateGenerationRunInput{ProductID: "product-1", CreatedByUserID: "user-1", CreatedByName: "王璐"})
	if err != nil {
		t.Fatalf("create generation run: %v", err)
	}
	if run.GenerationBatchID == "" {
		t.Fatal("expected an implicit generation batch id")
	}
	if err := service.LinkTask(context.Background(), run.ID, "voiceover-task-1", generationRunTaskStageVoiceover); err != nil {
		t.Fatalf("link voiceover task: %v", err)
	}
	if err := service.AttachVoiceoverArtifacts(context.Background(), run.ID, "voiceover-task-1", "script-1", "voiceover-1"); err != nil {
		t.Fatalf("attach artifacts: %v", err)
	}
	if err := service.UpdateStage(context.Background(), run.ID, generationRunStagePlanReady, 88); err != nil {
		t.Fatalf("mark plan ready: %v", err)
	}
	if _, err := service.SaveEditPlan(context.Background(), EditPlan{
		GenerationRunID: run.ID,
		ScriptVariantID: "script-1",
		VoiceoverID:     "voiceover-1",
		Status:          "ready",
		VisualBeats: []VisualBeat{{
			ID:                 "visual-1",
			NarrationSegmentID: "narration-1",
			StartMs:            0,
			EndMs:              1000,
			Label:              "展示",
			VisualGoal:         "展示固定动作",
			SourceType:         "visual_only",
		}},
		Clips: []EditPlanClip{{
			VisualBeatID:       "visual-1",
			NarrationSegmentID: "narration-1",
			AssetID:            "asset-1",
			SourceInMs:         0,
			SourceOutMs:        1000,
			StartMs:            0,
			EndMs:              1000,
			TimelineDurationMs: 1000,
			SourceType:         "visual_only",
		}},
	}); err != nil {
		t.Fatalf("save edit plan: %v", err)
	}
	work, err := service.GetWork(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get work: %v", err)
	}
	if work.Status != generationRunStatusGenerating || work.StageLabel != "编排完成，等待渲染" || work.Progress != 88 {
		t.Fatalf("expected plan-ready work to remain generating, got %#v", work)
	}
	if work.ID != run.ID || len(work.VisualBeats) != 1 || len(work.EditPlan) != 1 || work.EditPlan[0].VisualBeatID != "visual-1" || work.EditPlan[0].AssetID != "asset-1" {
		t.Fatalf("unexpected work projection %#v", work)
	}
	if work.VisualBeats[0].DurationClass != VisualBeatDurationLegacy {
		t.Fatalf("expected historical visual beat to default to legacy duration class, got %#v", work.VisualBeats[0])
	}
	if work.CreatedByUserID != "user-1" || work.CreatedByName != "王璐" {
		t.Fatalf("unexpected creator projection %#v", work)
	}
}

func TestGenerationRunPersistsMultipleClipsPerVisualBeat(t *testing.T) {
	service := NewGenerationRunService(nil)
	run, err := service.Create(context.Background(), CreateGenerationRunInput{ProductID: "product-1"})
	if err != nil {
		t.Fatalf("create generation run: %v", err)
	}
	plan, err := service.SaveEditPlan(context.Background(), EditPlan{
		GenerationRunID: run.ID,
		ScriptVariantID: "script-1",
		VoiceoverID:     "voiceover-1",
		Status:          "ready",
		VisualBeats: []VisualBeat{{
			ID: "visual-pocket", NarrationSegmentID: "narration-pocket", StartMs: 0, EndMs: 3440,
			Label: "小巧便携", VisualGoal: "展示束裤带小巧并放入口袋", SourceType: "visual_only",
		}},
		Clips: []EditPlanClip{
			{
				VisualBeatID: "visual-pocket", NarrationSegmentID: "narration-pocket", AssetID: "asset-detail",
				SourceInMs: 200, SourceOutMs: 1140, StartMs: 0, EndMs: 940, TimelineDurationMs: 940,
				SourceType: "visual_only", Label: "产品特写", VisualGoal: "展示束裤带小巧",
			},
			{
				VisualBeatID: "visual-pocket", NarrationSegmentID: "narration-pocket", AssetID: "asset-pocket",
				SourceInMs: 0, SourceOutMs: 2500, StartMs: 940, EndMs: 3440, TimelineDurationMs: 2500,
				SourceType: "visual_only", Label: "放入口袋", VisualGoal: "完整展示放入口袋动作",
			},
		},
	})
	if err != nil {
		t.Fatalf("save multi-clip edit plan: %v", err)
	}
	if len(plan.Clips) != 2 || plan.Clips[0].VisualBeatID != plan.Clips[1].VisualBeatID || plan.Clips[1].StartMs != plan.Clips[0].EndMs {
		t.Fatalf("unexpected multi-clip plan %#v", plan.Clips)
	}
	stored, err := service.GetEditPlan(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get multi-clip edit plan: %v", err)
	}
	if len(stored.Clips) != 2 || stored.Clips[1].AssetID != "asset-pocket" {
		t.Fatalf("multi-clip plan was not retained %#v", stored.Clips)
	}
}

func TestGenerationRunRejectsRepeatedAssetInEditPlan(t *testing.T) {
	service := NewGenerationRunService(nil)
	run, err := service.Create(context.Background(), CreateGenerationRunInput{ProductID: "product-1"})
	if err != nil {
		t.Fatalf("create generation run: %v", err)
	}
	_, err = service.SaveEditPlan(context.Background(), EditPlan{
		GenerationRunID: run.ID,
		ScriptVariantID: "script-1",
		VoiceoverID:     "voiceover-1",
		Status:          "ready",
		VisualBeats: []VisualBeat{
			{ID: "visual-1", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 1000, Label: "第一段", VisualGoal: "展示外观", SourceType: "visual_only"},
			{ID: "visual-2", NarrationSegmentID: "narration-2", StartMs: 1000, EndMs: 2000, Label: "第二段", VisualGoal: "展示使用", SourceType: "visual_only"},
		},
		Clips: []EditPlanClip{
			{VisualBeatID: "visual-1", NarrationSegmentID: "narration-1", AssetID: "asset-shared", SourceInMs: 0, SourceOutMs: 1000, StartMs: 0, EndMs: 1000, TimelineDurationMs: 1000, SourceType: "visual_only"},
			{VisualBeatID: "visual-2", NarrationSegmentID: "narration-2", AssetID: "asset-shared", SourceInMs: 1000, SourceOutMs: 2000, StartMs: 1000, EndMs: 2000, TimelineDurationMs: 1000, SourceType: "visual_only"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "reuses asset") {
		t.Fatalf("expected repeated asset to be rejected, got %v", err)
	}
}

func TestGenerationRunSnapshotIsJSONText(t *testing.T) {
	snapshot, err := generationRunSnapshotJSON(map[string]any{
		"voice_profile_id": "profile-1",
		"variant_index":    1,
	})
	if err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(snapshot), &decoded); err != nil {
		t.Fatalf("snapshot is not JSON text: %v", err)
	}
	if decoded["voice_profile_id"] != "profile-1" || decoded["variant_index"] != float64(1) {
		t.Fatalf("unexpected snapshot %#v", decoded)
	}
}

func TestGenerationRunPrepareRetryKeepsRunAndClearsStalePlanWork(t *testing.T) {
	loader := staticVoiceoverWorkLoader{work: VoiceoverWork{ID: "voiceover-task-1", ProductID: "product-1", ProductName: "束裤带", Status: "completed"}}
	service := NewGenerationRunService(loader)
	run, err := service.Create(context.Background(), CreateGenerationRunInput{ProductID: "product-1", CreatedByUserID: "user-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := service.LinkTask(context.Background(), run.ID, "voiceover-task-1", generationRunTaskStageVoiceover); err != nil {
		t.Fatalf("link voiceover task: %v", err)
	}
	if err := service.LinkTask(context.Background(), run.ID, "edit-plan-task-1", generationRunTaskStageEditPlan); err != nil {
		t.Fatalf("link edit plan task: %v", err)
	}
	if err := service.AttachVoiceoverArtifacts(context.Background(), run.ID, "voiceover-task-1", "script-1", "voiceover-1"); err != nil {
		t.Fatalf("attach voiceover artifacts: %v", err)
	}
	if _, err := service.SaveEditPlan(context.Background(), EditPlan{
		GenerationRunID: run.ID,
		ScriptVariantID: "script-1",
		VoiceoverID:     "voiceover-1",
		Status:          "failed",
	}); err != nil {
		t.Fatalf("save failed plan: %v", err)
	}
	if err := service.MarkFailed(context.Background(), run.ID, errors.New("planner timeout")); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	retried, err := service.PrepareRetry(context.Background(), run.ID, GenerationRunRetryEditPlan)
	if err != nil {
		t.Fatalf("prepare edit plan retry: %v", err)
	}
	if retried.ID != run.ID || retried.Status != generationRunStatusGenerating || retried.Stage != generationRunStageRetrieving || retried.Progress != 76 || retried.ErrorMessage != "" {
		t.Fatalf("unexpected retried run %#v", retried)
	}
	if _, exists, err := service.FindTaskByStage(context.Background(), run.ID, generationRunTaskStageEditPlan); err != nil || exists {
		t.Fatalf("expected stale edit plan task link to be removed: exists=%t err=%v", exists, err)
	}
	if _, exists, err := service.FindTaskByStage(context.Background(), run.ID, generationRunTaskStageVoiceover); err != nil || !exists {
		t.Fatalf("expected successful voiceover task link to remain: exists=%t err=%v", exists, err)
	}
	if _, err := service.GetEditPlan(context.Background(), run.ID); !errors.Is(err, ErrEditPlanNotFound) {
		t.Fatalf("expected previous edit plan to be cleared, got %v", err)
	}

	if err := service.MarkFailed(context.Background(), run.ID, errors.New("voiceover unavailable")); err != nil {
		t.Fatalf("mark failed for voice retry: %v", err)
	}
	if _, err := service.PrepareRetry(context.Background(), run.ID, GenerationRunRetryVoiceover); err != nil {
		t.Fatalf("prepare voiceover retry: %v", err)
	}
	if _, exists, err := service.FindTaskByStage(context.Background(), run.ID, generationRunTaskStageVoiceover); err != nil || exists {
		t.Fatalf("expected stale voiceover task link to be removed: exists=%t err=%v", exists, err)
	}
}

func TestGenerationRunMarksRenderOutputCompleted(t *testing.T) {
	t.Parallel()
	service := NewGenerationRunService(nil)
	run, err := service.Create(context.Background(), CreateGenerationRunInput{ProductID: "product-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	output := GenerationRenderOutput{
		StorageKey:    "renders/generations/run/final.mp4",
		MimeType:      "video/mp4",
		DurationMs:    17240,
		Width:         1080,
		Height:        1920,
		FileSizeBytes: 4096,
		Renderer:      "ffmpeg",
		RenderVersion: "ffmpeg-v1",
	}
	if err := service.MarkRenderCompleted(context.Background(), run.ID, output); err != nil {
		t.Fatalf("mark render completed: %v", err)
	}
	completed, err := service.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get completed run: %v", err)
	}
	if completed.Status != generationRunStatusCompleted || completed.Stage != generationRunStageCompleted || completed.Progress != 100 || completed.OutputStorageKey != output.StorageKey || completed.CompletedAt == nil {
		t.Fatalf("unexpected completed run %#v", completed)
	}
}

func TestGenerationRunRenderRetryKeepsReadyPlan(t *testing.T) {
	t.Parallel()
	service := NewGenerationRunService(nil)
	run, err := service.Create(context.Background(), CreateGenerationRunInput{ProductID: "product-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := service.SaveEditPlan(context.Background(), EditPlan{
		GenerationRunID: run.ID,
		ScriptVariantID: "script-1",
		VoiceoverID:     "voiceover-1",
		Status:          "ready",
		VisualBeats: []VisualBeat{{
			ID: "visual-1", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 1000,
			Label: "展示", VisualGoal: "展示产品", SourceType: "visual_only",
		}},
		Clips: []EditPlanClip{{
			VisualBeatID: "visual-1", NarrationSegmentID: "narration-1", AssetID: "asset-1",
			SourceInMs: 0, SourceOutMs: 1000, StartMs: 0, EndMs: 1000, TimelineDurationMs: 1000,
			SourceType: "visual_only",
		}},
	}); err != nil {
		t.Fatalf("save plan: %v", err)
	}
	if err := service.LinkTask(context.Background(), run.ID, "render-task-1", generationRunTaskStageRender); err != nil {
		t.Fatalf("link render task: %v", err)
	}
	if err := service.MarkFailed(context.Background(), run.ID, errors.New("ffmpeg failed")); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	retried, err := service.PrepareRetry(context.Background(), run.ID, GenerationRunRetryRender)
	if err != nil {
		t.Fatalf("prepare render retry: %v", err)
	}
	if retried.Stage != generationRunStagePlanReady || retried.Progress != 88 || retried.Status != generationRunStatusGenerating {
		t.Fatalf("unexpected render retry state %#v", retried)
	}
	if _, exists, err := service.FindTaskByStage(context.Background(), run.ID, generationRunTaskStageRender); err != nil || exists {
		t.Fatalf("render task link should be cleared: exists=%t err=%v", exists, err)
	}
	if _, err := service.GetEditPlan(context.Background(), run.ID); err != nil {
		t.Fatalf("ready plan must survive render retry: %v", err)
	}
}

func TestGenerationRunRegeneratesCompletedRunAndDeletesInactiveRun(t *testing.T) {
	t.Parallel()
	service := NewGenerationRunService(nil)
	run, err := service.Create(context.Background(), CreateGenerationRunInput{ProductID: "product-1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := service.LinkTask(context.Background(), run.ID, "voiceover-task-1", generationRunTaskStageVoiceover); err != nil {
		t.Fatalf("link voiceover task: %v", err)
	}
	if err := service.MarkRenderCompleted(context.Background(), run.ID, GenerationRenderOutput{
		StorageKey: "renders/generations/run/final.mp4", MimeType: "video/mp4", DurationMs: 1000,
		Width: 1080, Height: 1920, FileSizeBytes: 1024, Renderer: "ffmpeg", RenderVersion: "v1",
	}); err != nil {
		t.Fatalf("complete run: %v", err)
	}

	regenerated, err := service.PrepareRetry(context.Background(), run.ID, GenerationRunRetryVoiceover)
	if err != nil {
		t.Fatalf("regenerate completed run: %v", err)
	}
	if regenerated.Status != generationRunStatusGenerating || regenerated.Stage != generationRunStageVoicing || regenerated.OutputStorageKey != "" {
		t.Fatalf("unexpected regenerated run %#v", regenerated)
	}
	if _, err := service.Delete(context.Background(), run.ID); !errors.Is(err, ErrGenerationRunActive) {
		t.Fatalf("expected active delete error, got %v", err)
	}
	if err := service.MarkFailed(context.Background(), run.ID, errors.New("stopped")); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	deleted, err := service.Delete(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("delete failed run: %v", err)
	}
	if deleted.ID != run.ID {
		t.Fatalf("unexpected deleted run %#v", deleted)
	}
	if _, err := service.Get(context.Background(), run.ID); !errors.Is(err, ErrGenerationRunNotFound) {
		t.Fatalf("expected deleted run to be missing, got %v", err)
	}
}
