package queue

const TypeTestTask = "test:task"
const TypeAssetExtractFrames = "asset:extract_frames"
const TypeAssetAnalyze = "asset:analyze"

const (
	FrameExtractionModeFixedInterval = "fixed_interval"
	FrameExtractionModeKeyframe      = "keyframe"
)

type TestTaskPayload struct {
	TaskID string `json:"task_id"`
}

type FrameExtractionStrategy struct {
	Mode             string `json:"mode"`
	FrameCount       int    `json:"frame_count,omitempty"`
	KeyframeWindowMs int    `json:"keyframe_window_ms,omitempty"`
}

type AssetExtractFramesPayload struct {
	TaskID      string                  `json:"task_id"`
	AssetID     string                  `json:"asset_id"`
	StorageKey  string                  `json:"storage_key"`
	DurationMs  int                     `json:"duration_ms"`
	Strategy    FrameExtractionStrategy `json:"strategy"`
	SkipAnalyze bool                    `json:"skip_analyze,omitempty"`
}

type AssetAnalyzePayload struct {
	TaskID  string `json:"task_id"`
	AssetID string `json:"asset_id"`
}
