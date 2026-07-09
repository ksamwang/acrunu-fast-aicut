package modelgateway

import (
	"fmt"
	"strings"
)

const OutputSchemaVersion = "phase2.asset_analysis.v1"

var allowedUsabilityStatuses = map[string]struct{}{
	"usable":       {},
	"needs_review": {},
	"discarded":    {},
}

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
			"usability_status",
			"scene_description",
			"shot_size",
			"camera_movement",
			"subjects",
			"scene_tags",
			"quality_tags",
		},
		"properties": map[string]any{
			"usability_status": map[string]any{
				"type": "string",
				"enum": []string{"usable", "needs_review", "discarded"},
			},
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
			"subjects": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"scene_tags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"quality_tags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
	}
}

func ValidateAnalyzeAssetResult(result AnalyzeAssetResult) error {
	if _, ok := allowedUsabilityStatuses[result.UsabilityStatus]; !ok {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("invalid usability_status: %s", result.UsabilityStatus), false, nil)
	}
	if _, ok := allowedShotSizes[result.ShotSize]; !ok {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("invalid shot_size: %s", result.ShotSize), false, nil)
	}
	if _, ok := allowedCameraMovements[result.CameraMovement]; !ok {
		return NewError(ErrorCodeInvalidResponse, fmt.Sprintf("invalid camera_movement: %s", result.CameraMovement), false, nil)
	}
	if strings.TrimSpace(result.SceneDescription) == "" {
		return NewError(ErrorCodeInvalidResponse, "scene_description is required", false, nil)
	}
	return nil
}
