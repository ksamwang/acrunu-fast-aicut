import { apiRequest } from "../../shared/api/server-api";
import type { FinishedWork, ScriptVariant } from "../../shared/types/generation";
import type { VoiceAudition } from "../../shared/types/voice";

type VoiceoverTaskVariant = Pick<ScriptVariant, "hook" | "script_text" | "editing_intent" | "beats">;

type GenerateWorkbenchScriptsInput = {
  product_id: string;
  selling_point_ids: string[];
  custom_selling_points: string[];
  variant_count: number;
};

export function generateWorkbenchScripts(input: GenerateWorkbenchScriptsInput, token: string) {
  return apiRequest<ScriptVariant[]>("/api/workbench/scripts/generate", {
    method: "POST",
    body: JSON.stringify(input)
  }, token);
}

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
  outputRatio: "9:16" | "3:4",
  subtitlePresetID: string,
  variants: VoiceoverTaskVariant[],
  token: string
) {
  return apiRequest<FinishedWork[]>("/api/workbench/voiceover-tasks", {
    method: "POST",
    body: JSON.stringify({
      product_id: productID,
      voice_profile_id: voiceProfileID,
      output_ratio: outputRatio,
      subtitle_preset_id: subtitlePresetID,
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

export function regenerateVoiceoverWork(workID: string, token: string) {
  return apiRequest<FinishedWork>(`/api/workbench/works/${encodeURIComponent(workID)}/regenerate`, {
    method: "POST"
  }, token);
}

export function deleteVoiceoverWork(workID: string, token: string) {
  return apiRequest<{ deleted: boolean }>(`/api/workbench/works/${encodeURIComponent(workID)}`, {
    method: "DELETE"
  }, token);
}
