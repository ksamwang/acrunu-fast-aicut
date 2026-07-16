package modelgateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAICompatibleEditPlannerRequestsCandidateBoundJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		responseFormat, _ := request["response_format"].(map[string]any)
		if responseFormat["type"] != "json_object" {
			t.Fatalf("expected json output request, got %#v", responseFormat)
		}
		if request["max_tokens"] != float64(defaultEditPlanMaxTokens) {
			t.Fatalf("unexpected max_tokens %#v", request["max_tokens"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"clips\":[{\"visual_beat_id\":\"visual-1\",\"candidate_id\":\"candidate-1\",\"source_in_ms\":0,\"source_out_ms\":1800,\"label\":\"展示\",\"visual_goal\":\"展示产品使用动作\"}]}"}}]}`))
	}))
	defer server.Close()

	planner := NewOpenAICompatibleEditPlanner(Config{
		Provider: "openai_compatible",
		BaseURL:  server.URL,
		Model:    "planner-model",
		Timeout:  time.Second,
	})
	result, err := planner.PlanEdits(context.Background(), EditPlanInput{
		ProductName: "束裤带",
		ScriptText:  "骑行时固定裤脚，更安心。",
		Requirements: []EditPlanRequirement{{
			VisualBeatID:       "visual-1",
			NarrationSegmentID: "narration-1",
			StartMs:            0,
			EndMs:              1800,
			NarrationText:      "骑行时固定裤脚，更安心。",
			VisualGoal:         "展示产品使用动作",
			SourceType:         "visual_only",
			Candidates: []EditPlanCandidate{{
				ID:          "candidate-1",
				SourceType:  "visual_only",
				SourceInMs:  0,
				SourceOutMs: 2400,
			}},
		}},
	})
	if err != nil {
		t.Fatalf("plan edits: %v", err)
	}
	if len(result.Clips) != 1 || result.Clips[0].CandidateID != "candidate-1" {
		t.Fatalf("unexpected plan result %#v", result)
	}
}

func TestValidateEditPlanResultAllowsRepeatedNarrationSegmentAcrossVisualBeats(t *testing.T) {
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{
		{VisualBeatID: "visual-1", CandidateID: "candidate-1", SourceInMs: 0, SourceOutMs: 1000, Label: "开头", VisualGoal: "画面"},
		{VisualBeatID: "visual-2", CandidateID: "candidate-2", SourceInMs: 0, SourceOutMs: 1000, Label: "收束", VisualGoal: "画面"},
	}}, []EditPlanRequirement{
		{VisualBeatID: "visual-1", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 1000, NarrationText: "第一句", VisualGoal: "画面", SourceType: "visual_only", Candidates: []EditPlanCandidate{{ID: "candidate-1", SourceOutMs: 1000}}},
		{VisualBeatID: "visual-2", NarrationSegmentID: "narration-1", StartMs: 1000, EndMs: 2000, NarrationText: "第一句", VisualGoal: "画面", SourceType: "visual_only", Candidates: []EditPlanCandidate{{ID: "candidate-2", SourceOutMs: 1000}}},
	})
	if err != nil {
		t.Fatalf("expected multiple visual beats for one narration segment to be valid: %v", err)
	}
}

func TestValidateEditPlanResultRejectsOutOfOrderVisualBeats(t *testing.T) {
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{
		{VisualBeatID: "visual-2", CandidateID: "candidate-2", SourceInMs: 0, SourceOutMs: 1000, Label: "第二段", VisualGoal: "画面"},
		{VisualBeatID: "visual-1", CandidateID: "candidate-1", SourceInMs: 0, SourceOutMs: 1000, Label: "第一段", VisualGoal: "画面"},
	}}, []EditPlanRequirement{
		{VisualBeatID: "visual-1", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 1000, NarrationText: "第一句", VisualGoal: "画面", SourceType: "visual_only", Candidates: []EditPlanCandidate{{ID: "candidate-1", SourceOutMs: 1000}}},
		{VisualBeatID: "visual-2", NarrationSegmentID: "narration-2", StartMs: 1000, EndMs: 2000, NarrationText: "第二句", VisualGoal: "画面", SourceType: "visual_only", Candidates: []EditPlanCandidate{{ID: "candidate-2", SourceOutMs: 1000}}},
	})
	if err == nil {
		t.Fatal("expected out-of-order visual beats to be rejected")
	}
}

func TestValidateEditPlanResultRejectsMismatchedSourceDuration(t *testing.T) {
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{{
		VisualBeatID: "visual-1", CandidateID: "candidate-1", SourceInMs: 0, SourceOutMs: 900, Label: "展示", VisualGoal: "画面",
	}}}, []EditPlanRequirement{{
		VisualBeatID: "visual-1", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 1000, NarrationText: "第一句", VisualGoal: "画面", SourceType: "visual_only", Candidates: []EditPlanCandidate{{ID: "candidate-1", SourceOutMs: 1200}},
	}})
	if err == nil {
		t.Fatal("expected mismatched source duration to be rejected")
	}
}

func TestValidateEditPlanResultRejectsNonContinuousVisualTimeline(t *testing.T) {
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{
		{VisualBeatID: "visual-1", CandidateID: "candidate-1", SourceInMs: 0, SourceOutMs: 1000, Label: "第一段", VisualGoal: "画面一"},
		{VisualBeatID: "visual-2", CandidateID: "candidate-2", SourceInMs: 0, SourceOutMs: 900, Label: "第二段", VisualGoal: "画面二"},
	}}, []EditPlanRequirement{
		{VisualBeatID: "visual-1", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 1000, NarrationText: "第一句", VisualGoal: "画面一", SourceType: "visual_only", Candidates: []EditPlanCandidate{{ID: "candidate-1", SourceOutMs: 1000}}},
		{VisualBeatID: "visual-2", NarrationSegmentID: "narration-2", StartMs: 1100, EndMs: 2000, NarrationText: "第二句", VisualGoal: "画面二", SourceType: "visual_only", Candidates: []EditPlanCandidate{{ID: "candidate-2", SourceOutMs: 900}}},
	})
	if err == nil {
		t.Fatal("expected non-continuous visual timeline to be rejected")
	}
}
