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

var lowValueSceneDescriptionPhrases = []string{
	"清晰可见",
	"完整可见",
	"完整展示",
	"持续展示",
	"静态展示",
	"保持展示状态",
	"保持固定可见",
	"展示整体外观",
	"产品完整展示",
}

var lowValueActionDescriptionPhrases = []string{
	"无明显操作",
	"持续展示",
	"静态展示",
	"未见明显操作",
	"未见操作",
	"未见拆装",
	"未见开合",
	"未见状态变化",
	"未发生明显操作",
	"未发生明显变化",
	"未发生操作变化",
	"保持静止",
	"状态无明显变化",
	"全程保持固定展示",
	"全程保持固定安装",
}

var humanEvidencePhrases = []string{
	"人物",
	"骑行者",
	"佩戴者",
	"双手",
	"手部",
	"手指",
	"手持",
	"手将",
	"手从",
	"手扶",
	"手托",
	"人手",
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
	issues := analyzeAssetResultIssues(result)
	if len(issues) == 0 {
		return nil
	}
	return NewError(ErrorCodeInvalidResponse, "invalid asset analysis: "+strings.Join(issues, "; "), false, nil)
}

func analyzeAssetResultIssues(result AnalyzeAssetResult) []string {
	issues := make([]string, 0, 6)
	if _, ok := allowedShotSizes[result.ShotSize]; !ok {
		issues = append(issues, fmt.Sprintf("invalid shot_size: %s", result.ShotSize))
	}
	if _, ok := allowedCameraMovements[result.CameraMovement]; !ok {
		issues = append(issues, fmt.Sprintf("invalid camera_movement: %s", result.CameraMovement))
	}

	sceneDescription := strings.TrimSpace(result.SceneDescription)
	actionDescription := strings.TrimSpace(result.ActionDescription)
	if sceneDescription == "" {
		issues = append(issues, "scene_description is required")
	}
	if len(result.VisualTags) == 0 {
		issues = append(issues, "visual_tags is required")
	}
	if strings.TrimSpace(result.SceneContext) == "" {
		issues = append(issues, "scene_context is required")
	}
	if actionDescription == "" {
		issues = append(issues, "action_description is required")
	}
	if result.FaceVisible && !result.PeoplePresence {
		issues = append(issues, "face_visible cannot be true when people_presence is false")
	}
	if !result.PeoplePresence && containsAnyPhrase(sceneDescription+" "+actionDescription, humanEvidencePhrases) {
		issues = append(issues, "people_presence must be true when a person or human hand is described")
	}
	return issues
}

// Quality issues trigger one corrective provider request, but do not discard an
// otherwise valid analysis when the provider repeats a generic phrase.
func analyzeAssetResultRepairIssues(result AnalyzeAssetResult) []string {
	issues := append([]string(nil), analyzeAssetResultIssues(result)...)
	sceneDescription := strings.TrimSpace(result.SceneDescription)
	actionDescription := strings.TrimSpace(result.ActionDescription)
	if phrase := firstContainedPhrase(sceneDescription, lowValueSceneDescriptionPhrases); phrase != "" {
		issues = append(issues, fmt.Sprintf("scene_description contains low-value presentation phrase %q", phrase))
	}
	if phrase := firstContainedPhrase(actionDescription, lowValueActionDescriptionPhrases); phrase != "" {
		issues = append(issues, fmt.Sprintf("action_description contains low-value presentation phrase %q", phrase))
	}
	if normalizeAnalysisDescription(sceneDescription) != "" && normalizeAnalysisDescription(sceneDescription) == normalizeAnalysisDescription(actionDescription) {
		issues = append(issues, "scene_description and action_description must provide different retrieval evidence")
	}
	return issues
}

func containsAnyPhrase(text string, phrases []string) bool {
	return firstContainedPhrase(text, phrases) != ""
}

func firstContainedPhrase(text string, phrases []string) string {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return phrase
		}
	}
	return ""
}

func normalizeAnalysisDescription(text string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n', ',', '，', '。', '.', '、', ';', '；', ':', '：':
			return -1
		default:
			return r
		}
	}, strings.TrimSpace(text))
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
