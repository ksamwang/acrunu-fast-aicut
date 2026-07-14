export type UploadToken = { token: string; product_id: string };

export type WorkspaceProbe = {
  duration_ms?: number;
  width?: number;
  height?: number;
  fps?: number;
  codec?: string;
  has_audio?: boolean;
  audio_codec?: string;
  audio_sample_rate?: number;
  bitrate_kbps?: number;
};

export type WorkspaceFrameSnapshot = {
  frame_index: number;
  timestamp_ms: number;
  image_url: string;
};

export type WorkspaceAnalysis = {
  scene_description?: string;
  shot_size?: string;
  camera_movement?: string;
  visual_tags?: string[];
  quality_tags?: string[];
  visible_product?: boolean;
  product_position?: string;
  scene_context?: string;
  action_description?: string;
  people_presence?: boolean;
  face_visible?: boolean;
  lighting_condition?: string;
};

export type WorkspaceTranscriptSegment = {
  start_ms: number;
  end_ms: number;
  text: string;
};

export type WorkspaceASRDraft = {
  text: string;
  segments: WorkspaceTranscriptSegment[];
  source_in_ms: number;
  source_out_ms: number;
  time_base: "selection_relative_ms";
  generated_at: string;
};

export type WorkspaceItem = {
  id: string;
  status: "pending" | "saved" | "ready_to_submit" | "submitted";
  product_id?: string;
  submitted_asset_id?: string;
  asset_name?: string;
  source_type?: "visual_only" | "talking_head";
  original_file_name: string;
  original_probe?: WorkspaceProbe;
  source_in_ms: number;
  source_out_ms: number;
  interpret_fps_enabled?: boolean;
  playback_fps?: number;
  speed_ratio?: number;
  transcript?: string;
  transcript_segments?: WorkspaceTranscriptSegment[];
  asr_draft?: WorkspaceASRDraft;
  reviewer_notes?: string;
  probe: WorkspaceProbe;
  preview_in_ms?: number;
  preview_out_ms?: number;
  preview_frame_snapshots: WorkspaceFrameSnapshot[];
  analysis?: WorkspaceAnalysis;
  vlm_status?: "idle" | "queued" | "running" | "ready" | "failed";
  vlm_error?: string;
  frame_snapshots: WorkspaceFrameSnapshot[];
  source_url: string;
  clean_shot_url?: string;
  checksum?: string;
  last_error?: string;
  updated_at: string;
};

export type WorkspaceListResponse = { items: WorkspaceItem[] };
export type WorkspaceItemResponse = { item: WorkspaceItem };
