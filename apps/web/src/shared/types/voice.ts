export type VoiceProfileStatus = "enabled" | "disabled";

export type VoicePreviewStatus = "queued" | "processing" | "ready" | "failed";

export type VoiceProfile = {
  id: string;
  name: string;
  language: string;
  style_tags: string[];
  reference_text: string;
  preview_text: string;
  preview_audio_url?: string;
  reference_audio_name?: string;
  preview_status: VoicePreviewStatus;
  preview_error?: string;
  status: VoiceProfileStatus;
  is_default: boolean;
  created_at: string;
  updated_at: string;
};

export type VoiceProfileInput = {
  name: string;
  language: string;
  style_tags: string[];
  reference_text: string;
  preview_text: string;
  status: VoiceProfileStatus;
  is_default: boolean;
};

export type VoiceAuditionStatus = "queued" | "synthesizing" | "completed" | "failed";

export type VoiceAudition = {
  id: string;
  task_id: string;
  voice_profile_id: string;
  voice_profile_name: string;
  text: string;
  audio_url?: string;
  sample_rate?: number;
  duration_ms?: number;
  status: VoiceAuditionStatus;
  error_message?: string;
  created_at: string;
  updated_at: string;
};
