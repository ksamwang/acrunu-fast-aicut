package services

import (
	"slices"
	"strings"
	"testing"

	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

func TestSplitNarrationSynthesisUnitsUsesSemanticPunctuation(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   []string
	}{
		{
			name:   "rain question",
			script: "骑行途中碰到雨水，车包没有防水设计怎么行？",
			want:   []string{"骑行途中碰到雨水，", "车包没有防水设计怎么行？"},
		},
		{
			name:   "shoulder carry",
			script: "肩背设计支持日常斜挎，骑行和普通出门都能派上用场。",
			want:   []string{"肩背设计支持日常斜挎，", "骑行和普通出门都能派上用场。"},
		},
		{
			name:   "short related clauses remain together",
			script: "容量大，带隔层。防水面料搭配压胶拉链，突发小雨不用慌。",
			want:   []string{"容量大，带隔层。", "防水面料搭配压胶拉链，", "突发小雨不用慌。"},
		},
		{
			name:   "distinct short product actions",
			script: "一秒快拆，大面积魔术贴，收纳方便，不占空间。",
			want:   []string{"一秒快拆，", "大面积魔术贴，", "收纳方便，", "不占空间。"},
		},
		{
			name:   "enumeration comma is not a speech boundary",
			script: "面料防水、防尘、耐磨，日常使用更省心。",
			want:   []string{"面料防水、防尘、耐磨，", "日常使用更省心。"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			units := splitNarrationSynthesisUnits(tt.script)
			got := make([]string, len(units))
			for index := range units {
				got[index] = units[index].Text
				if index < len(units)-1 && units[index].PauseAfterMs <= 0 {
					t.Fatalf("unit %d has no deterministic pause: %#v", index+1, units)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("synthesis units = %#v, want %#v", got, tt.want)
			}
			if units[len(units)-1].PauseAfterMs != 0 {
				t.Fatalf("final unit must not append silence: %#v", units)
			}
		})
	}
}

func TestSplitNarrationSynthesisUnitsBoundsProtocolSize(t *testing.T) {
	script := strings.Repeat("甲。", maximumNarrationSynthesisUnits+50)
	units := splitNarrationSynthesisUnits(script)
	if len(units) > maximumNarrationSynthesisUnits {
		t.Fatalf("synthesis unit count %d exceeds protocol limit", len(units))
	}
	var combined strings.Builder
	for _, unit := range units {
		combined.WriteString(unit.Text)
	}
	if combined.String() != script {
		t.Fatal("compacting synthesis units changed approved text")
	}
}

func TestNarrationTimingKeepsApprovedClausesInsideSynthesisUnits(t *testing.T) {
	script := "肩背设计支持日常斜挎，骑行和普通出门都能派上用场。"
	planned := splitNarrationSynthesisUnits(script)
	units, err := materializeSynthesizedNarrationUnits(planned, []modelgateway.CosyVoiceSynthesisUnitResult{
		{SpeechSamples: 800, TotalSamples: 920},
		{SpeechSamples: 1000, TotalSamples: 1000},
	}, 1000, 1920)
	if err != nil {
		t.Fatalf("materialize synthesis timing: %v", err)
	}
	segments, err := normalizeNarrationSegmentsWithSynthesisUnits([]modelgateway.ASRTranscriptSegment{
		{StartMs: 0, EndMs: 1100, Text: "肩背设计支持直接斜挎骑行"},
		{StartMs: 1100, EndMs: 1850, Text: "和普通出门都能派上用场"},
	}, script, 1920, units)
	if err != nil {
		t.Fatalf("normalize narration timing: %v", err)
	}
	if len(segments) != 2 || segments[0].Text != "肩背设计支持日常斜挎，" || segments[1].Text != "骑行和普通出门都能派上用场。" {
		t.Fatalf("approved text was not retained %#v", segments)
	}
	if segments[0].StartMs != 0 || segments[0].EndMs != 920 || segments[1].StartMs != 920 || segments[1].EndMs != 1920 {
		t.Fatalf("ASR moved a synthesis boundary %#v", segments)
	}
	if segments[0].SynthesisUnitIndex == nil || *segments[0].SynthesisUnitIndex != 0 || segments[1].SynthesisUnitIndex == nil || *segments[1].SynthesisUnitIndex != 1 {
		t.Fatalf("synthesis unit association is missing %#v", segments)
	}
}

func TestVisualPlanningGroupsCaptionsBySynthesisUnit(t *testing.T) {
	unitZero, unitOne := 0, 1
	segments, safeBoundaries := visualPlanningNarrationSegments([]NarrationSegment{
		{ID: "caption-1", StartMs: 0, EndMs: 500, Text: "一秒快拆，", SynthesisUnitIndex: &unitZero},
		{ID: "caption-2", StartMs: 500, EndMs: 1100, Text: "大面积魔术贴。", SynthesisUnitIndex: &unitZero},
		{ID: "caption-3", StartMs: 1100, EndMs: 2200, Text: "收纳方便。", SynthesisUnitIndex: &unitOne},
	})
	if len(segments) != 2 || segments[0].ID != "caption-1" || segments[0].Text != "一秒快拆，大面积魔术贴。" || segments[0].EndMs != 1100 {
		t.Fatalf("captions were not grouped by synthesis unit %#v", segments)
	}
	if !slices.Equal(safeBoundaries, []int{1100, 2200}) {
		t.Fatalf("unexpected safe pause boundaries %#v", safeBoundaries)
	}
}

func TestVisualTimelineDoesNotPadUnknownASRBoundaries(t *testing.T) {
	input := modelgateway.VisualPlanInput{
		ProductName: "束裤带",
		ScriptText:  "完整展示收纳。",
		NarrationSegments: []modelgateway.VisualPlanNarrationSegment{
			{ID: "caption-1", StartMs: 0, EndMs: 1000, Text: "完整展示收纳。"},
		},
	}
	result := modelgateway.VisualPlanResult{VisualBeats: []modelgateway.VisualPlanBeat{
		{NarrationSegmentID: "caption-1", StartMs: 0, EndMs: 1000, DurationClass: modelgateway.VisualDurationClassAction, Label: "收纳", VisualGoal: "完整展示放入口袋", SourceType: "visual_only"},
	}}
	beats, _, pauses, durationMs, err := materializeVisualTimeline(result, input, nil)
	if err != nil {
		t.Fatalf("materialize legacy timeline: %v", err)
	}
	if len(pauses) != 0 || durationMs != 1000 || beats[0].EndMs != 1000 || beats[0].DurationClass != VisualBeatDurationBrief {
		t.Fatalf("unknown ASR boundary received unsafe silence %#v %#v", beats, pauses)
	}
}
