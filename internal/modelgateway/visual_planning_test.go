package modelgateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidateVisualPlanResultUsesCompleteNarrationSegments(t *testing.T) {
	input := VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "骑车时裤脚总被链条蹭脏。固定后骑行更安心。",
		NarrationSegments: []VisualPlanNarrationSegment{
			{ID: "n-1", StartMs: 0, EndMs: 2000, Text: "骑车时裤脚总被链条蹭脏。"},
			{ID: "n-2", StartMs: 2000, EndMs: 4000, Text: "固定后骑行更安心。"},
		},
	}
	result := VisualPlanResult{VisualBeats: []VisualPlanBeat{
		{NarrationSegmentID: "n-1", StartMs: 0, EndMs: 2000, DurationClass: VisualDurationClassStandard, Label: "痛点", VisualGoal: "裤脚接近链条", SourceType: "visual_only"},
		{NarrationSegmentID: "n-2", StartMs: 2000, EndMs: 4000, DurationClass: VisualDurationClassStandard, Label: "固定结果", VisualGoal: "完整展示固定裤脚并开始骑行", SourceType: "visual_only"},
	}}
	if err := ValidateVisualPlanResult(result, input); err != nil {
		t.Fatalf("expected visual plan to be valid: %v", err)
	}
}

func TestValidateVisualPlanResultRepairsTimelineFromNarrationAnchors(t *testing.T) {
	input := VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "第一句。第二句。",
		NarrationSegments: []VisualPlanNarrationSegment{
			{ID: "n-1", StartMs: 0, EndMs: 1000, Text: "第一句。"},
			{ID: "n-2", StartMs: 1000, EndMs: 2000, Text: "第二句。"},
		},
	}
	result := VisualPlanResult{VisualBeats: []VisualPlanBeat{
		{NarrationSegmentID: "n-2", StartMs: 1050, EndMs: 1990, DurationClass: VisualDurationClassStandard, Label: "二", VisualGoal: "画面二", SourceType: "visual_only"},
		{NarrationSegmentID: "n-1", StartMs: 10, EndMs: 1100, DurationClass: VisualDurationClassStandard, Label: "一", VisualGoal: "画面一", SourceType: "visual_only"},
	}}
	if err := ValidateVisualPlanResult(result, input); err != nil {
		t.Fatalf("expected timeline arithmetic to be repaired: %v", err)
	}
	if result.VisualBeats[0].NarrationSegmentID != "n-1" || result.VisualBeats[0].StartMs != 0 || result.VisualBeats[0].EndMs != 1000 ||
		result.VisualBeats[1].NarrationSegmentID != "n-2" || result.VisualBeats[1].StartMs != 1000 || result.VisualBeats[1].EndMs != 2000 {
		t.Fatalf("unexpected normalized timeline %#v", result.VisualBeats)
	}

	invalidCases := [][]VisualPlanBeat{
		{
			{NarrationSegmentID: "n-1", DurationClass: VisualDurationClassBrief, Label: "一", VisualGoal: "画面一", SourceType: "visual_only"},
			{NarrationSegmentID: "n-1", DurationClass: VisualDurationClassBrief, Label: "二", VisualGoal: "画面二", SourceType: "visual_only"},
		},
		{{NarrationSegmentID: "n-2", DurationClass: VisualDurationClassStandard, Label: "一", VisualGoal: "画面一", SourceType: "visual_only"}},
	}
	for _, beats := range invalidCases {
		if err := ValidateVisualPlanResult(VisualPlanResult{VisualBeats: beats}, input); err == nil {
			t.Fatal("expected an ambiguous visual timeline to be rejected")
		}
	}
}

func TestValidateVisualPlanResultAllowsShortNarrationForActionPadding(t *testing.T) {
	input := VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "快拆设计，收纳方便。",
		NarrationSegments: []VisualPlanNarrationSegment{
			{ID: "n-1", StartMs: 0, EndMs: 1400, Text: "快拆设计，"},
			{ID: "n-2", StartMs: 1400, EndMs: 3000, Text: "收纳方便。"},
		},
	}
	err := ValidateVisualPlanResult(VisualPlanResult{VisualBeats: []VisualPlanBeat{{
		NarrationSegmentID: "n-1", StartMs: 0, EndMs: 1400, DurationClass: VisualDurationClassAction,
		Label: "快拆", VisualGoal: "完整展示拆下束裤带", SourceType: "visual_only",
	}, {
		NarrationSegmentID: "n-2", StartMs: 1400, EndMs: 3000, DurationClass: VisualDurationClassStandard,
		Label: "结束", VisualGoal: "展示收纳结果", SourceType: "visual_only",
	}}}, input)
	if err != nil {
		t.Fatalf("expected service-side action padding to allow short narration: %v", err)
	}
}

func TestValidateVisualPlanResultPromotesVisibleActionDuration(t *testing.T) {
	input := VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "高弹松紧带拉伸自如，",
		NarrationSegments: []VisualPlanNarrationSegment{{
			ID: "n-1", StartMs: 0, EndMs: 1807, Text: "高弹松紧带拉伸自如，",
		}},
	}
	result := VisualPlanResult{VisualBeats: []VisualPlanBeat{{
		NarrationSegmentID: "n-1", StartMs: 0, EndMs: 1807,
		DurationClass: VisualDurationClassStandard, Label: "弹力演示",
		VisualGoal: "展示高弹松紧带反复拉伸", SourceType: "visual_only",
	}}}
	if err := ValidateVisualPlanResult(result, input); err != nil {
		t.Fatalf("expected visible action class to be normalized: %v", err)
	}
	if result.VisualBeats[0].DurationClass != VisualDurationClassAction {
		t.Fatalf("expected visible action to use action duration, got %q", result.VisualBeats[0].DurationClass)
	}
}

func TestValidateVisualPlanResultAllowsHookLongerThanBriefGuideline(t *testing.T) {
	for _, durationMs := range []int{2320, 2230, 2094} {
		input := VisualPlanInput{
			ProductName: "束裤带",
			ScriptText:  "骑行水壶总没地方放？",
			NarrationSegments: []VisualPlanNarrationSegment{{
				ID: "n-1", StartMs: 0, EndMs: durationMs, Text: "骑行水壶总没地方放？",
			}},
		}
		result := VisualPlanResult{VisualBeats: []VisualPlanBeat{{
			NarrationSegmentID: "n-1", StartMs: 0, EndMs: durationMs,
			DurationClass: VisualDurationClassBrief, Label: "痛点钩子",
			VisualGoal: "展示骑行水壶无处固定", SourceType: "visual_only",
		}}}
		if err := ValidateVisualPlanResult(result, input); err != nil {
			t.Fatalf("expected %dms hook to keep its complete narration boundary: %v", durationMs, err)
		}
	}
}

func TestValidateVisualPlanResultAllowsSequentialWordingInOneBeat(t *testing.T) {
	input := VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "一秒快拆，收纳方便。",
		NarrationSegments: []VisualPlanNarrationSegment{
			{ID: "n-1", StartMs: 0, EndMs: 1000, Text: "一秒快拆，"},
			{ID: "n-2", StartMs: 1000, EndMs: 2000, Text: "收纳方便。"},
		},
	}
	err := ValidateVisualPlanResult(VisualPlanResult{VisualBeats: []VisualPlanBeat{{
		NarrationSegmentID: "n-1", StartMs: 0, EndMs: 2000, DurationClass: VisualDurationClassAction,
		Label: "快拆与收纳", VisualGoal: "快速拆卸，然后折叠收纳", SourceType: "visual_only",
	}}}, input)
	if err != nil {
		t.Fatalf("expected sequential wording to be handled by downstream multi-shot planning: %v", err)
	}
}

func TestValidateVisualPlanResultLimitsBriefCuts(t *testing.T) {
	input := VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "展示产品，展示细节，展示结果。",
		NarrationSegments: []VisualPlanNarrationSegment{
			{ID: "n-1", StartMs: 0, EndMs: 1200, Text: "展示产品，"},
			{ID: "n-2", StartMs: 1200, EndMs: 2400, Text: "展示细节，"},
			{ID: "n-3", StartMs: 2400, EndMs: 4000, Text: "展示结果。"},
		},
	}
	err := ValidateVisualPlanResult(VisualPlanResult{VisualBeats: []VisualPlanBeat{
		{NarrationSegmentID: "n-1", StartMs: 0, EndMs: 1200, DurationClass: VisualDurationClassBrief, Label: "短切一", VisualGoal: "产品外观", SourceType: "visual_only"},
		{NarrationSegmentID: "n-2", StartMs: 1200, EndMs: 2400, DurationClass: VisualDurationClassBrief, Label: "短切二", VisualGoal: "产品细节", SourceType: "visual_only"},
		{NarrationSegmentID: "n-3", StartMs: 2400, EndMs: 4000, DurationClass: VisualDurationClassBrief, Label: "短切三", VisualGoal: "产品结果", SourceType: "visual_only"},
	}}, input)
	if err == nil {
		t.Fatal("expected excessive brief cuts to be rejected")
	}
}

func TestValidateVisualPlanResultRejectsMissingNarrativeBeat(t *testing.T) {
	input := VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "魔术贴绑带，收纳方便，不占空间。",
		NarrationSegments: []VisualPlanNarrationSegment{
			{ID: "n-1", StartMs: 0, EndMs: 1200, Text: "魔术贴绑带，"},
			{ID: "n-2", StartMs: 1200, EndMs: 3000, Text: "收纳方便，不占空间。"},
		},
		NarrativeBeats: []VisualPlanNarrativeBeat{
			{ID: "business-velcro", Label: "魔术贴", VisualGoal: "展示魔术贴开合", SourceType: "visual_only"},
			{ID: "business-storage", Label: "收纳", VisualGoal: "展示折叠收纳", SourceType: "visual_only"},
		},
	}
	err := ValidateVisualPlanResult(VisualPlanResult{VisualBeats: []VisualPlanBeat{{
		NarrationSegmentID: "n-1", NarrativeBeatID: "business-velcro",
		StartMs: 0, EndMs: 3000, DurationClass: VisualDurationClassAction,
		Label: "魔术贴收纳", VisualGoal: "展示魔术贴开合及折叠收纳", SourceType: "visual_only",
	}}}, input)
	if err == nil || !strings.Contains(err.Error(), "business-storage") {
		t.Fatalf("expected omitted storage business intention to be rejected, got %v", err)
	}
}

func TestValidateVisualPlanResultAllowsMultipleAtomicVisualsForNarrativeBeat(t *testing.T) {
	input := VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "一物多用，能绑水壶和修车工具。",
		NarrationSegments: []VisualPlanNarrationSegment{
			{ID: "n-1", StartMs: 0, EndMs: 3000, Text: "一物多用，能绑水壶。"},
			{ID: "n-2", StartMs: 3000, EndMs: 5000, Text: "也能绑修车工具。"},
		},
		NarrativeBeats: []VisualPlanNarrativeBeat{{
			ID: "business-multiuse", Label: "一物多用", VisualGoal: "展示绑水壶和修车工具", SourceType: "visual_only",
		}},
	}
	result := VisualPlanResult{VisualBeats: []VisualPlanBeat{
		{NarrationSegmentID: "n-1", NarrativeBeatID: "business-multiuse", StartMs: 0, EndMs: 3000, DurationClass: VisualDurationClassAction, Label: "绑水壶", VisualGoal: "展示束裤带固定水壶", SourceType: "visual_only"},
		{NarrationSegmentID: "n-2", NarrativeBeatID: "business-multiuse", StartMs: 3000, EndMs: 5000, DurationClass: VisualDurationClassStandard, Label: "绑工具", VisualGoal: "展示束裤带固定修车工具的结果", SourceType: "visual_only"},
	}}
	if err := ValidateVisualPlanResult(result, input); err != nil {
		t.Fatalf("expected one business intention to allow multiple atomic visuals: %v", err)
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
		NarrationSegmentID: "n-1", StartMs: 0, EndMs: 1000, DurationClass: VisualDurationClassBrief, Label: "口播", VisualGoal: "人物口播展示", SourceType: "talking_head",
	}}}, input)
	if err == nil {
		t.Fatal("expected talking-head material to be rejected for TTS planning")
	}
}

func TestNormalizeVisualGoalForRetrievalRemovesNarrationOnlyPrefix(t *testing.T) {
	tests := map[string]string{
		"骑行结束后将束裤带折叠并放入口袋": "将束裤带折叠并放入口袋",
		"出门前将束裤带固定在裤脚处":    "将束裤带固定在裤脚处",
		"夜间骑行时展示束裤带反光效果":   "夜间骑行时展示束裤带反光效果",
	}
	for input, expected := range tests {
		if got := normalizeVisualGoalForRetrieval(input); got != expected {
			t.Fatalf("normalize %q: expected %q, got %q", input, expected, got)
		}
	}
}

func TestOpenAICompatibleEditPlannerPlansVisualBeats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"visual_beats\":[{\"narration_segment_id\":\"n-1\",\"narrative_beat_id\":\"\",\"start_ms\":0,\"end_ms\":1000,\"duration_class\":\"brief\",\"label\":\"展示\",\"selling_point\":\"\",\"visual_goal\":\"展示产品外观\",\"source_type\":\"visual_only\"}]}"}}]}`))
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
	if len(result.VisualBeats) != 1 || result.VisualBeats[0].VisualGoal != "展示产品外观" || result.VisualBeats[0].DurationClass != VisualDurationClassBrief {
		t.Fatalf("unexpected visual plan %#v", result)
	}
}
