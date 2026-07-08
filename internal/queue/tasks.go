package queue

const TypeTestTask = "test:task"
const TypeAssetExtractFrames = "asset:extract_frames"
const TypeAssetAnalyze = "asset:analyze"

type TestTaskPayload struct {
	TaskID string `json:"task_id"`
}

type AssetExtractFramesPayload struct {
	AssetID     string `json:"asset_id"`
	StorageKey  string `json:"storage_key"`
	DurationMs  int    `json:"duration_ms"`
}

type AssetAnalyzePayload struct {
	AssetID string `json:"asset_id"`
}
