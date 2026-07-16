package modelgateway

import (
	"context"
	"fmt"
	"strings"
)

const (
	minimumVisualBeatDurationMs = 800
	maximumVisualBeatDurationMs = 3000
)

type VisualPlanNarrationSegment struct {
	ID      string `json:"id"`
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
	Text    string `json:"text"`
}

type VisualPlanNarrativeBeat struct {
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
	StartMs            int    `json:"start_ms"`
	EndMs              int    `json:"end_ms"`
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
	for _, segment := range input.NarrationSegments {
		segments[segment.ID] = segment
	}
	expectedStart := input.NarrationSegments[0].StartMs
	for index := range result.VisualBeats {
		beat := &result.VisualBeats[index]
		beat.NarrationSegmentID = strings.TrimSpace(beat.NarrationSegmentID)
		beat.Label = strings.TrimSpace(beat.Label)
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
		if beat.StartMs != expectedStart || beat.EndMs <= beat.StartMs {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d does not continue the timeline", index+1), false, nil)
		}
		if beat.StartMs < segment.StartMs || beat.EndMs > segment.EndMs {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d crosses its narration segment", index+1), false, nil)
		}
		durationMs := beat.EndMs - beat.StartMs
		if durationMs > maximumVisualBeatDurationMs {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d is longer than %dms", index+1, maximumVisualBeatDurationMs), false, nil)
		}
		if segment.EndMs-segment.StartMs >= minimumVisualBeatDurationMs && durationMs < minimumVisualBeatDurationMs {
			return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("visual beat %d is shorter than %dms", index+1, minimumVisualBeatDurationMs), false, nil)
		}
		expectedStart = beat.EndMs
	}
	if expectedStart != input.NarrationSegments[len(input.NarrationSegments)-1].EndMs {
		return NewError(ErrorCodeInvalidResponse, "visual beats do not cover the narration timeline", false, nil)
	}
	return nil
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
	return nil
}

func isVisualPlanSourceType(value string) bool {
	return value == "visual_only" || value == "talking_head" || value == "mixed"
}
