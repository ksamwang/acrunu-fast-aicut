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
	ProductName    string           `json:"product_name,omitempty"`
	DurationMs     int              `json:"duration_ms"`
	Width          int              `json:"width"`
	Height         int              `json:"height"`
	HasAudio       bool             `json:"has_audio"`
	AudioCodec     string           `json:"audio_codec,omitempty"`
	FrameSnapshots []FrameReference `json:"frame_snapshots,omitempty"`
}

type AnalyzeAssetResult struct {
	SceneDescription  string         `json:"scene_description"`
	ShotSize          string         `json:"shot_size"`
	CameraMovement    string         `json:"camera_movement"`
	VisualTags        []string       `json:"visual_tags"`
	QualityTags       []string       `json:"quality_tags"`
	VisibleProduct    bool           `json:"visible_product"`
	ProductPosition   string         `json:"product_position"`
	SceneContext      string         `json:"scene_context"`
	ActionDescription string         `json:"action_description"`
	PeoplePresence    bool           `json:"people_presence"`
	FaceVisible       bool           `json:"face_visible"`
	LightingCondition string         `json:"lighting_condition"`
	UsabilityStatus   string         `json:"-"`
	Subjects          []string       `json:"-"`
	SceneTags         []string       `json:"-"`
	ModelResult       map[string]any `json:"model_result"`
}

type AssetAnalyzer interface {
	AnalyzeAsset(ctx context.Context, input AnalyzeAssetInput) (AnalyzeAssetResult, error)
}

type MockAssetAnalyzer struct{}

func NewMockAssetAnalyzer() *MockAssetAnalyzer {
	return &MockAssetAnalyzer{}
}

func (a *MockAssetAnalyzer) AnalyzeAsset(_ context.Context, input AnalyzeAssetInput) (AnalyzeAssetResult, error) {
	promptBundle := BuildPromptBundle(input)
	sceneDescription := "product close-up with stable framing"
	shotSize := "close_up"
	cameraMovement := "static"
	visualTags := []string{"product", "indoor", "demo"}
	qualityTags := []string{}
	visibleProduct := true
	productPosition := "center"
	sceneContext := "indoor demo"
	actionDescription := "product is shown to camera"
	peoplePresence := false
	faceVisible := false
	lightingCondition := "normal indoor lighting"

	if input.SourceType == "talking_head" {
		sceneDescription = "presenter delivers product message to camera"
		shotSize = "medium_close_up"
		cameraMovement = "static"
		visualTags = []string{"person", "product", "talking_head", "indoor"}
		sceneContext = "talking head indoor"
		actionDescription = "presenter talks while showing the product"
		peoplePresence = true
		faceVisible = true
	}

	if input.DurationMs >= 8000 {
		cameraMovement = "slow_push_in"
	}
	if len(input.FrameSnapshots) >= 3 && input.Width > input.Height {
		shotSize = "medium_shot"
	}
	if !input.HasAudio && input.SourceType == "talking_head" {
		qualityTags = append(qualityTags, "missing_expected_audio")
	}
	if input.Width == 0 || input.Height == 0 {
		qualityTags = append(qualityTags, "missing_resolution_metadata")
	}

	return AnalyzeAssetResult{
		SceneDescription:  sceneDescription,
		ShotSize:          shotSize,
		CameraMovement:    cameraMovement,
		VisualTags:        visualTags,
		QualityTags:       qualityTags,
		VisibleProduct:    visibleProduct,
		ProductPosition:   productPosition,
		SceneContext:      sceneContext,
		ActionDescription: actionDescription,
		PeoplePresence:    peoplePresence,
		FaceVisible:       faceVisible,
		LightingCondition: lightingCondition,
		UsabilityStatus:   "usable",
		Subjects:          append([]string(nil), visualTags...),
		SceneTags:         append([]string(nil), visualTags...),
		ModelResult: map[string]any{
			"provider":        "mock",
			"prompt_version":  promptBundle.Version,
			"schema_version":  OutputSchemaVersion,
			"frame_count":     len(input.FrameSnapshots),
			"source_type":     input.SourceType,
			"prompt_overview": promptBundle.Prompts,
		},
	}, nil
}
