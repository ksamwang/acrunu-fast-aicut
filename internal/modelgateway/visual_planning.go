package modelgateway

import (
	"context"
	"fmt"
	"strings"
)

const (
	VisualDurationClassBrief    = "brief"
	VisualDurationClassStandard = "standard"
	VisualDurationClassAction   = "action"

	actionVisualBeatMinimumMs = 2800
	briefVisualBeatIntervalMs = 8000
)

type VisualPlanNarrationSegment struct {
	ID      string `json:"id"`
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
	Text    string `json:"text"`
}

type VisualPlanNarrativeBeat struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	SellingPoint string `json:"selling_point"`
	VisualGoal   string `json:"visual_goal"`
	SourceType   string `json:"source_type"`
}

type VisualPlanInput struct {
	ProductName       string                       `json:"product_name"`
	ScriptText        string                       `json:"script_text"`
	EditingIntent     string                       `json:"editing_intent"`
	NarrationSegments []VisualPlanNarrationSegment `json:"narration_segments"`
	NarrativeBeats    []VisualPlanNarrativeBeat    `json:"narrative_beats"`
}

type VisualPlanBeat struct {
	NarrationSegmentID string `json:"narration_segment_id"`
	NarrativeBeatID    string `json:"narrative_beat_id"`
	StartMs            int    `json:"start_ms"`
	EndMs              int    `json:"end_ms"`
	DurationClass      string `json:"duration_class"`
	Label              string `json:"label"`
	SellingPoint       string `json:"selling_point"`
	VisualGoal         string `json:"visual_goal"`
	SourceType         string `json:"source_type"`
}

type VisualPlanResult struct {
	VisualBeats []VisualPlanBeat `json:"visual_beats"`
}

type VisualPlanner interface {
	PlanVisuals(context.Context, VisualPlanInput) (VisualPlanResult, error)
}

type visualPlannerFunc func(context.Context, VisualPlanInput) (VisualPlanResult, error)

func (f visualPlannerFunc) PlanVisuals(ctx context.Context, input VisualPlanInput) (VisualPlanResult, error) {
	return f(ctx, input)
}

func NewVisualPlanner(cfg Config) VisualPlanner {
	switch strings.TrimSpace(cfg.Provider) {
	case "openai_compatible":
		return NewOpenAICompatibleEditPlanner(cfg)
	default:
		provider := strings.TrimSpace(cfg.Provider)
		if provider == "" {
			provider = "openai_compatible"
		}
		return visualPlannerFunc(func(context.Context, VisualPlanInput) (VisualPlanResult, error) {
			return VisualPlanResult{}, NewError(
				ErrorCodeUnsupportedProvider,
				fmt.Sprintf("llm provider %q is not implemented", provider),
				false,
				nil,
			)
		})
	}
}

func (p *OpenAICompatibleEditPlanner) PlanVisuals(ctx context.Context, input VisualPlanInput) (VisualPlanResult, error) {
	if p == nil || p.baseURL == "" {
		return VisualPlanResult{}, NewError(ErrorCodeConfiguration, "openai compatible base_url is required", false, nil)
	}
	if p.model == "" {
		return VisualPlanResult{}, NewError(ErrorCodeConfiguration, "llm model is required", false, nil)
	}
	if err := validateVisualPlanInput(input); err != nil {
		return VisualPlanResult{}, err
	}

	var result VisualPlanResult
	if err := p.completeJSON(ctx, BuildVisualPlanPrompt(input), &result); err != nil {
		return VisualPlanResult{}, err
	}
	if err := ValidateVisualPlanResult(result, input); err != nil {
		return VisualPlanResult{}, err
	}
	return result, nil
}

func ValidateVisualPlanResult(result VisualPlanResult, input VisualPlanInput) error {
	if len(result.VisualBeats) == 0 {
		return NewError(ErrorCodeInvalidResponse, "visual plan is empty", false, nil)
	}
	if err := validateVisualPlanInput(input); err != nil {
		return err
	}
	segments := make(map[string]VisualPlanNarrationSegment, len(input.NarrationSegments))
	segmentEnds := make(map[int]struct{}, len(input.NarrationSegments))
	for _, segment := range input.NarrationSegments {
		segments[segment.ID] = segment
		segmentEnds[segment.EndMs] = struct{}{}
	}
	narrativeBeats := make(map[string]struct{}, len(input.NarrativeBeats))
	for _, beat := range input.NarrativeBeats {
		narrativeBeats[beat.ID] = struct{}{}
	}
	coveredNarrativeBeats := make(map[string]struct{}, len(input.NarrativeBeats))
	expectedStart := input.NarrationSegments[0].StartMs
	timelineEnd := input.NarrationSegments[len(input.NarrationSegments)-1].EndMs
	briefBeatCount := 0
	for index := range result.VisualBeats {
		beat := &result.VisualBeats[index]
		beat.NarrationSegmentID = strings.TrimSpace(beat.NarrationSegmentID)
		beat.NarrativeBeatID = strings.TrimSpace(beat.NarrativeBeatID)
		beat.Label = strings.TrimSpace(beat.Label)
		beat.DurationClass = strings.TrimSpace(beat.DurationClass)
		beat.SellingPoint = strings.TrimSpace(beat.SellingPoint)
		beat.VisualGoal = strings.TrimSpace(beat.VisualGoal)
		beat.SourceType = strings.TrimSpace(beat.SourceType)
		segment, ok := segments[beat.NarrationSegmentID]
		if !ok {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d references an unknown narration segment", index+1), false, nil)
		}
		if beat.Label == "" || beat.VisualGoal == "" || !isVisualPlanSourceType(beat.SourceType) {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d is incomplete", index+1), false, nil)
		}
		if containsSequentialVisualActions(beat.VisualGoal) {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d combines sequential actions", index+1), false, nil)
		}
		if visualGoalRequiresActionDuration(beat.VisualGoal) && beat.DurationClass != VisualDurationClassAction {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d must use action duration", index+1), false, nil)
		}
		if beat.NarrativeBeatID != "" {
			if _, ok := narrativeBeats[beat.NarrativeBeatID]; !ok {
				return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d references an unknown narrative beat", index+1), false, nil)
			}
			coveredNarrativeBeats[beat.NarrativeBeatID] = struct{}{}
		}
		if beat.StartMs != expectedStart || beat.EndMs <= beat.StartMs {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d does not continue the timeline", index+1), false, nil)
		}
		if beat.StartMs != segment.StartMs {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d must start at its narration segment boundary", index+1), false, nil)
		}
		if _, ok := segmentEnds[beat.EndMs]; !ok {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d must end at a narration segment boundary", index+1), false, nil)
		}
		if !isVisualDurationClass(beat.DurationClass) {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d duration class is invalid", index+1), false, nil)
		}
		if beat.DurationClass == VisualDurationClassBrief {
			briefBeatCount++
		}
		expectedStart = beat.EndMs
	}
	if expectedStart != timelineEnd {
		return NewError(ErrorCodeInvalidResponse, "visual beats do not cover the narration timeline", false, nil)
	}
	maximumBriefBeats := (timelineEnd + briefVisualBeatIntervalMs - 1) / briefVisualBeatIntervalMs
	if briefBeatCount > maximumBriefBeats {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual plan has %d brief beats, maximum is %d", briefBeatCount, maximumBriefBeats), false, nil)
	}
	for _, beat := range input.NarrativeBeats {
		if _, ok := coveredNarrativeBeats[beat.ID]; !ok {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual plan does not cover narrative beat %q", beat.ID), false, nil)
		}
	}
	return nil
}

func containsSequentialVisualActions(value string) bool {
	for _, marker := range []string{"然后", "随后", "接着", "再将", "再把"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func visualGoalRequiresActionDuration(value string) bool {
	for _, marker := range []string{
		"放入口袋", "塞入口袋", "折叠收纳", "拉伸", "撕开", "重新粘贴",
		"环绕并固定", "缠绕并固定", "安装过程", "调节过程", "佩戴过程",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func isVisualDurationClass(class string) bool {
	return class == VisualDurationClassBrief || class == VisualDurationClassStandard || class == VisualDurationClassAction
}

func validateVisualPlanInput(input VisualPlanInput) error {
	if strings.TrimSpace(input.ProductName) == "" || strings.TrimSpace(input.ScriptText) == "" {
		return NewError(ErrorCodeConfiguration, "product name and script text are required", false, nil)
	}
	if len(input.NarrationSegments) == 0 {
		return NewError(ErrorCodeConfiguration, "narration segments are required", false, nil)
	}
	previousEnd := 0
	for index, segment := range input.NarrationSegments {
		if strings.TrimSpace(segment.ID) == "" || strings.TrimSpace(segment.Text) == "" || segment.StartMs != previousEnd || segment.EndMs <= segment.StartMs {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("narration segment %d is invalid", index+1), false, nil)
		}
		previousEnd = segment.EndMs
	}
	seenNarrativeBeats := make(map[string]struct{}, len(input.NarrativeBeats))
	for index := range input.NarrativeBeats {
		beat := &input.NarrativeBeats[index]
		beat.ID = strings.TrimSpace(beat.ID)
		beat.Label = strings.TrimSpace(beat.Label)
		beat.VisualGoal = strings.TrimSpace(beat.VisualGoal)
		if beat.ID == "" || beat.Label == "" || beat.VisualGoal == "" {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("narrative beat %d is invalid", index+1), false, nil)
		}
		if _, exists := seenNarrativeBeats[beat.ID]; exists {
			return NewError(ErrorCodeConfiguration, fmt.Sprintf("narrative beat %q is repeated", beat.ID), false, nil)
		}
		seenNarrativeBeats[beat.ID] = struct{}{}
	}
	return nil
}

func isVisualPlanSourceType(value string) bool {
	return value == TTSVisualSourceType
}
