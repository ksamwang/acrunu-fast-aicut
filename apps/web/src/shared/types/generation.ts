export type EditingIntentBeat = {
  id: string;
  label: string;
  selling_point: string;
  visual_goal: string;
  source_type: "visual_only" | "talking_head" | "mixed";
};

export type NarrationSegment = {
  id: string;
  start_ms: number;
  end_ms: number;
  text: string;
};

export type EditPlanBeat = {
  id: string;
  start_ms: number;
  end_ms: number;
  label: string;
  visual_goal: string;
  source_type: "visual_only" | "talking_head" | "mixed";
};

export type ScriptVariantStatus = "draft" | "confirmed";

export type ScriptVariant = {
  id: string;
  order: number;
  hook: string;
  script_text: string;
  estimated_duration_ms: number;
  editing_intent: string;
  beats: EditingIntentBeat[];
  status: ScriptVariantStatus;
  intent_stale?: boolean;
  updated_at: string;
};

export type WorkbenchDraft = {
  product_id: string;
  selling_point_ids: string[];
  custom_selling_points: string[];
  voice_profile_id: string;
  variant_count: number;
  variants: ScriptVariant[];
  active_variant_id: string;
};

export type PrototypeRunStatus = "preparing" | "voicing" | "planning" | "rendering" | "completed";

export type PrototypeRun = {
  id: string;
  product_id: string;
  product_name: string;
  product_cover_url?: string;
  script_variant_id: string;
  voice_profile_id?: string;
  voice_profile_name?: string;
  hook: string;
  script_text: string;
  duration_ms: number;
  status: PrototypeRunStatus;
  progress: number;
  stage_label: string;
  started_at: string;
  editing_intent?: string;
  beats?: EditingIntentBeat[];
};

export type FinishedWorkStatus = "generating" | "completed" | "failed";

export type FinishedWork = {
  id: string;
  run_id: string;
  product_id: string;
  product_name: string;
  product_cover_url?: string;
  title: string;
  hook: string;
  voice_profile_id?: string;
  voice_profile_name?: string;
  script_text: string;
  duration_ms: number;
  status: FinishedWorkStatus;
  progress: number;
  stage_label: string;
  created_at: string;
  completed_at?: string;
  editing_intent?: string;
  beats?: EditingIntentBeat[];
  narration_segments?: NarrationSegment[];
  edit_plan?: EditPlanBeat[];
  audio_url?: string;
  error_message?: string;
  is_demo?: boolean;
};
