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

func TestOpenAICompatibleEditPlannerRequestsMaterialSelectionsOnly(t *testing.T) {
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
		for _, forbidden := range []string{"narration-1", "visual-1", "source_in_ms", "source_out_ms"} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("planner selection request must not expose %q: %s", forbidden, string(body))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"clips\":[{\"slot_id\":\"s001\",\"candidate_id\":\"m001\"}]}"}}]}`))
	}))
	defer server.Close()

	planner := NewOpenAICompatibleEditPlanner(Config{
		Provider:  "openai_compatible",
		BaseURL:   server.URL,
		Model:     "planner-model",
		MaxTokens: 8192,
		Timeout:   time.Second,
	}).WithLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
	result, err := planner.PlanEdits(context.Background(), editPlannerTestInput())
	if err != nil {
		t.Fatalf("plan edits: %v", err)
	}
	if len(result.Clips) != 1 || result.Clips[0].SlotID != "s001" || result.Clips[0].CandidateID != "m001" {
		t.Fatalf("unexpected plan result %#v", result)
	}
	if !strings.Contains(logs.String(), `"request_bytes"`) || !strings.Contains(logs.String(), `"duration_ms"`) || !strings.Contains(logs.String(), `"response_model"`) {
		t.Fatalf("expected request telemetry, got %s", logs.String())
	}
}

func TestOpenAICompatibleEditPlannerUsesConfiguredTokensAndDefaultTimeout(t *testing.T) {
	planner := NewOpenAICompatibleEditPlanner(Config{MaxTokens: 8192})
	if planner.maxTokens != 8192 || planner.timeout != 300*time.Second {
		t.Fatalf("unexpected planner config tokens=%d timeout=%s", planner.maxTokens, planner.timeout)
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
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"clips\":[{\"slot_id\":\"s001\",\"candidate_id\":\"m001\"}]}"}}]}`))
	}))
	defer server.Close()

	planner := NewOpenAICompatibleEditPlanner(Config{Provider: "openai_compatible", BaseURL: server.URL, Model: "planner-model", Timeout: time.Second})
	if _, err := planner.PlanEdits(context.Background(), editPlannerTestInput()); err != nil {
		t.Fatalf("plan edits: %v", err)
	}
}

func TestOpenAICompatibleEditPlannerReportsEmptyStandardContentMetadata(t *testing.T) {
	var logs bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"response-model","choices":[{"finish_reason":"length","message":{"content":""}}]}`))
	}))
	defer server.Close()

	planner := NewOpenAICompatibleEditPlanner(Config{Provider: "openai_compatible", BaseURL: server.URL, Model: "planner-model", Timeout: time.Second}).
		WithLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
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
			DurationClass:      VisualDurationClassStandard,
			NarrationText:      "骑行时固定裤脚，更安心。",
			Label:              "固定裤脚",
			VisualGoal:         "手将束裤带环绕裤脚并粘贴固定",
			SourceType:         TTSVisualSourceType,
			Slots: []EditPlanSlot{{
				ID: "s001", StartMs: 0, EndMs: 1800, DurationMs: 1800, Role: EditPlanSlotRolePrimary,
				Candidates: []EditPlanCandidate{{ID: "m001", SourceType: TTSVisualSourceType, SourceInMs: 0, SourceOutMs: 2400}},
			}},
		}},
	}
}

func TestValidateEditPlanResultAllowsRepeatedNarrationSegmentAcrossVisualBeats(t *testing.T) {
	requirements := []EditPlanRequirement{
		testEditRequirement("visual-1", "narration-1", "s001", "m001", 0, 1000),
		testEditRequirement("visual-2", "narration-1", "s002", "m002", 1000, 2000),
	}
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{
		{SlotID: "s001", CandidateID: "m001"},
		{SlotID: "s002", CandidateID: "m002"},
	}}, requirements)
	if err != nil {
		t.Fatalf("expected multiple visual beats for one narration segment to be valid: %v", err)
	}
}

func TestValidateEditPlanResultRejectsRepeatedMaterial(t *testing.T) {
	requirements := []EditPlanRequirement{
		testEditRequirement("visual-1", "narration-1", "s001", "m001", 0, 1000),
		testEditRequirement("visual-2", "narration-2", "s002", "m001", 1000, 2000),
	}
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{
		{SlotID: "s001", CandidateID: "m001"},
		{SlotID: "s002", CandidateID: "m001"},
	}}, requirements)
	if err == nil || !strings.Contains(err.Error(), "reuses material") {
		t.Fatalf("expected repeated material to be rejected, got %v", err)
	}
}

func TestValidateEditPlanResultRejectsCandidateOutsideSlot(t *testing.T) {
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{{SlotID: "s001", CandidateID: "m999"}}}, editPlannerTestInput().Requirements)
	if err == nil || !strings.Contains(err.Error(), "outside its allowed set") {
		t.Fatalf("expected closed slot candidate validation, got %v", err)
	}
}

func TestValidateEditPlanResultRejectsOutOfOrderSlots(t *testing.T) {
	requirements := []EditPlanRequirement{
		testEditRequirement("visual-1", "narration-1", "s001", "m001", 0, 1000),
		testEditRequirement("visual-2", "narration-2", "s002", "m002", 1000, 2000),
	}
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{
		{SlotID: "s002", CandidateID: "m002"},
		{SlotID: "s001", CandidateID: "m001"},
	}}, requirements)
	if err == nil || !strings.Contains(err.Error(), "out of slot order") {
		t.Fatalf("expected out-of-order slots to be rejected, got %v", err)
	}
}

func TestValidateEditPlanResultAcceptsMultipleServerSlotsForOneBeat(t *testing.T) {
	requirement := testEditRequirement("visual-1", "narration-1", "s001", "m001", 0, 3440)
	requirement.Slots = []EditPlanSlot{
		{ID: "s001", StartMs: 0, EndMs: 2640, DurationMs: 2640, Role: EditPlanSlotRolePrimary, Candidates: []EditPlanCandidate{{ID: "m001", SourceOutMs: 3000}}},
		{ID: "s002", StartMs: 2640, EndMs: 3440, DurationMs: 800, Role: EditPlanSlotRoleSupport, Candidates: []EditPlanCandidate{{ID: "m002", SourceOutMs: 1200}}},
	}
	err := ValidateEditPlanResult(EditPlanResult{Clips: []EditPlanClipChoice{
		{SlotID: "s001", CandidateID: "m001"},
		{SlotID: "s002", CandidateID: "m002"},
	}}, []EditPlanRequirement{requirement})
	if err != nil {
		t.Fatalf("expected deterministic multi-slot beat to be valid: %v", err)
	}
}

func TestValidateEditPlanInputRejectsCandidateShorterThanSlot(t *testing.T) {
	input := editPlannerTestInput()
	input.Requirements[0].Slots[0].Candidates[0].SourceOutMs = 1000
	err := validateEditPlanInput(input)
	if err == nil || !strings.Contains(err.Error(), "cannot cover its duration") {
		t.Fatalf("expected short candidate to be rejected, got %v", err)
	}
}

func testEditRequirement(visualBeatID string, narrationID string, slotID string, candidateID string, startMs int, endMs int) EditPlanRequirement {
	return EditPlanRequirement{
		VisualBeatID: visualBeatID, NarrationSegmentID: narrationID, StartMs: startMs, EndMs: endMs,
		DurationClass: VisualDurationClassStandard, NarrationText: "旁白", Label: "展示", VisualGoal: "展示产品", SourceType: TTSVisualSourceType,
		Slots: []EditPlanSlot{{
			ID: slotID, StartMs: startMs, EndMs: endMs, DurationMs: endMs - startMs, Role: EditPlanSlotRolePrimary,
			Candidates: []EditPlanCandidate{{ID: candidateID, SourceInMs: 0, SourceOutMs: endMs - startMs}},
		}},
	}
}
