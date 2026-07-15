import { apiRequest } from "../../shared/api/server-api";
import type { FinishedWork, ScriptVariant } from "../../shared/types/generation";
import type { VoiceAudition } from "../../shared/types/voice";

type VoiceoverTaskVariant = Pick<ScriptVariant, "hook" | "script_text" | "editing_intent" | "beats">;

export function createVoiceAudition(voiceProfileID: string, text: string, token: string) {
  return apiRequest<VoiceAudition>(`/api/voice-profiles/${encodeURIComponent(voiceProfileID)}/auditions`, {
    method: "POST",
    body: JSON.stringify({ text })
  }, token);
}

export function getVoiceAudition(auditionID: string, token: string) {
  return apiRequest<VoiceAudition>(`/api/voice-auditions/${encodeURIComponent(auditionID)}`, {}, token);
}

export function createVoiceoverTasks(
  productID: string,
  voiceProfileID: string,
  variants: VoiceoverTaskVariant[],
  token: string
) {
  return apiRequest<FinishedWork[]>("/api/workbench/voiceover-tasks", {
    method: "POST",
    body: JSON.stringify({
      product_id: productID,
      voice_profile_id: voiceProfileID,
      variants
    })
  }, token);
}

export function listVoiceoverWorks(token: string) {
  return apiRequest<FinishedWork[]>("/api/workbench/works", {}, token);
}

export function getVoiceoverWork(taskID: string, token: string) {
  return apiRequest<FinishedWork>(`/api/workbench/works/${encodeURIComponent(taskID)}`, {}, token);
}
