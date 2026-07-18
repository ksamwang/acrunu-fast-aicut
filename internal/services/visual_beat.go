package services

const (
	VisualBeatDurationLegacy   = "legacy"
	VisualBeatDurationBrief    = "brief"
	VisualBeatDurationStandard = "standard"
	VisualBeatDurationAction   = "action"
)

type VisualBeat struct {
	ID                 string `json:"id"`
	NarrationSegmentID string `json:"narration_segment_id"`
	StartMs            int    `json:"start_ms"`
	EndMs              int    `json:"end_ms"`
	DurationClass      string `json:"duration_class"`
	Label              string `json:"label"`
	SellingPoint       string `json:"selling_point,omitempty"`
	VisualGoal         string `json:"visual_goal"`
	SourceType         string `json:"source_type"`
}

func (b VisualBeat) DurationMs() int {
	return b.EndMs - b.StartMs
}

func normalizeVisualBeatDurationClass(value string) string {
	switch value {
	case "", VisualBeatDurationLegacy:
		return VisualBeatDurationLegacy
	case VisualBeatDurationBrief, VisualBeatDurationStandard, VisualBeatDurationAction:
		return value
	default:
		return value
	}
}

func isVisualBeatDurationClass(value string) bool {
	switch value {
	case VisualBeatDurationLegacy, VisualBeatDurationBrief, VisualBeatDurationStandard, VisualBeatDurationAction:
		return true
	default:
		return false
	}
}

func isVisualBeatDurationValid(class string, durationMs int) bool {
	switch class {
	case VisualBeatDurationLegacy:
		return durationMs > 0
	case VisualBeatDurationBrief:
		return durationMs >= 1000 && durationMs <= 1800
	case VisualBeatDurationStandard:
		return durationMs >= 1800 && durationMs <= 4500
	case VisualBeatDurationAction:
		return durationMs >= 2800 && durationMs <= 6000
	default:
		return false
	}
}
