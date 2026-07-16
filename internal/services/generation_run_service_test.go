package services

import (
	"context"
	"encoding/json"
	"errors"
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
	run, err := service.Create(context.Background(), CreateGenerationRunInput{ProductID: "product-1", CreatedByUserID: "user-1"})
	if err != nil {
		t.Fatalf("create generation run: %v", err)
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
