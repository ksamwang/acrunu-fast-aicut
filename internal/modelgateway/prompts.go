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
		"asset_id=%s source_type=%s duration_ms=%d resolution=%dx%d has_audio=%t frames=%d. Frame timestamps are in milliseconds.",
		input.AssetID,
		input.SourceType,
		input.DurationMs,
		input.Width,
		input.Height,
		input.HasAudio,
		len(input.FrameSnapshots),
	)
	productContext := ""
	if input.SourceType == "visual_only" && input.ProductName != "" {
		productContext = fmt.Sprintf(
			" product_name=%q is weak visual context only. Do not assume the product is visible because of product_name; visible_product must be based only on visual evidence.",
			input.ProductName,
		)
	}
	referenceContext := ""
	if input.ProductReferenceImage != nil && input.ProductReferenceImage.StorageKey != "" {
		referenceContext = " A product reference image may be provided after the video frames. It is only for identifying whether the same product appears in the video frames. Do not describe the reference image as part of the scene. visible_product must be true only when the product is visible in the video frames."
	}

	return PromptBundle{
		Version: PromptVersion,
		Schema:  AnalyzeAssetOutputSchema(),
		Prompts: []PromptSpec{
			{
				Name:   "vlm_label",
				System: "You label short-video material for retrieval and automatic editing. Return only one valid JSON object. Do not include markdown.",
				User: "Analyze the provided frames for the current trim range. " +
					"Return JSON with exactly these keys: scene_description, shot_size, camera_movement, visual_tags, quality_tags, visible_product, product_position, scene_context, action_description, people_presence, face_visible, lighting_condition. " +
					"shot_size enum: close_up, medium_close_up, medium_shot, wide_shot. camera_movement enum: static, slow_push_in, pan, handheld. " +
					"Use concise Chinese values for descriptions/tags where possible. " + contextLine + productContext + referenceContext,
			},
		},
	}
}
