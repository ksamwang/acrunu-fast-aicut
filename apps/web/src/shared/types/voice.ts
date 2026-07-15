export type VoiceProfileStatus = "enabled" | "disabled";

export type VoicePreviewKind = "browser" | "reference_audio";

export type VoiceProfile = {
  id: string;
  name: string;
  language: string;
  style_tags: string[];
  reference_text: string;
  preview_text: string;
  preview_kind: VoicePreviewKind;
  preview_audio_url?: string;
  reference_audio_name?: string;
  status: VoiceProfileStatus;
  is_default: boolean;
  created_at: string;
  updated_at: string;
};
