package modelgateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateVisualPlanResultAllowsMultipleBeatsWithinNarrationSegment(t *testing.T) {
	input := VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "骑车时裤脚总被链条蹭脏。固定后骑行更安心。",
		NarrationSegments: []VisualPlanNarrationSegment{
			{ID: "n-1", StartMs: 0, EndMs: 2000, Text: "骑车时裤脚总被链条蹭脏。"},
			{ID: "n-2", StartMs: 2000, EndMs: 3500, Text: "固定后骑行更安心。"},
		},
	}
	result := VisualPlanResult{VisualBeats: []VisualPlanBeat{
		{NarrationSegmentID: "n-1", StartMs: 0, EndMs: 1000, Label: "痛点", VisualGoal: "裤脚接近链条", SourceType: "visual_only"},
		{NarrationSegmentID: "n-1", StartMs: 1000, EndMs: 2000, Label: "脏污", VisualGoal: "展示被蹭脏的裤脚", SourceType: "visual_only"},
		{NarrationSegmentID: "n-2", StartMs: 2000, EndMs: 3500, Label: "结果", VisualGoal: "展示固定后骑行", SourceType: "visual_only"},
	}}
	if err := ValidateVisualPlanResult(result, input); err != nil {
		t.Fatalf("expected visual plan to be valid: %v", err)
	}
}

func TestValidateVisualPlanResultRejectsInvalidTimeline(t *testing.T) {
	input := VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "第一句。第二句。",
		NarrationSegments: []VisualPlanNarrationSegment{
			{ID: "n-1", StartMs: 0, EndMs: 1000, Text: "第一句。"},
			{ID: "n-2", StartMs: 1000, EndMs: 2000, Text: "第二句。"},
		},
	}
	cases := []struct {
		name  string
		beats []VisualPlanBeat
	}{
		{
			name: "gap",
			beats: []VisualPlanBeat{
				{NarrationSegmentID: "n-1", StartMs: 0, EndMs: 900, Label: "一", VisualGoal: "画面一", SourceType: "visual_only"},
				{NarrationSegmentID: "n-2", StartMs: 950, EndMs: 2000, Label: "二", VisualGoal: "画面二", SourceType: "visual_only"},
			},
		},
		{
			name: "overlap",
			beats: []VisualPlanBeat{
				{NarrationSegmentID: "n-1", StartMs: 0, EndMs: 1000, Label: "一", VisualGoal: "画面一", SourceType: "visual_only"},
				{NarrationSegmentID: "n-2", StartMs: 900, EndMs: 2000, Label: "二", VisualGoal: "画面二", SourceType: "visual_only"},
			},
		},
		{
			name: "crosses parent narration segment",
			beats: []VisualPlanBeat{
				{NarrationSegmentID: "n-1", StartMs: 0, EndMs: 1200, Label: "一", VisualGoal: "画面一", SourceType: "visual_only"},
				{NarrationSegmentID: "n-2", StartMs: 1200, EndMs: 2000, Label: "二", VisualGoal: "画面二", SourceType: "visual_only"},
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := ValidateVisualPlanResult(VisualPlanResult{VisualBeats: testCase.beats}, input); err == nil {
				t.Fatal("expected invalid visual timeline to be rejected")
			}
		})
	}
}

func TestValidateVisualPlanResultRejectsNonVisualOnlyMaterial(t *testing.T) {
	input := VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "固定裤脚。",
		NarrationSegments: []VisualPlanNarrationSegment{{
			ID: "n-1", StartMs: 0, EndMs: 1000, Text: "固定裤脚。",
		}},
	}
	err := ValidateVisualPlanResult(VisualPlanResult{VisualBeats: []VisualPlanBeat{{
		NarrationSegmentID: "n-1", StartMs: 0, EndMs: 1000, Label: "口播", VisualGoal: "人物口播展示", SourceType: "talking_head",
	}}}, input)
	if err == nil {
		t.Fatal("expected talking-head material to be rejected for TTS planning")
	}
}

func TestOpenAICompatibleEditPlannerPlansVisualBeats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"visual_beats\":[{\"narration_segment_id\":\"n-1\",\"start_ms\":0,\"end_ms\":1000,\"label\":\"展示\",\"selling_point\":\"\",\"visual_goal\":\"展示产品动作\",\"source_type\":\"visual_only\"}]}"}}]}`))
	}))
	defer server.Close()
	planner := NewOpenAICompatibleEditPlanner(Config{Provider: "openai_compatible", BaseURL: server.URL, Model: "planner-model", Timeout: time.Second})
	result, err := planner.PlanVisuals(context.Background(), VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "固定裤脚。",
		NarrationSegments: []VisualPlanNarrationSegment{{
			ID: "n-1", StartMs: 0, EndMs: 1000, Text: "固定裤脚。",
		}},
	})
	if err != nil {
		t.Fatalf("plan visuals: %v", err)
	}
	if len(result.VisualBeats) != 1 || result.VisualBeats[0].VisualGoal != "展示产品动作" {
		t.Fatalf("unexpected visual plan %#v", result)
	}
}
