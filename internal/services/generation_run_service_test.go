package services

import (
	"context"
	"encoding/json"
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
