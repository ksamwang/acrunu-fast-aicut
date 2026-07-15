import { apiRequest } from "../../shared/api/server-api";
import type { VoiceProfile, VoiceProfileInput } from "../../shared/types/voice";

function profileFormData(input: VoiceProfileInput, referenceAudio?: File | null) {
  const form = new FormData();
  form.set("name", input.name.trim());
  form.set("language", input.language.trim());
  form.set("style_tags_json", JSON.stringify(input.style_tags.map((tag) => tag.trim()).filter(Boolean)));
  form.set("reference_text", input.reference_text.trim());
  form.set("preview_text", input.preview_text.trim());
  form.set("status", input.status);
  form.set("is_default", String(input.is_default));
  if (referenceAudio) {
    form.set("reference_audio", referenceAudio, referenceAudio.name);
  }
  return form;
}

export function listVoiceProfiles(_path: string, token: string) {
  return apiRequest<VoiceProfile[]>("/api/voice-profiles", {}, token);
}

export function createVoiceProfile(input: VoiceProfileInput, referenceAudio: File, token: string) {
  return apiRequest<VoiceProfile>("/api/admin/voice-profiles", {
    method: "POST",
    body: profileFormData(input, referenceAudio)
  }, token);
}

export function updateVoiceProfile(profileID: string, input: VoiceProfileInput, referenceAudio: File | null, token: string) {
  return apiRequest<VoiceProfile>(`/api/admin/voice-profiles/${encodeURIComponent(profileID)}`, {
    method: "PUT",
    body: profileFormData(input, referenceAudio)
  }, token);
}

export function deleteVoiceProfile(profileID: string, token: string) {
  return apiRequest<{ deleted: boolean }>(`/api/admin/voice-profiles/${encodeURIComponent(profileID)}`, { method: "DELETE" }, token);
}

export function setDefaultVoiceProfile(profileID: string, token: string) {
  return apiRequest<VoiceProfile>(`/api/admin/voice-profiles/${encodeURIComponent(profileID)}/default`, { method: "POST" }, token);
}
