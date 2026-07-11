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
			" Target product name: %q. Use it with the reference image, when provided, to identify the target product in the video frames.",
			input.ProductName,
		)
	}
	referenceContext := ""
	if input.ProductReferenceImage != nil && input.ProductReferenceImage.StorageKey != "" {
		referenceContext = " A product reference image is provided after the video frames. It defines the target product. The target product may appear in a different color, angle, scale, or installed/attached usage state. Use the reference image to recognize the target product in the video frames, but do not describe the reference image itself as scene content."
	}
	targetProductRules := ""
	if productContext != "" || referenceContext != "" {
		targetProductRules = " Prioritize describing the target product's visible usage or installation state in scene_description and action_description. Do not confuse the target product with carrier/background objects such as bottle, bicycle, table, packaging, wall, or hand. visible_product means the target product is visible in the video frames, not merely that any product-like object exists. product_position should describe the target product position or attachment relationship in the video frame; use not_visible when the target product is not visible. visual_tags should include target-product tags when visible, plus useful scene tags."
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
					"Use concise Chinese values for descriptions/tags where possible. " + contextLine + productContext + referenceContext + targetProductRules,
			},
		},
	}
}
