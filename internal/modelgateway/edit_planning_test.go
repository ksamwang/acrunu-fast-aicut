package modelgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAICompatibleEditPlannerRequestsCandidateBoundJSON(t *testing.T) {
	var logs bytes.Buffer
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
		if request["max_tokens"] != float64(8192) {
			t.Fatalf("unexpected max_tokens %#v", request["max_tokens"])
		}
		if _, exists := request["enable_thinking"]; exists {
			t.Fatalf("planner request must not contain provider-specific fields: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"clips\":[{\"visual_beat_id\":\"visual-1\",\"candidate_id\":\"candidate-1\",\"start_ms\":0,\"end_ms\":1800,\"source_in_ms\":0,\"source_out_ms\":1800,\"label\":\"展示\",\"visual_goal\":\"展示产品使用动作\"}]}"}}]}`))
	}))
	defer server.Close()

	planner := NewOpenAICompatibleEditPlanner(Config{
		Provider:  "openai_compatible",
		BaseURL:   server.URL,
		Model:     "planner-model",
		MaxTokens: 8192,
		Timeout:   time.Second,
	}).WithLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
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
	if !strings.Contains(logs.String(), `"request_bytes"`) || !strings.Contains(logs.String(), `"duration_ms"`) || !strings.Contains(logs.String(), `"response_model"`) {
		t.Fatalf("expected request telemetry, got %s", logs.String())
	}
}

func TestOpenAICompatibleEditPlannerUsesConfiguredTokensAndDefaultTimeout(t *testing.T) {
	planner := NewOpenAICompatibleEditPlanner(Config{MaxTokens: 8192})
	if planner.maxTokens != 8192 {
		t.Fatalf("expected configured max tokens, got %d", planner.maxTokens)
	}
	if planner.timeout != 300*time.Second {
		t.Fatalf("expected 300 second default timeout, got %s", planner.timeout)
	}
}

func TestOpenAICompatibleEditPlannerOmitsMaxTokensWhenUnconfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, exists := request["max_tokens"]; exists {
			t.Fatalf("max_tokens should be omitted when unconfigured: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"clips\":[{\"visual_beat_id\":\"visual-1\",\"candidate_id\":\"candidate-1\",\"start_ms\":0,\"end_ms\":1800,\"source_in_ms\":0,\"source_out_ms\":1800,\"label\":\"展示\",\"visual_goal\":\"展示产品使用动作\"}]}"}}]}`))
	}))
	defer server.Close()

	planner := NewOpenAICompatibleEditPlanner(Config{
		Provider: "openai_compatible",
		BaseURL:  server.URL,
		Model:    "planner-model",
		Timeout:  time.Second,
	})
	if planner.maxTokens != 0 {
		t.Fatalf("expected unconfigured max tokens to remain unset, got %d", planner.maxTokens)
	}
	if _, err := planner.PlanEdits(context.Background(), editPlannerTestInput()); err != nil {
		t.Fatalf("plan edits: %v", err)
	}
}

func TestOpenAICompatibleEditPlannerReportsEmptyStandardContentMetadata(t *testing.T) {
	var logs bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"response-model","choices":[{"finish_reason":"length","message":{"content":""}}]}`))
	}))
	defer server.Close()

	planner := NewOpenAICompatibleEditPlanner(Config{
		Provider: "openai_compatible",
		BaseURL:  server.URL,
		Model:    "planner-model",
		Timeout:  time.Second,
	}).WithLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
	_, err := planner.PlanEdits(context.Background(), editPlannerTestInput())
	if err == nil || !strings.Contains(err.Error(), `choices=1, finish_reason="length"`) {
		t.Fatalf("expected standard empty-response diagnostics, got %v", err)
	}
	for _, field := range []string{`"response_model":"response-model"`, `"choice_count":1`, `"finish_reason":"length"`, `"content_bytes":0`} {
		if !strings.Contains(logs.String(), field) {
			t.Fatalf("expected %s in response metadata, got %s", field, logs.String())
		}
	}
}

func editPlannerTestInput() EditPlanInput {
	return EditPlanInput{
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
	}
}

func TestValidateEditPlanResultAllowsRepeatedNarrationSegmentAcrossVisualBeats(t *testing.T) {
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{
		{VisualBeatID: "visual-1", CandidateID: "candidate-1", StartMs: 0, EndMs: 1000, SourceInMs: 0, SourceOutMs: 1000, Label: "开头", VisualGoal: "画面"},
		{VisualBeatID: "visual-2", CandidateID: "candidate-2", StartMs: 1000, EndMs: 2000, SourceInMs: 0, SourceOutMs: 1000, Label: "收束", VisualGoal: "画面"},
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
		{VisualBeatID: "visual-2", CandidateID: "candidate-2", StartMs: 0, EndMs: 1000, SourceInMs: 0, SourceOutMs: 1000, Label: "第二段", VisualGoal: "画面"},
		{VisualBeatID: "visual-1", CandidateID: "candidate-1", StartMs: 1000, EndMs: 2000, SourceInMs: 0, SourceOutMs: 1000, Label: "第一段", VisualGoal: "画面"},
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
		VisualBeatID: "visual-1", CandidateID: "candidate-1", StartMs: 0, EndMs: 1000, SourceInMs: 0, SourceOutMs: 900, Label: "展示", VisualGoal: "画面",
	}}}, []EditPlanRequirement{{
		VisualBeatID: "visual-1", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 1000, NarrationText: "第一句", VisualGoal: "画面", SourceType: "visual_only", Candidates: []EditPlanCandidate{{ID: "candidate-1", SourceOutMs: 1200}},
	}})
	if err == nil {
		t.Fatal("expected mismatched source duration to be rejected")
	}
}

func TestValidateEditPlanResultRejectsNonContinuousVisualTimeline(t *testing.T) {
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{
		{VisualBeatID: "visual-1", CandidateID: "candidate-1", StartMs: 0, EndMs: 1000, SourceInMs: 0, SourceOutMs: 1000, Label: "第一段", VisualGoal: "画面一"},
		{VisualBeatID: "visual-2", CandidateID: "candidate-2", StartMs: 1100, EndMs: 2000, SourceInMs: 0, SourceOutMs: 900, Label: "第二段", VisualGoal: "画面二"},
	}}, []EditPlanRequirement{
		{VisualBeatID: "visual-1", NarrationSegmentID: "narration-1", StartMs: 0, EndMs: 1000, NarrationText: "第一句", VisualGoal: "画面一", SourceType: "visual_only", Candidates: []EditPlanCandidate{{ID: "candidate-1", SourceOutMs: 1000}}},
		{VisualBeatID: "visual-2", NarrationSegmentID: "narration-2", StartMs: 1100, EndMs: 2000, NarrationText: "第二句", VisualGoal: "画面二", SourceType: "visual_only", Candidates: []EditPlanCandidate{{ID: "candidate-2", SourceOutMs: 900}}},
	})
	if err == nil {
		t.Fatal("expected non-continuous visual timeline to be rejected")
	}
}

func TestValidateEditPlanResultAllowsMultipleNormalSpeedClipsForOneVisualBeat(t *testing.T) {
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{
		{VisualBeatID: "visual-pocket", CandidateID: "candidate-detail", StartMs: 0, EndMs: 940, SourceInMs: 200, SourceOutMs: 1140, Label: "产品特写", VisualGoal: "展示束裤带小巧"},
		{VisualBeatID: "visual-pocket", CandidateID: "candidate-pocket", StartMs: 940, EndMs: 3440, SourceInMs: 0, SourceOutMs: 2500, Label: "放入口袋", VisualGoal: "完整展示放入口袋动作"},
	}}, []EditPlanRequirement{{
		VisualBeatID: "visual-pocket", NarrationSegmentID: "narration-pocket", StartMs: 0, EndMs: 3440,
		NarrationText: "小小一个，放口袋里完全没负担。", VisualGoal: "展示束裤带小巧，放入口袋", SourceType: "visual_only",
		Candidates: []EditPlanCandidate{
			{ID: "candidate-detail", SourceInMs: 0, SourceOutMs: 1800},
			{ID: "candidate-pocket", SourceInMs: 0, SourceOutMs: 2500},
		},
	}})
	if err != nil {
		t.Fatalf("expected multiple normal-speed clips for one visual beat to be valid: %v", err)
	}
}

func TestValidateEditPlanResultRejectsTinyFillerClip(t *testing.T) {
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{
		{VisualBeatID: "visual-pocket", CandidateID: "candidate-pocket", StartMs: 0, EndMs: 3300, SourceInMs: 0, SourceOutMs: 3300, Label: "放入口袋", VisualGoal: "完整展示放入口袋动作"},
		{VisualBeatID: "visual-pocket", CandidateID: "candidate-detail", StartMs: 3300, EndMs: 3440, SourceInMs: 0, SourceOutMs: 140, Label: "产品特写", VisualGoal: "展示束裤带小巧"},
	}}, []EditPlanRequirement{{
		VisualBeatID: "visual-pocket", NarrationSegmentID: "narration-pocket", StartMs: 0, EndMs: 3440,
		NarrationText: "小小一个，放口袋里完全没负担。", VisualGoal: "展示束裤带小巧，放入口袋", SourceType: "visual_only",
		Candidates: []EditPlanCandidate{
			{ID: "candidate-detail", SourceInMs: 0, SourceOutMs: 1800},
			{ID: "candidate-pocket", SourceInMs: 0, SourceOutMs: 3300},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "shorter than 800ms") {
		t.Fatalf("expected tiny filler clip to be rejected, got %v", err)
	}
}
