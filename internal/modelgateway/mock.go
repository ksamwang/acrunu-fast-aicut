package modelgateway

import "context"

type FrameReference struct {
	FrameIndex  int    `json:"frame_index"`
	TimestampMs int    `json:"timestamp_ms"`
	StorageKey  string `json:"storage_key"`
}

type AnalyzeAssetInput struct {
	AssetID        string           `json:"asset_id"`
	SourceType     string           `json:"source_type"`
	DurationMs     int              `json:"duration_ms"`
	Width          int              `json:"width"`
	Height         int              `json:"height"`
	HasAudio       bool             `json:"has_audio"`
	AudioCodec     string           `json:"audio_codec,omitempty"`
	FrameSnapshots []FrameReference `json:"frame_snapshots,omitempty"`
}

type AnalyzeAssetResult struct {
	UsabilityStatus string         `json:"usability_status"`
	SceneDescription string        `json:"scene_description"`
	ShotSize        string         `json:"shot_size"`
	CameraMovement  string         `json:"camera_movement"`
	Subjects        []string       `json:"subjects"`
	SceneTags       []string       `json:"scene_tags"`
	QualityTags     []string       `json:"quality_tags"`
	ModelResult     map[string]any `json:"model_result"`
}

type AssetAnalyzer interface {
	AnalyzeAsset(ctx context.Context, input AnalyzeAssetInput) (AnalyzeAssetResult, error)
}

type MockAssetAnalyzer struct{}

func NewMockAssetAnalyzer() *MockAssetAnalyzer {
	return &MockAssetAnalyzer{}
}

func (a *MockAssetAnalyzer) AnalyzeAsset(_ context.Context, input AnalyzeAssetInput) (AnalyzeAssetResult, error) {
	sceneDescription := "product close-up with stable framing"
	shotSize := "close_up"
	cameraMovement := "static"
	subjects := []string{"product"}
	sceneTags := []string{"indoor", "demo"}
	qualityTags := []string{}
	usabilityStatus := "usable"

	if input.SourceType == "talking_head" {
		sceneDescription = "presenter delivers product message to camera"
		shotSize = "medium_close_up"
		cameraMovement = "static"
		subjects = []string{"person", "product"}
		sceneTags = []string{"talking_head", "indoor"}
	}

	if input.DurationMs >= 8000 {
		cameraMovement = "slow_push_in"
	}
	if len(input.FrameSnapshots) >= 3 && input.Width > input.Height {
		shotSize = "medium_shot"
	}
	if !input.HasAudio && input.SourceType == "talking_head" {
		qualityTags = append(qualityTags, "missing_expected_audio")
		usabilityStatus = "needs_review"
	}
	if input.Width == 0 || input.Height == 0 {
		qualityTags = append(qualityTags, "missing_resolution_metadata")
	}

	return AnalyzeAssetResult{
		UsabilityStatus:  usabilityStatus,
		SceneDescription: sceneDescription,
		ShotSize:         shotSize,
		CameraMovement:   cameraMovement,
		Subjects:         subjects,
		SceneTags:        sceneTags,
		QualityTags:      qualityTags,
		ModelResult: map[string]any{
			"provider":       "mock",
			"prompt_version": "phase2-v1",
			"frame_count":    len(input.FrameSnapshots),
			"source_type":    input.SourceType,
		},
	}, nil
}
