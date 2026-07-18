package queue

const TypeTestTask = "test:task"
const TypeAssetExtractFrames = "asset:extract_frames"
const TypeAssetAnalyze = "asset:analyze"
const TypeAssetEmbedding = "asset:embedding"
const TypeVoiceProfilePreview = "voice:profile_preview"
const TypeVoiceAudition = "voice:audition"
const TypeVoiceoverGenerate = "voice:generate"
const TypeEditPlanGenerate = "edit_plan:generate"
const TypeGenerationRender = "generation:render"
const TypeWorkbenchScriptGenerate = "workbench:script_generate"

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

type AssetEmbeddingPayload struct {
	TaskID  string `json:"task_id"`
	AssetID string `json:"asset_id"`
}

type VoiceProfilePreviewPayload struct {
	TaskID         string `json:"task_id"`
	VoiceProfileID string `json:"voice_profile_id"`
}

type VoiceAuditionPayload struct {
	TaskID         string `json:"task_id"`
	AuditionID     string `json:"audition_id"`
	VoiceProfileID string `json:"voice_profile_id,omitempty"`
}

type VoiceoverGeneratePayload struct {
	TaskID          string `json:"task_id"`
	GenerationRunID string `json:"generation_run_id,omitempty"`
	ScriptVariantID string `json:"script_variant_id"`
	VoiceoverID     string `json:"voiceover_id"`
}

type EditPlanGeneratePayload struct {
	TaskID          string `json:"task_id"`
	GenerationRunID string `json:"generation_run_id"`
	ScriptVariantID string `json:"script_variant_id"`
	VoiceoverID     string `json:"voiceover_id"`
}

type GenerationRenderPayload struct {
	TaskID          string `json:"task_id"`
	GenerationRunID string `json:"generation_run_id"`
}

type WorkbenchScriptGeneratePayload struct {
	JobID string `json:"job_id"`
}
