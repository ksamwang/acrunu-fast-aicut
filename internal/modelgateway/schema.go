package modelgateway

import (
	"fmt"
	"strings"
)

const OutputSchemaVersion = "phase2.asset_analysis.v2"
const ScriptGenerationOutputSchemaVersion = "workbench.script_generation.v2"
const EditPlanOutputSchemaVersion = "workbench.edit_plan.v3"

var allowedShotSizes = map[string]struct{}{
	"":                {},
	"close_up":        {},
	"medium_close_up": {},
	"medium_shot":     {},
	"wide_shot":       {},
	"full_shot":       {},
}

var allowedCameraMovements = map[string]struct{}{
	"":             {},
	"static":       {},
	"pan":          {},
	"tilt":         {},
	"push_in":      {},
	"pull_out":     {},
	"tracking":     {},
	"orbit":        {},
	"zoom":         {},
	"handheld":     {},
	"mixed":        {},
	"unknown":      {},
	"slow_push_in": {}, // Legacy value kept for previously stored/provider results.
}

func AnalyzeAssetOutputSchema() map[string]any {
	return map[string]any{
		"version": OutputSchemaVersion,
		"type":    "object",
		"required": []string{
			"scene_description",
			"shot_size",
			"camera_movement",
			"visual_tags",
			"quality_tags",
			"visible_product",
			"product_position",
			"scene_context",
			"action_description",
			"people_presence",
			"face_visible",
			"lighting_condition",
		},
		"properties": map[string]any{
			"scene_description": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"shot_size": map[string]any{
				"type": "string",
				"enum": []string{"wide_shot", "full_shot", "medium_shot", "medium_close_up", "close_up"},
			},
			"camera_movement": map[string]any{
				"type": "string",
				"enum": []string{"static", "pan", "tilt", "push_in", "pull_out", "tracking", "orbit", "zoom", "handheld", "mixed", "unknown"},
			},
			"visual_tags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"quality_tags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"visible_product":    map[string]any{"type": "boolean"},
			"product_position":   map[string]any{"type": "string"},
			"scene_context":      map[string]any{"type": "string"},
			"action_description": map[string]any{"type": "string"},
			"people_presence":    map[string]any{"type": "boolean"},
			"face_visible":       map[string]any{"type": "boolean"},
			"lighting_condition": map[string]any{"type": "string"},
		},
	}
}

func ValidateAnalyzeAssetResult(result AnalyzeAssetResult) error {
	if _, ok := allowedShotSizes[result.ShotSize]; !ok {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("invalid shot_size: %s", result.ShotSize), false, nil)
	}
	if _, ok := allowedCameraMovements[result.CameraMovement]; !ok {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("invalid camera_movement: %s", result.CameraMovement), false, nil)
	}
	if strings.TrimSpace(result.SceneDescription) == "" {
		return NewError(ErrorCodeInvalidResponse, "scene_description is required", false, nil)
	}
	if len(result.VisualTags) == 0 {
		return NewError(ErrorCodeInvalidResponse, "visual_tags is required", false, nil)
	}
	if strings.TrimSpace(result.SceneContext) == "" {
		return NewError(ErrorCodeInvalidResponse, "scene_context is required", false, nil)
	}
	if strings.TrimSpace(result.ActionDescription) == "" {
		return NewError(ErrorCodeInvalidResponse, "action_description is required", false, nil)
	}
	return nil
}

func ScriptGenerationOutputSchema() map[string]any {
	return map[string]any{
		"version": ScriptGenerationOutputSchemaVersion,
		"type":    "object",
		"required": []string{
			"variants",
		},
		"properties": map[string]any{
			"variants": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"hook", "script_text", "editing_intent", "beats"},
					"properties": map[string]any{
						"hook":           map[string]any{"type": "string", "minLength": 1},
						"script_text":    map[string]any{"type": "string", "minLength": 1},
						"editing_intent": map[string]any{"type": "string", "minLength": 1},
						"beats": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type":     "object",
								"required": []string{"label", "selling_point", "visual_goal", "source_type"},
								"properties": map[string]any{
									"label":         map[string]any{"type": "string", "minLength": 1},
									"selling_point": map[string]any{"type": "string", "minLength": 1},
									"visual_goal":   map[string]any{"type": "string", "minLength": 1},
									"source_type": map[string]any{
										"type": "string",
										"enum": []string{TTSVisualSourceType},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func EditPlanOutputSchema() map[string]any {
	return map[string]any{
		"version": EditPlanOutputSchemaVersion,
		"type":    "object",
		"required": []string{
			"clips",
		},
		"properties": map[string]any{
			"clips": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"slot_id", "candidate_id"},
					"properties": map[string]any{
						"slot_id":      map[string]any{"type": "string", "minLength": 1},
						"candidate_id": map[string]any{"type": "string", "minLength": 1},
					},
				},
			},
		},
	}
}

func VisualPlanOutputSchema() map[string]any {
	return map[string]any{
		"version": VisualPlanPromptVersion,
		"type":    "object",
		"required": []string{
			"visual_beats",
		},
		"properties": map[string]any{
			"visual_beats": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"narration_segment_id", "narrative_beat_id", "start_ms", "end_ms", "duration_class", "label", "selling_point", "visual_goal", "source_type"},
					"properties": map[string]any{
						"narration_segment_id": map[string]any{"type": "string", "minLength": 1},
						"narrative_beat_id":    map[string]any{"type": "string"},
						"start_ms":             map[string]any{"type": "integer", "minimum": 0},
						"end_ms":               map[string]any{"type": "integer", "minimum": 1},
						"duration_class": map[string]any{
							"type": "string",
							"enum": []string{VisualDurationClassBrief, VisualDurationClassStandard, VisualDurationClassAction},
						},
						"label":         map[string]any{"type": "string", "minLength": 1},
						"selling_point": map[string]any{"type": "string"},
						"visual_goal":   map[string]any{"type": "string", "minLength": 1},
						"source_type": map[string]any{
							"type": "string",
							"enum": []string{TTSVisualSourceType},
						},
					},
				},
			},
		},
	}
}
