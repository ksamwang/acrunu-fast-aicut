export type Asset = {
  id: string;
  product_id: string;
  asset_name?: string;
  storage_key: string;
  file_name: string;
  source_original_name?: string;
  source_type: string;
  status: string;
  analysis_status?: string;
  usability_status?: string;
  manual_clean_status?: string;
  duration_ms?: number;
  width?: number;
  height?: number;
  fps?: number;
  codec?: string;
  has_audio?: boolean;
  audio_codec?: string;
  bitrate_kbps?: number;
  reviewer_notes?: string;
  scene_description?: string;
  action_description?: string;
  shot_size?: string;
  camera_movement?: string;
  subjects?: string[];
  scene_tags?: string[];
  quality_tags?: string[];
  model_labels?: Record<string, unknown>;
  model_result?: Record<string, unknown>;
  review_overrides?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  analysis_error?: string;
  semantic_score?: number;
  updated_at?: string;
  analyzed_at?: string;
  archived_at?: string;
};

export type AssetSellingPointPayload = { selling_point_ids: string[] };

export type AssetListResponse = {
  items: Asset[];
  total: number;
  page: number;
  page_size: number;
};

export type AssetSelectionResponse = {
  asset_ids: string[];
  total: number;
};

export type AssetBulkArchiveResult = {
  archived: Asset[];
  skipped_ids: string[];
  failures: Array<{ asset_id: string; message: string }>;
};

export type AssetFrameSnapshot = {
  id: string;
  asset_id: string;
  frame_index: number;
  timestamp_ms: number;
  storage_key: string;
  width?: number;
  height?: number;
  created_at: string;
};

export type AssetFrameResponse = { asset_id: string; frames: AssetFrameSnapshot[] };

export type AssetSpeechSegment = {
  id: string;
  asset_id: string;
  start_ms: number;
  end_ms: number;
  transcript: string;
  confidence?: number;
  source: string;
  status: string;
  created_at: string;
  updated_at: string;
};

export type AssetEmbeddingTarget = {
  object_type: string;
  object_id: string;
  asset_id: string;
  text: string;
  metadata?: Record<string, unknown>;
};

export type AssetSemanticPreview = {
  asset_id: string;
  open_semantic_description: string;
  embedding_targets: AssetEmbeddingTarget[];
};

export type AssetEmbeddingObject = {
  id: string;
  asset_id: string;
  object_type: string;
  object_id: string;
  text: string;
  text_hash: string;
  provider_id: string;
  model: string;
  dimension: number;
  metadata?: Record<string, unknown>;
  status: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
};

export type AssetEmbeddingListResponse = { asset_id: string; items: AssetEmbeddingObject[] };

export type AssetEmbeddingRunResult = {
  asset_id: string;
  provider_id: string;
  model: string;
  dimension: number;
  objects: AssetEmbeddingObject[];
};

export type AssetReviewPayload = {
  scene_description: string;
  action_description: string;
  shot_size: string;
  camera_movement: string;
  subjects: string[];
  scene_tags: string[];
  quality_tags: string[];
  usability_status: string;
  reviewer_notes: string;
};
