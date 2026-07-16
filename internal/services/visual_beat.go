package services

type VisualBeat struct {
	ID                 string `json:"id"`
	NarrationSegmentID string `json:"narration_segment_id"`
	StartMs            int    `json:"start_ms"`
	EndMs              int    `json:"end_ms"`
	Label              string `json:"label"`
	SellingPoint       string `json:"selling_point,omitempty"`
	VisualGoal         string `json:"visual_goal"`
	SourceType         string `json:"source_type"`
}

func (b VisualBeat) DurationMs() int {
	return b.EndMs - b.StartMs
}
