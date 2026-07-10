package modelgateway

import (
	"fmt"
	"strings"
)

const OutputSchemaVersion = "phase2.asset_analysis.v1"

var allowedShotSizes = map[string]struct{}{
	"":                {},
	"close_up":        {},
	"medium_close_up": {},
	"medium_shot":     {},
	"wide_shot":       {},
}

var allowedCameraMovements = map[string]struct{}{
	"":             {},
	"static":       {},
	"slow_push_in": {},
	"pan":          {},
	"handheld":     {},
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
				"enum": []string{"close_up", "medium_close_up", "medium_shot", "wide_shot"},
			},
			"camera_movement": map[string]any{
				"type": "string",
				"enum": []string{"static", "slow_push_in", "pan", "handheld"},
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
