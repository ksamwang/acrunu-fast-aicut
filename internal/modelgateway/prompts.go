package modelgateway

import "fmt"

type PromptSpec struct {
	Name   string `json:"name"`
	System string `json:"system"`
	User   string `json:"user"`
}

type PromptBundle struct {
	Version string         `json:"version"`
	Schema  map[string]any `json:"schema"`
	Prompts []PromptSpec   `json:"prompts"`
}

const PromptVersion = "phase2-v1"

func BuildPromptBundle(input AnalyzeAssetInput) PromptBundle {
	contextLine := fmt.Sprintf(
		"asset_id=%s source_type=%s duration_ms=%d resolution=%dx%d has_audio=%t frames=%d",
		input.AssetID,
		input.SourceType,
		input.DurationMs,
		input.Width,
		input.Height,
		input.HasAudio,
		len(input.FrameSnapshots),
	)

	return PromptBundle{
		Version: PromptVersion,
		Schema:  AnalyzeAssetOutputSchema(),
		Prompts: []PromptSpec{
			{
				Name:   "scene_description",
				System: "Describe the visible scene in one concise sentence for short-video asset retrieval.",
				User:   "Focus on the dominant visual action and product context. " + contextLine,
			},
			{
				Name:   "shot_size",
				System: "Classify the primary shot size using one enum value.",
				User:   "Choose from close_up, medium_close_up, medium_shot, wide_shot. " + contextLine,
			},
			{
				Name:   "camera_movement",
				System: "Classify the primary camera movement using one enum value.",
				User:   "Choose from static, slow_push_in, pan, handheld. " + contextLine,
			},
			{
				Name:   "usability_status",
				System: "Judge whether the asset is usable for downstream editing.",
				User:   "Return usable, needs_review, or discarded based on visual/audio quality and completeness. " + contextLine,
			},
		},
	}
}
